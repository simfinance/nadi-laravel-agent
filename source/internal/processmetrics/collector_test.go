package processmetrics

import (
	"fmt"
	"os"
	"testing"
)

func TestParseProcessStat(t *testing.T) {
	content := []byte("123 (nginx: worker process) S 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15\n")
	pid, state, cpuTicks, err := parseProcessStat(content)
	if err != nil {
		t.Fatalf("parse process stat: %v", err)
	}
	if pid != 123 || state != "S" || cpuTicks != 23 {
		t.Fatalf("unexpected process stat: pid=%d state=%s cpu_ticks=%d", pid, state, cpuTicks)
	}
}

func TestParseResidentMemory(t *testing.T) {
	memory, err := parseResidentMemory([]byte("Name:\tnginx\nVmSize:\t2048 kB\nVmRSS:\t512 kB\n"))
	if err != nil {
		t.Fatalf("parse resident memory: %v", err)
	}
	if memory != 512*1024 {
		t.Fatalf("expected 512 KiB, got %d bytes", memory)
	}
}

func TestScanByName(t *testing.T) {
	root := t.TempDir()
	writeProcessFixture(t, root, "101", "nginx", "nginx: master process", "R", 100, 1024)
	writeProcessFixture(t, root, "102", "nginx", "nginx: worker process", "S", 200, 2048)
	writeProcessFixture(t, root, "103", "php-fpm", "php-fpm: pool www", "S", 300, 4096)

	processes, err := ScanByName(root, "nginx")
	if err != nil {
		t.Fatalf("scan nginx processes: %v", err)
	}
	if len(processes) != 2 {
		t.Fatalf("expected 2 nginx processes, got %d", len(processes))
	}
}

func writeProcessFixture(t *testing.T, root, pid, name, command, state string, cpuTicks, rssKiB uint64) {
	t.Helper()
	processDirectory := root + "/" + pid
	if err := os.MkdirAll(processDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(processDirectory+"/comm", []byte(name+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	userTicks := cpuTicks / 2
	systemTicks := cpuTicks - userTicks
	stat := fmt.Sprintf("%s (%s) %s 1 2 3 4 5 6 7 8 9 10 %d %d 14 15\n", pid, name, state, userTicks, systemTicks)
	if err := os.WriteFile(processDirectory+"/stat", []byte(stat), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(processDirectory+"/status", []byte(fmt.Sprintf("Name:\t%s\nVmRSS:\t%d kB\n", name, rssKiB)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(processDirectory+"/cmdline", []byte(command), 0600); err != nil {
		t.Fatal(err)
	}
}
