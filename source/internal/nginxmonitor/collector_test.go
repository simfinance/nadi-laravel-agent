package nginxmonitor

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestCollectorReportsMissingProbeSourcesAsUnavailable(t *testing.T) {
	collector := &Collector{
		procRoot: t.TempDir(),
		serviceLookup: func(context.Context) (ServiceStatus, error) {
			return ServiceStatus{Status: StatusUnavailable, Reason: "systemd unavailable"}, nil
		},
	}

	snapshot := collector.Collect(context.Background(), time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC))
	if snapshot.ProcessStatus != StatusNotDetected {
		t.Fatalf("expected process not_detected, got %s", snapshot.ProcessStatus)
	}
	if snapshot.HTTPAvailability.Status != StatusUnavailable {
		t.Fatalf("expected HTTP probe unavailable, got %s", snapshot.HTTPAvailability.Status)
	}
	if snapshot.MetricsEndpoint.Status != StatusUnavailable {
		t.Fatalf("expected metrics endpoint unavailable, got %s", snapshot.MetricsEndpoint.Status)
	}
	if snapshot.Collector.Status != StatusPartial {
		t.Fatalf("expected collector partial, got %s", snapshot.Collector.Status)
	}
}

func TestCollectorCountsMasterAndWorkerProcesses(t *testing.T) {
	root := t.TempDir()
	writeProcessFixture(t, root, "101", "nginx: master process", "R", 100, 1024)
	writeProcessFixture(t, root, "102", "nginx: worker process", "S", 200, 2048)

	collector := &Collector{
		procRoot: root,
		serviceLookup: func(context.Context) (ServiceStatus, error) {
			return ServiceStatus{Status: StatusRunning, ActiveState: "active"}, nil
		},
	}
	snapshot := collector.Collect(context.Background(), time.Now())
	if snapshot.TotalProcesses != 2 || snapshot.MasterProcesses != 1 || snapshot.WorkerProcesses != 1 {
		t.Fatalf("unexpected process counts: %+v", snapshot)
	}
	if snapshot.ProcessStatus != StatusRunning || snapshot.Service.Status != StatusRunning {
		t.Fatalf("expected Nginx running, got process=%s service=%s", snapshot.ProcessStatus, snapshot.Service.Status)
	}
}

func writeProcessFixture(t *testing.T, root, pid, command, state string, cpuTicks, rssKiB uint64) {
	t.Helper()
	processDirectory := root + "/" + pid
	if err := os.MkdirAll(processDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(processDirectory+"/comm", []byte("nginx\n"), 0600); err != nil {
		t.Fatal(err)
	}
	userTicks := cpuTicks / 2
	systemTicks := cpuTicks - userTicks
	stat := fmt.Sprintf("%s (nginx) %s 1 2 3 4 5 6 7 8 9 10 %d %d 14 15\n", pid, state, userTicks, systemTicks)
	if err := os.WriteFile(processDirectory+"/stat", []byte(stat), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(processDirectory+"/status", []byte(fmt.Sprintf("Name:\tnginx\nVmRSS:\t%d kB\n", rssKiB)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(processDirectory+"/cmdline", []byte(command), 0600); err != nil {
		t.Fatal(err)
	}
}
