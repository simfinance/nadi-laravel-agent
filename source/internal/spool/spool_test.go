package spool

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"nadi-agent/internal/servermetrics"
)

func TestAppendReadBatchAndAcknowledge(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "spool"), 1024*1024, 10)
	if err != nil {
		t.Fatalf("open spool: %v", err)
	}

	snapshot := servermetrics.Snapshot{
		Timestamp: time.Now().UTC(),
		Hostname:  "agent-host",
		OS:        "linux",
	}

	first, err := store.Append("server-1", "agent-1", snapshot)
	if err != nil {
		t.Fatalf("append first record: %v", err)
	}
	second, err := store.Append("server-1", "agent-1", snapshot)
	if err != nil {
		t.Fatalf("append second record: %v", err)
	}

	records, err := store.ReadBatch(10)
	if err != nil {
		t.Fatalf("read spool batch: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("expected two records, got %d", len(records))
	}
	if records[0].EventID != first.EventID || records[1].EventID != second.EventID {
		t.Fatalf("records were not returned in append order: %+v", records)
	}
	if records[0].EventType != "server_metrics" {
		t.Fatalf("unexpected event type: %q", records[0].EventType)
	}
	if records[0].ServerID != "server-1" || records[0].AgentID != "agent-1" {
		t.Fatalf("unexpected telemetry identity: %+v", records[0])
	}

	if err := store.Acknowledge([]string{first.EventID}); err != nil {
		t.Fatalf("acknowledge first record: %v", err)
	}

	records, err = store.ReadBatch(10)
	if err != nil {
		t.Fatalf("read remaining spool batch: %v", err)
	}
	if len(records) != 1 || records[0].EventID != second.EventID {
		t.Fatalf("unexpected remaining records: %+v", records)
	}
}

func TestAppendRejectsMissingIdentity(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "spool"), 1024, 10)
	if err != nil {
		t.Fatalf("open spool: %v", err)
	}

	_, err = store.Append("", "agent-1", servermetrics.Snapshot{Timestamp: time.Now().UTC()})
	if err == nil {
		t.Fatal("expected missing server identity to be rejected")
	}
}

func TestAppendRejectsWhenCapacityIsReached(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "spool"), 1024*1024, 1)
	if err != nil {
		t.Fatalf("open spool: %v", err)
	}

	snapshot := servermetrics.Snapshot{Timestamp: time.Now().UTC()}
	if _, err := store.Append("server-1", "agent-1", snapshot); err != nil {
		t.Fatalf("append first record: %v", err)
	}
	if _, err := store.Append("server-1", "agent-1", snapshot); !errors.Is(err, ErrFull) {
		t.Fatalf("expected ErrFull, got %v", err)
	}
}

func TestSpoolUsesRestrictedPermissions(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "spool")
	store, err := Open(directory, 1024*1024, 10)
	if err != nil {
		t.Fatalf("open spool: %v", err)
	}

	if _, err := store.Append("server-1", "agent-1", servermetrics.Snapshot{Timestamp: time.Now().UTC()}); err != nil {
		t.Fatalf("append record: %v", err)
	}

	directoryInfo, err := os.Stat(directory)
	if err != nil {
		t.Fatalf("stat spool directory: %v", err)
	}
	if directoryInfo.Mode().Perm() != 0700 {
		t.Fatalf("expected directory mode 0700, got %04o", directoryInfo.Mode().Perm())
	}

	fileInfo, err := os.Stat(filepath.Join(directory, "telemetry.jsonl"))
	if err != nil {
		t.Fatalf("stat spool file: %v", err)
	}
	if fileInfo.Mode().Perm() != 0600 {
		t.Fatalf("expected spool file mode 0600, got %04o", fileInfo.Mode().Perm())
	}
}

func TestClearRemovesQueuedTelemetry(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "spool")
	store, err := Open(directory, 1024*1024, 10)
	if err != nil {
		t.Fatalf("open spool: %v", err)
	}

	if _, err := store.Append("server-1", "agent-1", servermetrics.Snapshot{Timestamp: time.Now().UTC()}); err != nil {
		t.Fatalf("append record: %v", err)
	}
	if err := store.Clear(); err != nil {
		t.Fatalf("clear spool: %v", err)
	}

	records, err := store.ReadBatch(10)
	if err != nil {
		t.Fatalf("read cleared spool: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected empty spool, got %d records", len(records))
	}
}
