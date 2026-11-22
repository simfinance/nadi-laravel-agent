package control

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPollSendsAuthenticatedPOSTAndDecodesResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", request.Method)
		}
		if request.URL.Path != PollPath {
			t.Errorf("unexpected request path: %s", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer test-agent-token" {
			t.Errorf("missing bearer authentication header")
		}
		if request.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected Content-Type: %s", request.Header.Get("Content-Type"))
		}

		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"success":true,"data":{"agent":{"id":"agent-123","agent_instance_id":"instance-123","status":"active","installation_state":"installed","agent_version":"0.3.0-dev","protocol_version":"1"},"control":{"poll_interval_seconds":30},"applications":[{"application_id":"app-123","name":"Example","registration_status":"approved","activation_status":"pending","monitoring_status":"dormant","endpoint":{"base_url":"https://app.example.test"},"monitoring_enabled":false,"activation_grant":null}],"commands":[],"server_time":"2026-11-20T03:30:00Z"}}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-agent-token", 2*time.Second)
	data, err := client.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll returned error: %v", err)
	}

	if data.Agent.ID != "agent-123" {
		t.Fatalf("unexpected agent ID: %q", data.Agent.ID)
	}
	if len(data.Applications) != 1 || data.Applications[0].ApplicationID != "app-123" {
		t.Fatalf("unexpected applications: %#v", data.Applications)
	}
}

func TestPollRejectsMissingToken(t *testing.T) {
	client := NewClient("https://central.example.test", "", time.Second)
	if _, err := client.Poll(context.Background()); err == nil {
		t.Fatal("expected missing token error")
	}
}

func TestPollRejectsCentralHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Error(writer, `{"success":false,"message":"unauthorized"}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewClient(server.URL, "bad-token", time.Second)
	_, err := client.Poll(context.Background())
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("expected HTTP 401 error, got: %v", err)
	}
}

func TestValidatePollDataRejectsIncompleteGrant(t *testing.T) {
	data := PollData{
		Agent: AgentInfo{ID: "agent-123"},
		Applications: []Application{
			{
				ApplicationID: "app-123",
				ActivationGrant: &ActivationGrant{
					GrantID: "grant-123",
				},
			},
		},
	}

	if err := ValidatePollData(data); err == nil {
		t.Fatal("expected incomplete grant to be rejected")
	}
}

func TestPollRejectsPollURLWithDifferentOrigin(t *testing.T) {
	client := NewClient("https://central.example.test", "test-agent-token", time.Second)
	client.SetPollURL("https://attacker.example.test/api/v1/nadi/agent/control/poll")

	if _, err := client.Poll(context.Background()); err == nil || !strings.Contains(err.Error(), "origin does not match") {
		t.Fatalf("expected cross-origin Poll URL rejection, got: %v", err)
	}
}

func TestPollRejectsPollURLWithUserInformation(t *testing.T) {
	client := NewClient("https://central.example.test", "test-agent-token", time.Second)
	client.SetPollURL("https://user:password@central.example.test/api/v1/nadi/agent/control/poll")

	if _, err := client.Poll(context.Background()); err == nil || !strings.Contains(err.Error(), "user information") {
		t.Fatalf("expected Poll URL user information rejection, got: %v", err)
	}
}

func TestPollDoesNotFollowRedirects(t *testing.T) {
	redirectTargetCalled := false
	target := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		redirectTargetCalled = true
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()

	source := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, target.URL+PollPath, http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	client := NewClient(source.URL, "test-agent-token", time.Second)
	if _, err := client.Poll(context.Background()); err == nil || !strings.Contains(err.Error(), "HTTP 307") {
		t.Fatalf("expected redirect response to be rejected, got: %v", err)
	}

	if redirectTargetCalled {
		t.Fatal("Control Poll followed a redirect")
	}
}
