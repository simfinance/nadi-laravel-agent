package servermetrics

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"nadi-agent/internal/nginxmonitor"
)

type Collector struct {
	previousCPU *cpuCounters
	nginx       *nginxmonitor.Collector
}

type Snapshot struct {
	Timestamp     time.Time             `json:"timestamp"`
	Hostname      string                `json:"hostname"`
	OS            string                `json:"os"`
	Architecture  string                `json:"architecture"`
	CPU           CPUStats              `json:"cpu"`
	Memory        MemoryStats           `json:"memory"`
	Disks         []DiskStats           `json:"disks"`
	Network       []NetworkStats        `json:"network_interfaces"`
	Nginx         nginxmonitor.Snapshot `json:"nginx"`
	LoadAverage   LoadAverage           `json:"load_average"`
	UptimeSeconds float64               `json:"uptime_seconds"`
}

type CPUStats struct {
	LogicalCores int      `json:"logical_cores"`
	UsagePercent *float64 `json:"usage_percent,omitempty"`
}

type MemoryStats struct {
	TotalBytes     uint64  `json:"total_bytes"`
	AvailableBytes uint64  `json:"available_bytes"`
	UsedBytes      uint64  `json:"used_bytes"`
	UsagePercent   float64 `json:"usage_percent"`
}

type DiskStats struct {
	MountPoint     string  `json:"mount_point"`
	Filesystem     string  `json:"filesystem"`
	TotalBytes     uint64  `json:"total_bytes"`
	UsedBytes      uint64  `json:"used_bytes"`
	AvailableBytes uint64  `json:"available_bytes"`
	UsagePercent   float64 `json:"usage_percent"`
}

type LoadAverage struct {
	OneMinute      float64 `json:"one_minute"`
	FiveMinutes    float64 `json:"five_minutes"`
	FifteenMinutes float64 `json:"fifteen_minutes"`
}

type cpuCounters struct {
	total uint64
	idle  uint64
}

func NewCollector() *Collector {
	return &Collector{
		nginx: nginxmonitor.NewCollector(),
	}
}

func (c *Collector) Reset() {
	c.previousCPU = nil
}

func (c *Collector) Collect(now time.Time) (Snapshot, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return Snapshot{}, fmt.Errorf("read hostname: %w", err)
	}

	cpuContent, err := os.ReadFile("/proc/stat")
	if err != nil {
		return Snapshot{}, fmt.Errorf("read CPU counters: %w", err)
	}
	currentCPU, err := parseCPUStat(cpuContent)
	if err != nil {
		return Snapshot{}, fmt.Errorf("parse CPU counters: %w", err)
	}

	memoryContent, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return Snapshot{}, fmt.Errorf("read memory information: %w", err)
	}
	memory, err := parseMemInfo(memoryContent)
	if err != nil {
		return Snapshot{}, fmt.Errorf("parse memory information: %w", err)
	}

	loadContent, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return Snapshot{}, fmt.Errorf("read load average: %w", err)
	}
	load, err := parseLoadAverage(loadContent)
	if err != nil {
		return Snapshot{}, fmt.Errorf("parse load average: %w", err)
	}

	uptimeContent, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return Snapshot{}, fmt.Errorf("read uptime: %w", err)
	}
	uptime, err := parseUptime(uptimeContent)
	if err != nil {
		return Snapshot{}, fmt.Errorf("parse uptime: %w", err)
	}

	disks, err := collectDisks("/proc/self/mounts")
	if err != nil {
		return Snapshot{}, fmt.Errorf("collect disk statistics: %w", err)
	}

	network, err := collectNetworkStats("/proc/net/dev")
	if err != nil {
		return Snapshot{}, fmt.Errorf("collect network statistics: %w", err)
	}

	snapshot := Snapshot{
		Timestamp:    now.UTC(),
		Hostname:     hostname,
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
		CPU: CPUStats{
			LogicalCores: runtime.NumCPU(),
		},
		Memory:        memory,
		Disks:         disks,
		Network:       network,
		Nginx:         c.nginx.Collect(context.Background(), now),
		LoadAverage:   load,
		UptimeSeconds: uptime,
	}

	if c.previousCPU != nil {
		usage, valid := calculateCPUUsage(*c.previousCPU, currentCPU)
		if valid {
			snapshot.CPU.UsagePercent = &usage
		}
	}

	c.previousCPU = &currentCPU

	return snapshot, nil
}

