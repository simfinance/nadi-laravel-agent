package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"nadi-agent/internal/config"
	"nadi-agent/internal/identity"
	"nadi-agent/internal/state"
)

func TestEnrollPersistsCentralCredentials(t *testing.T) {
	stateDirectory := t.TempDir()
	token := strings.Repeat("e", 76)
	manifestPath := filepath.Join(t.TempDir(), "enrollment.json")
	manifest := `{"applications":[{"application_key":"portal","application_name":"Portal","environment":"production","base_url":"https://portal.example.test"}]}`
	if err := os.WriteFile(manifestPath, []byte(manifest), 0600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	instanceID := initializeTestIdentity(t, stateDirectory)

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "" {
			t.Errorf("Enrollment request unexpectedly included Authorization")
		}
		fmt.Fprintf(writer, `{"success":true,"message":"Enrollment completed","data":{"server_id":"server-1","agent_id":"agent-1","agent_instance_id":"%s","agent_status":"registered","agent_credential":"agent-token","agent_credential_expires_at":"%s","control":{"poll_url":"https://central.example.test/api/v1/nadi/agent/control/poll","poll_interval_seconds":30},"applications":[{"application_id":"app-1","application_key":"portal","application_name":"Portal","environment":"production","registration_id":"registration-1","registration_status":"pending","activation_status":"pending","monitoring_status":"dormant","endpoint":"https://portal.example.test/_nadi/v1","application_credential":"app-token"}]}}`, instanceID, time.Now().UTC().Add(90*24*time.Hour).Format(time.RFC3339))
	}))
	defer server.Close()

	application := New(config.Config{
		CentralURL:      server.URL,
		EnrollmentToken: token,
		HTTPTimeout:     2 * time.Second,
		StateDir:        stateDirectory,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if err := application.Enroll(context.Background(), manifestPath); err != nil {
		t.Fatalf("Enroll returned error: %v", err)
	}

	store, err := state.Open(stateDirectory)
	if err != nil {
		t.Fatalf("open state after Enrollment: %v", err)
	}
	defer store.Close()

	current, err := store.Load()
	if err != nil {
		t.Fatalf("load state after Enrollment: %v", err)
	}
	if current.AgentCredential != "agent-token" || current.CentralAgentID != "agent-1" {
		t.Fatalf("Agent credential or Central ID not persisted: %#v", current)
	}
	if len(current.Applications) != 1 || current.Applications[0].ApplicationCredential != "app-token" {
		t.Fatalf("application credentials not persisted: %#v", current.Applications)
	}
	if current.PendingEnrollment != nil {
		t.Fatal("pending Enrollment was not cleared after success")
	}
}

func TestEnrollRetryReusesExactPersistedRequest(t *testing.T) {
	stateDirectory := t.TempDir()
	manifestPath := filepath.Join(t.TempDir(), "enrollment.json")
	manifest := `{"applications":[{"application_key":"portal","application_name":"Portal","environment":"production","base_url":"https://portal.example.test"}]}`
	if err := os.WriteFile(manifestPath, []byte(manifest), 0600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	instanceID := initializeTestIdentity(t, stateDirectory)

	var mutex sync.Mutex
	var firstBody string
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}

		mutex.Lock()
		defer mutex.Unlock()
		requestCount++
		if requestCount == 1 {
			firstBody = string(body)
			http.Error(writer, `{"success":false,"message":"temporary failure"}`, http.StatusServiceUnavailable)
			return
		}
		if string(body) != firstBody {
			t.Errorf("retry did not reuse the exact Enrollment request body")
		}
		fmt.Fprintf(writer, `{"success":true,"message":"ok","data":{"server_id":"server-1","agent_id":"agent-1","agent_instance_id":"%s","agent_status":"registered","agent_credential":"agent-token","agent_credential_expires_at":"%s","control":{"poll_url":"https://central.example.test/api/v1/nadi/agent/control/poll","poll_interval_seconds":30},"applications":[{"application_id":"app-1","application_key":"portal","application_name":"Portal","environment":"production","registration_id":"registration-1","registration_status":"pending","activation_status":"pending","monitoring_status":"dormant","endpoint":"https://portal.example.test/_nadi/v1","application_credential":"app-token"}]}}`, instanceID, time.Now().UTC().Add(90*24*time.Hour).Format(time.RFC3339))
	}))
	defer server.Close()

	cfg := config.Config{
		CentralURL:      server.URL,
		EnrollmentToken: strings.Repeat("r", 76),
		HTTPTimeout:     2 * time.Second,
		StateDir:        stateDirectory,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	firstAttempt := New(cfg, logger)
	if err := firstAttempt.Enroll(context.Background(), manifestPath); err == nil {
		t.Fatal("expected first Enrollment attempt to fail")
	}

	store, err := state.Open(stateDirectory)
	if err != nil {
		t.Fatalf("open state after first attempt: %v", err)
	}
	pending, err := store.Load()
	if err != nil {
		t.Fatalf("load pending state: %v", err)
	}
	if pending.PendingEnrollment == nil {
		t.Fatal("pending Enrollment request was not persisted after network failure")
	}
	persistedBody := pending.PendingEnrollment.Body
	store.Close()

	secondAttempt := New(cfg, logger)
	if err := secondAttempt.Enroll(context.Background(), manifestPath); err != nil {
		t.Fatalf("retry Enrollment: %v", err)
	}

	if !strings.Contains(persistedBody, `"request_id"`) {
		t.Fatal("persisted Enrollment request is missing request_id")
	}
	if requestCount != 2 {
		t.Fatalf("expected two Enrollment requests, got %d", requestCount)
	}
}

func initializeTestIdentity(t *testing.T, directory string) string {
	t.Helper()

	store, err := state.Open(directory)
	if err != nil {
		t.Fatalf("open state for test identity: %v", err)
	}
	defer store.Close()

	current, err := identity.Ensure(store)
	if err != nil {
		t.Fatalf("initialize test identity: %v", err)
	}

	return current.AgentInstanceID
}
