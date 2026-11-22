package enrollment

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"nadi-agent/internal/state"
)

func ApplyResponse(current state.State, response Response, expected []ApplicationInput) (state.State, error) {
	if !response.Success {
		message := strings.TrimSpace(response.Message)
		if message == "" {
			message = "Central rejected the Enrollment request"
		}
		return current, errors.New(message)
	}

	data := response.Data
	if strings.TrimSpace(data.ServerID) == "" || strings.TrimSpace(data.AgentID) == "" {
		return current, errors.New("Enrollment response is missing server_id or agent_id")
	}
	if data.AgentInstanceID != current.AgentInstanceID {
		return current, errors.New("Enrollment response agent_instance_id does not match local Agent identity")
	}
	if data.AgentStatus != "registered" {
		return current, fmt.Errorf("Enrollment response has unexpected agent_status %q", data.AgentStatus)
	}
	if strings.TrimSpace(data.AgentCredential) == "" {
		return current, errors.New("Enrollment response is missing agent_credential")
	}

	credentialExpiry, err := time.Parse(time.RFC3339, data.AgentCredentialExpiresAt)
	if err != nil {
		return current, fmt.Errorf("Enrollment response has invalid agent_credential_expires_at: %w", err)
	}
	if !credentialExpiry.After(time.Now().UTC()) {
		return current, errors.New("Enrollment response agent credential is already expired")
	}

	pollURL, err := url.ParseRequestURI(data.Control.PollURL)
	if err != nil || pollURL.Scheme != "https" || pollURL.Host == "" {
		return current, errors.New("Enrollment response contains an invalid HTTPS control.poll_url")
	}
	if data.Control.PollIntervalSeconds < 1 {
		return current, errors.New("Enrollment response has an invalid control.poll_interval_seconds")
	}
	if len(data.Applications) < 1 {
		return current, errors.New("Enrollment response contains no applications")
	}
	if len(expected) != len(data.Applications) {
		return current, errors.New("Enrollment response application count does not match the submitted manifest")
	}

	expectedByKey := make(map[string]ApplicationInput, len(expected))
	for _, item := range expected {
		expectedByKey[item.ApplicationKey] = item
	}

	applications := make([]state.EnrolledApplication, 0, len(data.Applications))
	seenKeys := make(map[string]struct{}, len(data.Applications))
	for index, application := range data.Applications {
		if application.ApplicationID == "" || application.RegistrationID == "" || application.ApplicationKey == "" {
			return current, fmt.Errorf("Enrollment response application at index %d is missing identity fields", index)
		}
		expectedApplication, exists := expectedByKey[application.ApplicationKey]
		if !exists || expectedApplication.ApplicationName != application.ApplicationName || expectedApplication.Environment != application.Environment {
			return current, fmt.Errorf("Enrollment response application at index %d does not match the submitted manifest", index)
		}
		if _, exists := seenKeys[application.ApplicationKey]; exists {
			return current, fmt.Errorf("Enrollment response contains duplicate application_key %q", application.ApplicationKey)
		}
		seenKeys[application.ApplicationKey] = struct{}{}
		if application.ApplicationCredential == "" {
			return current, fmt.Errorf("Enrollment response application %q is missing application_credential", application.ApplicationID)
		}

		endpoint, err := url.ParseRequestURI(application.Endpoint)
		if err != nil || endpoint.Scheme != "https" || endpoint.Host == "" {
			return current, fmt.Errorf("Enrollment response application %q has an invalid HTTPS endpoint", application.ApplicationID)
		}

		applications = append(applications, state.EnrolledApplication{
			ApplicationID:         application.ApplicationID,
			ApplicationKey:        application.ApplicationKey,
			ApplicationName:       application.ApplicationName,
			Environment:           application.Environment,
			RegistrationID:        application.RegistrationID,
			RegistrationStatus:    application.RegistrationStatus,
			ActivationStatus:      application.ActivationStatus,
			MonitoringStatus:      application.MonitoringStatus,
			Endpoint:              application.Endpoint,
			ApplicationCredential: application.ApplicationCredential,
		})
	}

	current.CentralServerID = data.ServerID
	current.CentralAgentID = data.AgentID
	current.AgentCredential = data.AgentCredential
	current.AgentCredentialExpiresAt = credentialExpiry.UTC()
	current.ControlPollURL = data.Control.PollURL
	current.PollIntervalSeconds = data.Control.PollIntervalSeconds
	current.Applications = applications
	current.EnrollmentStatus = data.AgentStatus
	current.PendingEnrollment = nil

	return current, nil
}

func DecodeResponse(body []byte) (Response, error) {
	var response Response
	if err := json.Unmarshal(body, &response); err != nil {
		return Response{}, err
	}

	return response, nil
}
