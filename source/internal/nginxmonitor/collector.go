package nginxmonitor

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"nadi-agent/internal/processmetrics"
)

type Status string

const (
	StatusRunning     Status = "running"
	StatusStopped     Status = "stopped"
	StatusNotDetected Status = "not_detected"
	StatusAvailable   Status = "available"
	StatusUnavailable Status = "unavailable"
	StatusUnsupported Status = "unsupported"
	StatusUnknown     Status = "unknown"
	StatusPartial     Status = "partial"
)

type CollectorStatus struct {
	Status      Status    `json:"status"`
	CollectedAt time.Time `json:"collected_at"`
	Error       string    `json:"error,omitempty"`
}

type ServiceStatus struct {
	Status      Status `json:"status"`
	LoadState   string `json:"load_state,omitempty"`
	ActiveState string `json:"active_state,omitempty"`
	SubState    string `json:"sub_state,omitempty"`
	Source      string `json:"source,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

type ProcessStats struct {
	PID           int    `json:"pid"`
	Role          string `json:"role"`
	State         string `json:"state"`
	CPUTimeTicks  uint64 `json:"cpu_time_ticks"`
	ResidentBytes uint64 `json:"resident_bytes"`
}

type ProbeStatus struct {
	Status Status `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type Snapshot struct {
	Timestamp        time.Time       `json:"timestamp"`
	Collector        CollectorStatus `json:"collector"`
	ProcessStatus    Status          `json:"process_status"`
	Service          ServiceStatus   `json:"service"`
	MasterProcesses  int             `json:"master_processes"`
	WorkerProcesses  int             `json:"worker_processes"`
	TotalProcesses   int             `json:"total_processes"`
	Processes        []ProcessStats  `json:"processes"`
	HTTPAvailability ProbeStatus     `json:"http_availability"`
	MetricsEndpoint  ProbeStatus     `json:"metrics_endpoint"`
	Metrics          map[string]any  `json:"metrics,omitempty"`
}

type Collector struct {
	procRoot      string
	serviceLookup func(context.Context) (ServiceStatus, error)
}

func NewCollector() *Collector {
	return &Collector{
		procRoot:      "/proc",
		serviceLookup: lookupSystemdService,
	}
}

func (c *Collector) Collect(ctx context.Context, now time.Time) Snapshot {
	snapshot := Snapshot{
		Timestamp: now.UTC(),
		Collector: CollectorStatus{
			Status:      StatusAvailable,
			CollectedAt: now.UTC(),
		},
		ProcessStatus:    StatusNotDetected,
		Service:          ServiceStatus{Status: StatusUnavailable, Reason: "service status source has not been checked"},
		Processes:        make([]ProcessStats, 0),
		HTTPAvailability: ProbeStatus{Status: StatusUnavailable, Reason: "no HTTP/HTTPS probe endpoint is configured"},
		MetricsEndpoint:  ProbeStatus{Status: StatusUnavailable, Reason: "no Nginx metrics endpoint is configured"},
	}

	processes, err := c.inspectProcesses()
	if err != nil {
		snapshot.Collector.Status = StatusUnavailable
		snapshot.Collector.Error = err.Error()
		snapshot.ProcessStatus = StatusUnknown
	} else {
		snapshot.Processes = processes
		snapshot.TotalProcesses = len(processes)
		for _, process := range processes {
			switch process.Role {
			case "master":
				snapshot.MasterProcesses++
			case "worker":
				snapshot.WorkerProcesses++
			}
		}
		if len(processes) > 0 {
			snapshot.ProcessStatus = StatusRunning
		} else {
			snapshot.ProcessStatus = StatusNotDetected
		}
	}

	service, serviceErr := c.serviceLookup(ctx)
	if serviceErr != nil {
		if snapshot.Collector.Status == StatusAvailable {
			snapshot.Collector.Status = StatusPartial
		}
		snapshot.Service = ServiceStatus{
			Status: StatusUnavailable,
			Reason: serviceErr.Error(),
		}
	} else {
		snapshot.Service = service
		if (service.Status == StatusUnavailable || service.Status == StatusUnknown) && snapshot.Collector.Status == StatusAvailable {
			snapshot.Collector.Status = StatusPartial
		}
	}

	if snapshot.ProcessStatus == StatusNotDetected && (snapshot.Service.ActiveState == "inactive" || snapshot.Service.ActiveState == "failed") {
		snapshot.ProcessStatus = StatusStopped
	}

	return snapshot
}

func (c *Collector) inspectProcesses() ([]ProcessStats, error) {
	processes, err := processmetrics.ScanByName(c.procRoot, "nginx")
	if err != nil {
		return nil, fmt.Errorf("inspect Nginx processes: %w", err)
	}

	stats := make([]ProcessStats, 0, len(processes))
	for _, process := range processes {
		role := "unknown"
		switch {
		case strings.Contains(process.Command, "master process"):
			role = "master"
		case strings.Contains(process.Command, "worker process"):
			role = "worker"
		}

		stats = append(stats, ProcessStats{
			PID:           process.PID,
			Role:          role,
			State:         process.State,
			CPUTimeTicks:  process.CPUTimeTicks,
			ResidentBytes: process.ResidentBytes,
		})
	}

	return stats, nil
}

func lookupSystemdService(ctx context.Context) (ServiceStatus, error) {
	commandContext, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	command := exec.CommandContext(
		commandContext,
		"systemctl",
		"show",
		"nginx.service",
		"--property=LoadState,ActiveState,SubState",
		"--no-pager",
	)
	output, err := command.Output()
	if err != nil {
		return ServiceStatus{}, fmt.Errorf("query nginx.service through systemd: %w", err)
	}

	properties := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		key, value, found := strings.Cut(scanner.Text(), "=")
		if found {
			properties[key] = value
		}
	}
	if err := scanner.Err(); err != nil {
		return ServiceStatus{}, err
	}

	loadState := properties["LoadState"]
	activeState := properties["ActiveState"]
	subState := properties["SubState"]
	status := StatusUnknown
	switch activeState {
	case "active":
		status = StatusRunning
	case "inactive", "failed":
		status = StatusStopped
	case "activating", "deactivating", "reloading":
		status = StatusUnknown
	}
	if loadState == "not-found" {
		status = StatusNotDetected
	}

	return ServiceStatus{
		Status:      status,
		LoadState:   loadState,
		ActiveState: activeState,
		SubState:    subState,
		Source:      "systemd",
	}, nil
}
