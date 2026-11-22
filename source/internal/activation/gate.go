package activation

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"

	"nadi-agent/internal/control"
	"nadi-agent/internal/state"
)

type GateDecision struct {
	Enabled      bool
	Reason       string
	GrantID      string
	Capabilities []string
}

func (v *Verifier) Evaluate(
	application state.EnrolledApplication,
	polled *control.Application,
	agentState state.State,
	now time.Time,
) GateDecision {
	decision := GateDecision{
		Enabled: false,
		Reason:  "monitoring is disabled until Central confirms an active application and grant",
	}

	if polled == nil {
		decision.Reason = "application is missing from the latest Control Poll response"
		return decision
	}

	if strings.TrimSpace(polled.ApplicationID) == "" || polled.ApplicationID != application.ApplicationID {
		decision.Reason = "Control Poll application identity mismatch"
		return decision
	}

	if polled.RegistrationStatus != "approved" {
		decision.Reason = "application registration is not approved"
		return decision
	}

	if polled.ActivationStatus != "active" {
		decision.Reason = "application activation is not active"
		return decision
	}

	if polled.MonitoringStatus != "active" {
		decision.Reason = "application monitoring is not active"
		return decision
	}

	if !polled.MonitoringEnabled {
		decision.Reason = "Central Control Poll has monitoring_enabled=false"
		return decision
	}

	if polled.ActivationGrant == nil {
		decision.Reason = "Central did not provide an active Activation Grant"
		return decision
	}

	storedGrant, found := findStoredGrant(agentState, application.ApplicationID, polled.ActivationGrant.GrantID)
	if !found {
		decision.Reason = "Activation Grant has not been verified and persisted locally"
		return decision
	}

	if storedGrant.GrantVersion != polled.ActivationGrant.GrantVersion ||
		storedGrant.KeyID != polled.ActivationGrant.KeyID ||
		storedGrant.Signature != polled.ActivationGrant.Signature {
		decision.Reason = "stored Activation Grant metadata does not match the latest Central grant"
		return decision
	}

	payloadMatches, err := sameCanonicalPayload(storedGrant.Payload, polled.ActivationGrant.Payload)
	if err != nil {
		decision.Reason = fmt.Sprintf("unable to compare stored and latest Activation Grant payloads: %v", err)
		return decision
	}
	if !payloadMatches {
		decision.Reason = "stored Activation Grant payload does not match the latest Central grant"
		return decision
	}

	payload, err := v.Verify(*polled.ActivationGrant, *polled, agentState, now)
	if err != nil {
		decision.Reason = fmt.Sprintf("Activation Grant verification failed: %v", err)
		return decision
	}

	if !storedGrant.ExpiresAt.After(now) {
		decision.Reason = "stored Activation Grant has expired"
		return decision
	}

	if !storedGrant.Acknowledged {
		decision.Reason = "Activation Grant acknowledgement has not been accepted by Central"
		return decision
	}

	if len(payload.Capabilities) == 0 {
		decision.Reason = "Activation Grant has no monitoring capabilities"
		return decision
	}

	decision.Enabled = true
	decision.Reason = "Central application state and Activation Grant are valid"
	decision.GrantID = storedGrant.GrantID
	decision.Capabilities = append([]string(nil), payload.Capabilities...)

	return decision
}

func findStoredGrant(agentState state.State, applicationID string, grantID string) (state.StoredActivationGrant, bool) {
	for index := len(agentState.ActivationGrants) - 1; index >= 0; index-- {
		grant := agentState.ActivationGrants[index]
		if grant.ApplicationID == applicationID && grant.GrantID == grantID {
			return grant, true
		}
	}

	return state.StoredActivationGrant{}, false
}

func ValidateAgentMonitoringState(agent control.AgentInfo) error {
	if agent.InstallationState != "installed" {
		return errors.New("Agent installation_state is not installed")
	}

	return nil
}

func sameCanonicalPayload(stored []byte, latest []byte) (bool, error) {
	storedCanonical, err := CanonicalJSON(stored)
	if err != nil {
		return false, fmt.Errorf("canonicalize stored payload: %w", err)
	}

	latestCanonical, err := CanonicalJSON(latest)
	if err != nil {
		return false, fmt.Errorf("canonicalize latest payload: %w", err)
	}

	return bytes.Equal(storedCanonical, latestCanonical), nil
}
