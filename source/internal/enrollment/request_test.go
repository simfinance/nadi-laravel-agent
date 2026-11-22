package enrollment

import (
	"encoding/json"
	"net"
	"strings"
	"testing"

	"nadi-agent/internal/state"
)

func TestBuildRequestCreatesCentralCompatiblePayload(t *testing.T) {
	manifest := Manifest{
		Applications: []ApplicationInput{
			{
				ApplicationKey:  "portal-production",
				ApplicationName: "Portal",
				Environment:     "production",
				BaseURL:         "https://portal.example.test",
			},
		},
	}
	agentState := state.State{
		AgentInstanceID: "8f3d2f44-8b8d-4c91-8e35-58c5a6f79e14",
		Hostname:        "agent-test",
	}

	body, requestID, err := BuildRequest(agentState, strings.Repeat("x", 76), manifest)
	if err != nil {
		t.Fatalf("BuildRequest returned error: %v", err)
	}
	if len(requestID) != 36 {
		t.Fatalf("expected UUID request ID, got %q", requestID)
	}

	var request EnrollmentRequest
	if err := json.Unmarshal(body, &request); err != nil {
		t.Fatalf("decode generated request: %v", err)
	}
	if request.RequestID != requestID {
		t.Fatalf("request ID mismatch: body=%q return=%q", request.RequestID, requestID)
	}
	if request.Agent.AgentInstanceID != agentState.AgentInstanceID {
		t.Fatalf("unexpected Agent instance ID: %q", request.Agent.AgentInstanceID)
	}
	expectedIPAddress, err := readServerIPv4Address()
	if err != nil {
		t.Fatalf("read expected server IPv4 address: %v", err)
	}
	if request.Server.IPAddress != expectedIPAddress {
		t.Fatalf("unexpected server IPv4 address: got %q, want %q", request.Server.IPAddress, expectedIPAddress)
	}
	if strings.Contains(request.Server.IPAddress, ":") {
		t.Fatalf("server IP address must be IPv4, got %q", request.Server.IPAddress)
	}
	if len(request.Applications) != 1 || request.Applications[0].ApplicationKey != "portal-production" {
		t.Fatalf("unexpected application manifest: %#v", request.Applications)
	}
}

func TestValidateManifestRejectsDuplicateApplicationKeys(t *testing.T) {
	manifest := Manifest{
		Applications: []ApplicationInput{
			{ApplicationKey: "duplicate", ApplicationName: "One", Environment: "production", BaseURL: "https://one.example.test"},
			{ApplicationKey: "duplicate", ApplicationName: "Two", Environment: "production", BaseURL: "https://two.example.test"},
		},
	}

	if err := validateManifest(manifest); err == nil {
		t.Fatal("expected duplicate application_key to be rejected")
	}
}

func TestBuildRequestRequires76CharacterEnrollmentToken(t *testing.T) {
	manifest := Manifest{
		Applications: []ApplicationInput{
			{ApplicationKey: "portal", ApplicationName: "Portal", Environment: "production", BaseURL: "https://portal.example.test"},
		},
	}

	if _, _, err := BuildRequest(state.State{AgentInstanceID: "instance", Hostname: "host"}, "short", manifest); err == nil {
		t.Fatal("expected invalid Enrollment token length to be rejected")
	}
}

func TestUsableIPv4RejectsLoopbackIPv6AndLinkLocal(t *testing.T) {
	tests := []struct {
		name    string
		address net.Addr
	}{
		{
			name:    "loopback IPv4",
			address: &net.IPNet{IP: net.ParseIP("127.0.0.1")},
		},
		{
			name:    "IPv6",
			address: &net.IPNet{IP: net.ParseIP("fe80::20c:29ff:fee3:6e2a")},
		},
		{
			name:    "link-local IPv4",
			address: &net.IPNet{IP: net.ParseIP("169.254.10.20")},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := usableIPv4(test.address); got != "" {
				t.Fatalf("expected address to be rejected, got %q", got)
			}
		})
	}
}

func TestUsableIPv4AcceptsPrivateServerAddress(t *testing.T) {
	address := &net.IPNet{IP: net.ParseIP("172.16.222.154")}
	if got := usableIPv4(address); got != "172.16.222.154" {
		t.Fatalf("unexpected IPv4 address: got %q", got)
	}
}
