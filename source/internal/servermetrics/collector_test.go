package servermetrics

import (
	"math"
	"testing"
	"time"
)

func TestParseCPUStatAndCalculateUsage(t *testing.T) {
	previous, err := parseCPUStat([]byte("cpu 100 20 30 400 10 5 5 0 0 0\ncpu0 50 10 15 200 5 2 2 0 0 0\n"))
	if err != nil {
		t.Fatalf("parse previous CPU stat: %v", err)
	}

	current, err := parseCPUStat([]byte("cpu 150 20 40 440 10 5 5 0 0 0\ncpu0 75 10 20 220 5 2 2 0 0 0\n"))
	if err != nil {
		t.Fatalf("parse current CPU stat: %v", err)
	}

	usage, valid := calculateCPUUsage(previous, current)
	if !valid {
		t.Fatal("expected CPU usage delta to be valid")
	}
	if math.Abs(usage-60) > 0.001 {
		t.Fatalf("expected CPU usage 60, got %.3f", usage)
	}
}

func TestParseMemInfo(t *testing.T) {
	content := []byte("MemTotal:       1000 kB\nMemFree:         100 kB\nMemAvailable:    400 kB\nBuffers:          20 kB\nCached:          200 kB\n")

	stats, err := parseMemInfo(content)
	if err != nil {
		t.Fatalf("parse memory info: %v", err)
	}

	if stats.TotalBytes != 1024000 {
		t.Fatalf("unexpected total memory: %d", stats.TotalBytes)
	}
	if stats.AvailableBytes != 409600 {
		t.Fatalf("unexpected available memory: %d", stats.AvailableBytes)
	}
	if stats.UsedBytes != 614400 {
		t.Fatalf("unexpected used memory: %d", stats.UsedBytes)
	}
}

func TestParseMemInfoFallback(t *testing.T) {
	content := []byte("MemTotal: 1000 kB\nMemFree: 100 kB\nBuffers: 20 kB\nCached: 200 kB\n")

	stats, err := parseMemInfo(content)
	if err != nil {
		t.Fatalf("parse memory info fallback: %v", err)
	}
	if stats.AvailableBytes != 320*1024 {
		t.Fatalf("unexpected fallback available memory: %d", stats.AvailableBytes)
	}
}

func TestParseLoadAverage(t *testing.T) {
	stats, err := parseLoadAverage([]byte("0.25 0.50 1.25 2/100 1234\n"))
	if err != nil {
		t.Fatalf("parse load average: %v", err)
	}

	if stats.OneMinute != 0.25 || stats.FiveMinutes != 0.50 || stats.FifteenMinutes != 1.25 {
		t.Fatalf("unexpected load average: %+v", stats)
	}
}

func TestParseUptime(t *testing.T) {
	uptime, err := parseUptime([]byte("12345.67 9876.54\n"))
	if err != nil {
		t.Fatalf("parse uptime: %v", err)
	}
	if math.Abs(uptime-12345.67) > 0.001 {
		t.Fatalf("unexpected uptime: %.2f", uptime)
	}
}

func TestUnescapeMountField(t *testing.T) {
	actual := unescapeMountField("/mnt/data\\040archive")
	if actual != "/mnt/data archive" {
		t.Fatalf("unexpected mount point: %q", actual)
	}
}

func TestCalculateCPUUsageRejectsInvalidDelta(t *testing.T) {
	_, valid := calculateCPUUsage(
		cpuCounters{total: 100, idle: 80},
		cpuCounters{total: 100, idle: 80},
	)
	if valid {
		t.Fatal("expected zero total delta to be rejected")
	}
}

func TestCollectorCollectsHostSnapshot(t *testing.T) {
	collector := NewCollector()
	snapshot, err := collector.Collect(time.Now().UTC())
	if err != nil {
		t.Fatalf("collect host snapshot: %v", err)
	}

	if snapshot.Hostname == "" {
		t.Fatal("expected hostname to be populated")
	}
	if snapshot.CPU.LogicalCores < 1 {
		t.Fatalf("expected at least one logical CPU core, got %d", snapshot.CPU.LogicalCores)
	}
	if snapshot.Memory.TotalBytes == 0 {
		t.Fatal("expected total memory to be populated")
	}
	if snapshot.UptimeSeconds <= 0 {
		t.Fatalf("expected positive uptime, got %.2f", snapshot.UptimeSeconds)
	}
	if snapshot.Timestamp.IsZero() {
		t.Fatal("expected collection timestamp to be populated")
	}
	if snapshot.Network == nil {
		t.Fatal("expected network interface collection to be initialized")
	}
	if snapshot.Nginx.Timestamp.IsZero() {
		t.Fatal("expected Nginx collection timestamp to be populated")
	}
	if snapshot.Nginx.Processes == nil {
		t.Fatal("expected Nginx process collection to be initialized")
	}
}
