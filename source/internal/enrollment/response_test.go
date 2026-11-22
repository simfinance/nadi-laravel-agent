package enrollment

import (
	"testing"
	"time"

	"nadi-agent/internal/state"
)

func TestApplyResponsePersistsIssuedCredentials(t *testing.T) {
	expiry := time.Now().UTC().Add(90 * 24 * time.Hour).Format(time.RFC3339)
	current := state.State{
		AgentInstanceID:  "instance-123",
		EnrollmentStatus: "unregistered",
	}
	response := Response{
		Success: true,
		Data: ResponseData{
			ServerID:                 "server-123",
			AgentID:                  "agent-123",
			AgentInstanceID:          "instance-123",
			AgentStatus:              "registered",
			AgentCredential:          "agent-secret",
			AgentCredentialExpiresAt: expiry,
			Control: ControlInfo{
				PollURL:             "https://central.example.test/api/v1/nadi/agent/control/poll",
				PollIntervalSeconds: 30,
			},
			Applications: []ApplicationResponse{
				{
					ApplicationID:         "application-123",
					ApplicationKey:        "portal",
					ApplicationName:       "Portal",
					Environment:           "production",
					RegistrationID:        "registration-123",
					RegistrationStatus:    "pending",
					ActivationStatus:      "pending",
					MonitoringStatus:      "dormant",
					Endpoint:              "https://portal.example.test/_nadi/v1",
					ApplicationCredential: "application-secret",
				},
			},
		},
	}
	expected := []ApplicationInput{
		{ApplicationKey: "portal", ApplicationName: "Portal", Environment: "production"},
	}

	updated, err := ApplyResponse(current, response, expected)
	if err != nil {
		t.Fatalf("ApplyResponse returned error: %v", err)
	}
	if updated.AgentCredential != "agent-secret" {
		t.Fatal("Agent credential was not persisted in returned state")
	}
	if updated.Applications[0].ApplicationCredential != "application-secret" {
		t.Fatal("application credential was not persisted in returned state")
	}
	if updated.PendingEnrollment != nil {
		t.Fatal("pending Enrollment request was not cleared")
	}
	if updated.EnrollmentStatus != "registered" {
		t.Fatalf("unexpected Enrollment status: %q", updated.EnrollmentStatus)
	}
}

func TestApplyResponseRejectsMismatchedAgentIdentity(t *testing.T) {
	response := Response{
		Success: true,
		Data: ResponseData{
			AgentInstanceID: "different-instance",
		},
	}

	_, err := ApplyResponse(state.State{AgentInstanceID: "local-instance"}, response, nil)
	if err == nil {
		t.Fatal("expected mismatched Agent identity to be rejected")
	}
}
