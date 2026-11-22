package activation

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"nadi-agent/internal/control"
	"nadi-agent/internal/state"
)

func TestEvaluateFailsClosedWhenApplicationMissing(t *testing.T) {
	verifier := &Verifier{}
	application := state.EnrolledApplication{ApplicationID: "app-1"}

	decision := verifier.Evaluate(application, nil, state.State{}, time.Now().UTC())

	if decision.Enabled {
		t.Fatal("expected monitoring gate to remain disabled")
	}
	if decision.Reason != "application is missing from the latest Control Poll response" {
		t.Fatalf("unexpected gate reason: %s", decision.Reason)
	}
}

func TestEvaluateRequiresCentralActiveStatuses(t *testing.T) {
	verifier := &Verifier{}
	application := state.EnrolledApplication{ApplicationID: "app-1"}
	polled := &control.Application{
		ApplicationID:      "app-1",
		RegistrationStatus: "approved",
		ActivationStatus:   "pending",
		MonitoringStatus:   "active",
		MonitoringEnabled:  true,
	}

	decision := verifier.Evaluate(application, polled, state.State{}, time.Now().UTC())

	if decision.Enabled {
		t.Fatal("expected monitoring gate to remain disabled while activation is pending")
	}
	if decision.Reason != "application activation is not active" {
		t.Fatalf("unexpected gate reason: %s", decision.Reason)
	}
}

func TestValidateAgentMonitoringStateRequiresInstalled(t *testing.T) {
	err := ValidateAgentMonitoringState(control.AgentInfo{InstallationState: "registered"})
	if err == nil {
		t.Fatal("expected non-installed Agent to be rejected")
	}

	if err := ValidateAgentMonitoringState(control.AgentInfo{InstallationState: "installed"}); err != nil {
		t.Fatalf("expected installed Agent to pass: %v", err)
	}
}

func TestEvaluateOpensOnlyForVerifiedAcknowledgedActiveGrant(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate test key: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	payload := Payload{
		GrantID:           "grant-1",
		GrantVersion:      1,
		KeyID:             "test-key",
		RegistrationID:    "registration-1",
		ApplicationID:     "app-1",
		AgentID:           "agent-1",
		ServerID:          "server-1",
		IssuedAt:          now.Add(-time.Minute).Format(time.RFC3339Nano),
		ExpiresAt:         now.Add(time.Hour).Format(time.RFC3339Nano),
		Capabilities:      []string{"server_metrics", "database_monitoring"},
		MonitoringEnabled: true,
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal test payload: %v", err)
	}
	canonical, err := CanonicalJSON(payloadJSON)
	if err != nil {
		t.Fatalf("canonicalize test payload: %v", err)
	}

	signature := ed25519.Sign(privateKey, canonical)
	grant := control.ActivationGrant{
		GrantID:      payload.GrantID,
		GrantVersion: payload.GrantVersion,
		KeyID:        payload.KeyID,
		Payload:      payloadJSON,
		Signature:    base64.StdEncoding.EncodeToString(signature),
	}

	polled := &control.Application{
		ApplicationID:      payload.ApplicationID,
		RegistrationStatus: "approved",
		ActivationStatus:   "active",
		MonitoringStatus:   "active",
		MonitoringEnabled:  true,
		ActivationGrant:    &grant,
	}
	application := state.EnrolledApplication{
		ApplicationID:  payload.ApplicationID,
		RegistrationID: payload.RegistrationID,
	}
	storedGrant := state.StoredActivationGrant{
		GrantID:       grant.GrantID,
		GrantVersion:  grant.GrantVersion,
		KeyID:         grant.KeyID,
		ApplicationID: payload.ApplicationID,
		Payload:       payloadJSON,
		Signature:     grant.Signature,
		ExpiresAt:     now.Add(time.Hour),
		Acknowledged:  true,
	}
	agentState := state.State{
		CentralAgentID:  "agent-1",
		CentralServerID: "server-1",
		Applications:    []state.EnrolledApplication{application},
		ActivationGrants: []state.StoredActivationGrant{
			storedGrant,
		},
	}
	verifier := &Verifier{
		publicKey: publicKey,
		keyID:     "test-key",
	}

	var payloadFields map[string]json.RawMessage
	if err := json.Unmarshal(payloadJSON, &payloadFields); err != nil {
		t.Fatalf("decode grant payload fields: %v", err)
	}
	reorderedPayload, err := json.Marshal(payloadFields)
	if err != nil {
		t.Fatalf("marshal reordered grant payload: %v", err)
	}
	agentState.ActivationGrants[0].Payload = reorderedPayload

	decision := verifier.Evaluate(application, polled, agentState, now)
	if !decision.Enabled {
		t.Fatalf("expected semantically identical grant payload to open gate, got: %s", decision.Reason)
	}
	if decision.GrantID != grant.GrantID {
		t.Fatalf("unexpected grant ID: %s", decision.GrantID)
	}
	if len(decision.Capabilities) != 2 {
		t.Fatalf("expected two capabilities, got %d", len(decision.Capabilities))
	}

	polled.MonitoringStatus = "suspended"
	decision = verifier.Evaluate(application, polled, agentState, now)
	if decision.Enabled {
		t.Fatal("expected suspended monitoring to close the gate")
	}
}
