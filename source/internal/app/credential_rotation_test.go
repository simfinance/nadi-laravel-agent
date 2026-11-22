package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"nadi-agent/internal/config"
	"nadi-agent/internal/state"
)

func TestRotateCredentialPersistsNewCredentialBeforeACKAndCompletes(t *testing.T) {
	credentialExpiry := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339Nano)
	deliveryExpiry := time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339Nano)
	stateDir := t.TempDir()
	var ackUsedNewCredential bool

	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/api/v1/nadi/agent/credentials/deliveries/delivery-1":
			if request.Method != http.MethodGet || request.Header.Get("Authorization") != "Bearer old-secret" {
				t.Fatalf("retrieve did not use old credential")
			}
			writer.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(writer, `{"success":true,"data":{"delivery_id":"delivery-1","credential_id":"credential-2","token":"new-secret","expires_at":%q}}`, deliveryExpiry)
		case "/api/v1/nadi/agent/credentials/deliveries/delivery-1/ack":
			if request.Method != http.MethodPost || request.Header.Get("Authorization") != "Bearer new-secret" {
				t.Fatalf("ACK did not use new credential")
			}
			stateBytes, err := os.ReadFile(filepath.Join(stateDir, "state.json"))
			if err != nil {
				t.Fatalf("read state during ACK: %v", err)
			}
			var current state.State
			if err := json.Unmarshal(stateBytes, &current); err != nil {
				t.Fatalf("decode state during ACK: %v", err)
			}
			if current.AgentCredential != "new-secret" || current.CredentialRotation == nil || !current.CredentialRotation.AckStarted {
				t.Fatal("new credential was not persisted and switched before ACK")
			}
			ackUsedNewCredential = true
			writer.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(writer, `{"success":true,"message":"Credential rotation acknowledged successfully.","data":{"agent_id":"agent-1","credential_id":"credential-2","key_version":"2","status":"active","expires_at":%q}}`, credentialExpiry)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	store, err := state.Open(stateDir)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	initial := state.State{
		AgentInstanceID:  "instance-1",
		CentralAgentID:   "agent-1",
		AgentCredential:  "old-secret",
		EnrollmentStatus: "registered",
	}
	if err := store.Save(initial); err != nil {
		t.Fatalf("save initial state: %v", err)
	}
	store.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	application := New(config.Config{
		CentralURL:  server.URL,
		HTTPTimeout: time.Second,
		StateDir:    filepath.Clean(stateDir),
	}, logger)

	application.controlHTTP.HTTP = server.Client()
	if err := application.RotateCredential(context.Background(), "delivery-1"); err != nil {
		t.Fatalf("rotate credential: %v", err)
	}
	if !ackUsedNewCredential {
		t.Fatal("ACK was not sent")
	}

	store, err = state.Open(stateDir)
	if err != nil {
		t.Fatalf("reopen state: %v", err)
	}
	defer store.Close()
	current, err := store.Load()
	if err != nil {
		t.Fatalf("load final state: %v", err)
	}
	if current.AgentCredential != "new-secret" {
		t.Fatal("new credential is not active in persisted state")
	}
	if current.CredentialRotation != nil {
		t.Fatal("pending rotation was not cleared after ACK")
	}
	if current.AgentCredentialID != "credential-2" || current.AgentCredentialKeyVersion != 2 {
		t.Fatalf("credential metadata was not persisted: %+v", current)
	}
	if !current.AgentCredentialExpiresAt.After(time.Now().UTC()) {
		t.Fatal("new credential expiry was not persisted")
	}
}

func TestCredentialRotationResumesAfterACKNetworkFailure(t *testing.T) {
	credentialExpiry := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339Nano)
	stateDir := t.TempDir()
	ackAttempts := 0

	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer new-secret" {
			t.Fatalf("retry did not use the new credential")
		}
		ackAttempts++
		if ackAttempts == 1 {
			http.Error(writer, "temporary failure", http.StatusServiceUnavailable)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(writer, `{"success":true,"data":{"agent_id":"agent-1","credential_id":"credential-2","key_version":"2","status":"active","expires_at":%q}}`, credentialExpiry)
	}))
	defer server.Close()

	store, err := state.Open(stateDir)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	initial := state.State{
		AgentInstanceID: "instance-1",
		CentralAgentID:  "agent-1",
		AgentCredential: "new-secret",
		CredentialRotation: &state.CredentialRotation{
			DeliveryID:         "delivery-1",
			CredentialID:       "credential-2",
			PreviousCredential: "old-secret",
			NewCredential:      "new-secret",
			ExpiresAt:          time.Now().UTC().Add(10 * time.Minute),
			AckStarted:         true,
		},
	}
	if err := store.Save(initial); err != nil {
		t.Fatalf("save pending rotation: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	application := New(config.Config{
		CentralURL:  server.URL,
		HTTPTimeout: time.Second,
		StateDir:    stateDir,
	}, logger)
	application.controlHTTP.HTTP = server.Client()

	if err := application.resumeCredentialRotation(context.Background(), store, &initial); err == nil {
		t.Fatal("expected first ACK attempt to fail")
	}
	if initial.CredentialRotation == nil || initial.AgentCredential != "new-secret" {
		t.Fatal("failed ACK did not retain both credentials and pending rotation state")
	}

	reloaded, err := store.Load()
	if err != nil {
		t.Fatalf("reload pending state: %v", err)
	}
	if err := application.resumeCredentialRotation(context.Background(), store, &reloaded); err != nil {
		t.Fatalf("resume ACK after simulated restart: %v", err)
	}
	if ackAttempts != 2 {
		t.Fatalf("expected two ACK attempts, got %d", ackAttempts)
	}
	if reloaded.CredentialRotation != nil || reloaded.AgentCredential != "new-secret" {
		t.Fatal("rotation did not complete after retry")
	}
}

func TestExpiredDeliveryAfterACKStartedFailsClosedAndRetainsBothCredentials(t *testing.T) {
	stateDir := t.TempDir()
	store, err := state.Open(stateDir)
	if err != nil {
		t.Fatalf("open state: %v", err)
	}
	defer store.Close()

	current := state.State{
		AgentInstanceID: "instance-1",
		AgentCredential: "new-secret",
		CredentialRotation: &state.CredentialRotation{
			DeliveryID:         "delivery-1",
			CredentialID:       "credential-2",
			PreviousCredential: "old-secret",
			NewCredential:      "new-secret",
			ExpiresAt:          time.Now().UTC().Add(-time.Minute),
			AckStarted:         true,
		},
	}
	if err := store.Save(current); err != nil {
		t.Fatalf("save expired rotation: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	application := New(config.Config{StateDir: stateDir}, logger)
	if err := application.resumeCredentialRotation(context.Background(), store, &current); err == nil {
		t.Fatal("expected expired in-flight ACK to require Central recovery")
	}
	if current.AgentCredential != "new-secret" || current.CredentialRotation == nil || current.CredentialRotation.PreviousCredential != "old-secret" {
		t.Fatal("expired uncertain rotation did not retain both credentials")
	}
}