func parseCPUStat(content []byte) (cpuCounters, error) {
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "cpu" {
			continue
		}
		if len(fields) < 5 {
			return cpuCounters{}, errors.New("CPU stat line has too few fields")
		}

		var total uint64
		var idle uint64
		for index, field := range fields[1:] {
			value, err := strconv.ParseUint(field, 10, 64)
			if err != nil {
				return cpuCounters{}, fmt.Errorf("invalid CPU counter %q: %w", field, err)
			}
			if index < 8 {
				total += value
			}
			if index == 3 || index == 4 {
				idle += value
			}
		}

		return cpuCounters{total: total, idle: idle}, nil
	}

	return cpuCounters{}, errors.New("aggregate CPU stat line was not found")
}

func calculateCPUUsage(previous cpuCounters, current cpuCounters) (float64, bool) {
	if current.total <= previous.total || current.idle < previous.idle {
		return 0, false
	}

	totalDelta := current.total - previous.total
	idleDelta := current.idle - previous.idle
	if totalDelta == 0 || idleDelta > totalDelta {
		return 0, false
	}

	usage := (float64(totalDelta-idleDelta) / float64(totalDelta)) * 100
	return usage, true
}

func parseMemInfo(content []byte) (MemoryStats, error) {
	values := make(map[string]uint64)
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}

		key := strings.TrimSuffix(fields[0], ":")
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return MemoryStats{}, fmt.Errorf("invalid %s value: %w", key, err)
		}
		if len(fields) > 2 && fields[2] == "kB" {
			value *= 1024
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return MemoryStats{}, err
	}

	total, hasTotal := values["MemTotal"]
	available, hasAvailable := values["MemAvailable"]
	if !hasTotal || total == 0 {
		return MemoryStats{}, errors.New("MemTotal is missing or zero")
	}
	if !hasAvailable {
		available = values["MemFree"] + values["Buffers"] + values["Cached"]
	}
	if available > total {
		available = total
	}

	used := total - available
	return MemoryStats{
		TotalBytes:     total,
		AvailableBytes: available,
		UsedBytes:      used,
		UsagePercent:   (float64(used) / float64(total)) * 100,
	}, nil
}

func parseLoadAverage(content []byte) (LoadAverage, error) {
	fields := strings.Fields(string(content))
	if len(fields) < 3 {
		return LoadAverage{}, errors.New("load average contains fewer than three values")
	}

	values := make([]float64, 3)
	for index := range values {
		value, err := strconv.ParseFloat(fields[index], 64)
		if err != nil {
			return LoadAverage{}, fmt.Errorf("invalid load average value: %w", err)
		}
		values[index] = value
	}

	return LoadAverage{
		OneMinute:      values[0],
		FiveMinutes:    values[1],
		FifteenMinutes: values[2],
	}, nil
}

func parseUptime(content []byte) (float64, error) {
	fields := strings.Fields(string(content))
	if len(fields) == 0 {
		return 0, errors.New("uptime file is empty")
	}

	uptime, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid uptime value: %w", err)
	}
	if uptime < 0 {
		return 0, errors.New("uptime cannot be negative")
	}

	return uptime, nil
}

func collectDisks(mountsPath string) ([]DiskStats, error) {
	file, err := os.Open(mountsPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	pseudoFilesystems := map[string]struct{}{
		"autofs": {}, "binfmt_misc": {}, "cgroup": {}, "cgroup2": {},
		"configfs": {}, "debugfs": {}, "devpts": {}, "devtmpfs": {},
		"ramfs": {}, "tmpfs": {},
		"fusectl": {}, "mqueue": {}, "nsfs": {}, "proc": {},
		"pstore": {}, "rpc_pipefs": {}, "securityfs": {}, "sysfs": {},
		"tracefs": {},
	}

	seenMounts := make(map[string]struct{})
	disks := make([]DiskStats, 0)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}

		mountPoint := unescapeMountField(fields[1])
		filesystem := fields[2]
		if _, skipped := pseudoFilesystems[filesystem]; skipped {
			continue
		}
		if _, exists := seenMounts[mountPoint]; exists {
			continue
		}
		seenMounts[mountPoint] = struct{}{}

		var stats syscall.Statfs_t
		if err := syscall.Statfs(mountPoint, &stats); err != nil {
			continue
		}

		blockSize := uint64(stats.Bsize)
		total := stats.Blocks * blockSize
		available := stats.Bavail * blockSize
		used := total - (stats.Bfree * blockSize)
		if total == 0 {
			continue
		}
		if available > total {
			available = total
		}

		disks = append(disks, DiskStats{
			MountPoint:     mountPoint,
			Filesystem:     filesystem,
			TotalBytes:     total,
			UsedBytes:      used,
			AvailableBytes: available,
			UsagePercent:   (float64(used) / float64(total)) * 100,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return disks, nil
}

func unescapeMountField(value string) string {
	replacer := strings.NewReplacer(
		"\\040", " ",
		"\\011", "\t",
		"\\012", "\n",
		"\\134", "\\",
	)
	return replacer.Replace(value)
}
