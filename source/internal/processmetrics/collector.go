package processmetrics

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Process struct {
	PID           int
	Name          string
	State         string
	CPUTimeTicks  uint64
	ResidentBytes uint64
	Command       string
}

func ScanByName(procRoot string, processName string) ([]Process, error) {
	if strings.TrimSpace(procRoot) == "" || strings.TrimSpace(processName) == "" {
		return nil, errors.New("proc root and process name are required")
	}

	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return nil, fmt.Errorf("read proc filesystem: %w", err)
	}

	processes := make([]Process, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}

		process, found, err := readProcess(filepath.Join(procRoot, entry.Name()))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if errors.Is(err, os.ErrPermission) {
				return nil, fmt.Errorf("process inspection permission denied for PID %s: %w", entry.Name(), err)
			}
			return nil, err
		}
		if found && process.Name == processName {
			processes = append(processes, process)
		}
	}

	return processes, nil
}

func readProcess(processDirectory string) (Process, bool, error) {
	comm, err := os.ReadFile(filepath.Join(processDirectory, "comm"))
	if err != nil {
		return Process{}, false, err
	}
	name := strings.TrimSpace(string(comm))

	statContent, err := os.ReadFile(filepath.Join(processDirectory, "stat"))
	if err != nil {
		return Process{}, false, err
	}
	pid, state, cpuTicks, err := parseProcessStat(statContent)
	if err != nil {
		return Process{}, false, err
	}

	statusContent, err := os.ReadFile(filepath.Join(processDirectory, "status"))
	if err != nil {
		return Process{}, false, err
	}
	residentBytes, err := parseResidentMemory(statusContent)
	if err != nil {
		return Process{}, false, err
	}

	command := ""
	cmdline, cmdlineErr := os.ReadFile(filepath.Join(processDirectory, "cmdline"))
	if cmdlineErr == nil {
		command = strings.ReplaceAll(string(cmdline), "\x00", " ")
		command = strings.TrimSpace(command)
	}

	return Process{
		PID:           pid,
		Name:          name,
		State:         state,
		CPUTimeTicks:  cpuTicks,
		ResidentBytes: residentBytes,
		Command:       command,
	}, true, nil
}

func parseProcessStat(content []byte) (int, string, uint64, error) {
	line := strings.TrimSpace(string(content))
	closingParen := strings.LastIndex(line, ")")
	if closingParen < 0 || closingParen+2 >= len(line) {
		return 0, "", 0, errors.New("invalid process stat format")
	}

	pidEnd := strings.IndexByte(line, ' ')
	if pidEnd < 1 {
		return 0, "", 0, errors.New("invalid process PID")
	}
	pid, err := strconv.Atoi(line[:pidEnd])
	if err != nil {
		return 0, "", 0, fmt.Errorf("parse process PID: %w", err)
	}

	fields := strings.Fields(line[closingParen+1:])
	if len(fields) <= 12 {
		return 0, "", 0, errors.New("process stat has insufficient fields")
	}
	cpuUser, err := strconv.ParseUint(fields[11], 10, 64)
	if err != nil {
		return 0, "", 0, fmt.Errorf("parse process user CPU ticks: %w", err)
	}
	cpuSystem, err := strconv.ParseUint(fields[12], 10, 64)
	if err != nil {
		return 0, "", 0, fmt.Errorf("parse process system CPU ticks: %w", err)
	}

	return pid, fields[0], cpuUser + cpuSystem, nil
}

func parseResidentMemory(content []byte) (uint64, error) {
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 || fields[0] != "VmRSS:" {
			continue
		}
		kibibytes, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse process resident memory: %w", err)
		}
		return kibibytes * 1024, nil
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return 0, errors.New("VmRSS is unavailable")
}
