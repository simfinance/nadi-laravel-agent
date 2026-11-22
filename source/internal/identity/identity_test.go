package identity

import (
	"testing"

	"nadi-agent/internal/state"
)

func TestEnsureCreatesAndPersistsIdentity(t *testing.T) {
	store, err := state.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open state store: %v", err)
	}
	defer store.Close()

	first, err := Ensure(store)
	if err != nil {
		t.Fatalf("ensure identity: %v", err)
	}

	if first.AgentInstanceID == "" {
		t.Fatal("agent instance ID is empty")
	}

	if first.Hostname == "" {
		t.Fatal("hostname is empty")
	}

	if first.MachineFingerprint == "" {
		t.Fatal("machine fingerprint is empty")
	}

	if first.EnrollmentStatus != "unregistered" {
		t.Fatalf("unexpected enrollment status: %q", first.EnrollmentStatus)
	}

	second, err := Ensure(store)
	if err != nil {
		t.Fatalf("ensure identity on second call: %v", err)
	}

	if first.AgentInstanceID != second.AgentInstanceID {
		t.Fatal("agent instance ID changed between calls")
	}
}
