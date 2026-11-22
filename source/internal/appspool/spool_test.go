package appspool

import (
	"testing"
	"time"

	"nadi-agent/internal/appmonitor"
)

func TestApplicationSpoolIsolatesApplications(t *testing.T) {
	store, err := Open(t.TempDir(), DefaultMaxBytes, DefaultMaxRecords)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := appmonitor.Snapshot{Timestamp: time.Now().UTC()}
	if _, err := store.Append("server-1", "agent-1", "app-1", snapshot); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append("server-1", "agent-1", "app-2", snapshot); err != nil {
		t.Fatal(err)
	}

	appOne, err := store.ReadBatch("app-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	appTwo, err := store.ReadBatch("app-2", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(appOne) != 1 || appOne[0].ApplicationID != "app-1" {
		t.Fatalf("unexpected app-1 records: %#v", appOne)
	}
	if len(appTwo) != 1 || appTwo[0].ApplicationID != "app-2" {
		t.Fatalf("unexpected app-2 records: %#v", appTwo)
	}

	if err := store.Clear("app-1"); err != nil {
		t.Fatal(err)
	}
	appOne, err = store.ReadBatch("app-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	appTwo, err = store.ReadBatch("app-2", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(appOne) != 0 || len(appTwo) != 1 {
		t.Fatal("clearing one application spool affected another application")
	}
}

func TestApplicationSpoolAcknowledgesOnlyMatchingApplicationEvents(t *testing.T) {
	store, err := Open(t.TempDir(), DefaultMaxBytes, DefaultMaxRecords)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := appmonitor.Snapshot{Timestamp: time.Now().UTC()}
	first, err := store.Append("server-1", "agent-1", "app-1", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Append("server-1", "agent-1", "app-1", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append("server-1", "agent-1", "app-2", snapshot); err != nil {
		t.Fatal(err)
	}

	if err := store.Acknowledge("app-1", []string{first.EventID}); err != nil {
		t.Fatal(err)
	}
	appOne, err := store.ReadBatch("app-1", 10)
	if err != nil {
		t.Fatal(err)
	}
	appTwo, err := store.ReadBatch("app-2", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(appOne) != 1 || appOne[0].EventID != second.EventID {
		t.Fatalf("unexpected retained app-1 records: %#v", appOne)
	}
	if len(appTwo) != 1 {
		t.Fatal("acknowledging app-1 changed app-2 spool")
	}
}
