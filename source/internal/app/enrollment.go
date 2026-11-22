package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"nadi-agent/internal/enrollment"
	"nadi-agent/internal/identity"
	"nadi-agent/internal/state"
)

func (a *App) Enroll(ctx context.Context, manifestPath string) error {
	store, err := state.Open(a.config.StateDir)
	if err != nil {
		return fmt.Errorf("initialize local state: %w", err)
	}
	defer store.Close()

	agentState, err := identity.Ensure(store)
	if err != nil {
		return fmt.Errorf("initialize Agent identity: %w", err)
	}

	if agentState.PendingEnrollment == nil {
		if agentState.AgentCredential != "" || agentState.EnrollmentStatus == "registered" {
			return errors.New("this Agent is already enrolled; refusing to create a second Enrollment request")
		}

		if strings.TrimSpace(a.config.EnrollmentToken) == "" {
			return errors.New("NADI_ENROLLMENT_TOKEN is required for a new Enrollment request")
		}

		manifest, err := enrollment.ReadManifest(manifestPath)
		if err != nil {
			return err
		}

		body, requestID, err := enrollment.BuildRequest(agentState, a.config.EnrollmentToken, manifest)
		if err != nil {
			return err
		}

		agentState.PendingEnrollment = &state.PendingEnrollment{
			RequestID: requestID,
			Body:      string(body),
		}
		if err := store.Save(agentState); err != nil {
			return fmt.Errorf("persist pending Enrollment request: %w", err)
		}

		a.logger.Info(
			"Enrollment request prepared and persisted",
			"request_id", requestID,
			"applications", len(manifest.Applications),
		)
	} else {
		a.logger.Warn(
			"Retrying the previously persisted Enrollment request",
			"request_id", agentState.PendingEnrollment.RequestID,
		)
	}

	var request enrollment.EnrollmentRequest
	if err := json.Unmarshal([]byte(agentState.PendingEnrollment.Body), &request); err != nil {
		return fmt.Errorf("decode persisted Enrollment request: %w", err)
	}
	if request.RequestID == "" || request.RequestID != agentState.PendingEnrollment.RequestID {
		return errors.New("persisted Enrollment request ID does not match its state record")
	}

	client := enrollment.NewClient(a.config.CentralURL, a.config.HTTPTimeout)
	defer client.CloseIdleConnections()

	responseBody, err := client.Enroll(ctx, []byte(agentState.PendingEnrollment.Body))
	if err != nil {
		return err
	}

	response, err := enrollment.DecodeResponse(responseBody)
	if err != nil {
		return fmt.Errorf("decode Central Enrollment response: %w", err)
	}

	updatedState, err := enrollment.ApplyResponse(agentState, response, request.Applications)
	if err != nil {
		return err
	}

	if err := store.Save(updatedState); err != nil {
		return fmt.Errorf("persist successful Enrollment response: %w", err)
	}

	a.logger.Info(
		"NADI Agent Enrollment completed",
		"agent_id", updatedState.CentralAgentID,
		"server_id", updatedState.CentralServerID,
		"applications", len(updatedState.Applications),
		"poll_interval_seconds", updatedState.PollIntervalSeconds,
	)

	return nil
}
