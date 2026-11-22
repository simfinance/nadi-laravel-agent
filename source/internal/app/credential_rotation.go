package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"nadi-agent/internal/control"
	"nadi-agent/internal/state"
)

func (a *App) RotateCredential(ctx context.Context, deliveryID string) error {
	if strings.TrimSpace(a.config.AgentToken) != "" {
		return errors.New("credential rotation cannot run while NADI_AGENT_TOKEN overrides the persisted credential")
	}

	store, err := state.Open(a.config.StateDir)
	if err != nil {
		return fmt.Errorf("open Agent state for credential rotation: %w", err)
	}
	defer store.Close()

	current, err := store.Load()
	if err != nil {
		return fmt.Errorf("load Agent state for credential rotation: %w", err)
	}
	if current.CredentialRotation != nil {
		return errors.New("a credential rotation is already pending; resume it by starting the Agent")
	}
	if strings.TrimSpace(current.AgentCredential) == "" {
		return errors.New("persisted Agent credential is unavailable")
	}
	if !current.AgentCredentialExpiresAt.IsZero() && !current.AgentCredentialExpiresAt.After(time.Now().UTC()) {
		return errors.New("persisted Agent credential is expired; credential delivery cannot be retrieved")
	}

	client := control.NewClient(a.config.CentralURL, current.AgentCredential, a.config.HTTPTimeout)
	client.HTTP = a.controlHTTP.HTTP

	delivery, err := client.RetrieveCredentialDelivery(ctx, deliveryID)
	if err != nil {
		return err
	}

	if delivery.DeliveryID != deliveryID {
		return errors.New("retrieved credential delivery ID does not match the requested delivery")
	}

	expiresAt, err := time.Parse(time.RFC3339Nano, delivery.ExpiresAt)
	if err != nil || !expiresAt.After(time.Now().UTC()) {
		return errors.New("retrieved credential delivery is expired or has an invalid expiry")
	}

	current.CredentialRotation = &state.CredentialRotation{
		DeliveryID:         delivery.DeliveryID,
		CredentialID:       delivery.CredentialID,
		PreviousCredential: current.AgentCredential,
		NewCredential:      delivery.Token,
		ExpiresAt:          expiresAt.UTC(),
	}
	if err := store.Save(current); err != nil {
		return fmt.Errorf("persist staged credential rotation: %w", err)
	}

	a.logger.Info(
		"New Agent credential securely staged; previous credential remains available",
		"delivery_id", delivery.DeliveryID,
		"credential_id", delivery.CredentialID,
		"expires_at", expiresAt.UTC(),
	)

	if err := a.resumeCredentialRotation(ctx, store, &current); err != nil {
		return err
	}

	return nil
}

func (a *App) resumeCredentialRotation(ctx context.Context, store *state.Store, current *state.State) error {
	rotation := current.CredentialRotation
	if rotation == nil {
		return nil
	}

	now := time.Now().UTC()
	if !rotation.ExpiresAt.After(now) {
		if !rotation.AckStarted {
			current.CredentialRotation = nil
			if err := store.Save(*current); err != nil {
				return fmt.Errorf("clear expired, unactivated credential delivery: %w", err)
			}
			return errors.New("credential delivery expired before credential switch; previous credential retained")
		}

		return errors.New("credential delivery expired after ACK was started; both credentials are retained and Central recovery is required")
	}

	if !rotation.AckStarted {
		if current.AgentCredential != rotation.PreviousCredential {
			return errors.New("credential rotation state does not match the currently active previous credential")
		}

		current.AgentCredential = rotation.NewCredential
		rotation.AckStarted = true
		if err := store.Save(*current); err != nil {
			current.AgentCredential = rotation.PreviousCredential
			rotation.AckStarted = false
			return fmt.Errorf("persist new active credential before ACK: %w", err)
		}

		a.logger.Info(
			"Agent switched to the persisted new credential; ACK will use the new credential",
			"delivery_id", rotation.DeliveryID,
			"credential_id", rotation.CredentialID,
		)
	}

	if current.AgentCredential != rotation.NewCredential {
		return errors.New("active Agent credential does not match the pending rotation credential")
	}

	client := control.NewClient(a.config.CentralURL, rotation.NewCredential, a.config.HTTPTimeout)
	client.HTTP = a.controlHTTP.HTTP

	acknowledgement, err := client.AcknowledgeCredentialRotation(
		ctx,
		rotation.DeliveryID,
		current.CentralAgentID,
		rotation.CredentialID,
	)
	if err != nil {
		return err
	}

	current.AgentCredentialID = acknowledgement.CredentialID
	current.AgentCredentialKeyVersion = acknowledgement.KeyVersion
	current.AgentCredentialExpiresAt = acknowledgement.ExpiresAt
	current.CredentialRotation = nil
	if err := store.Save(*current); err != nil {
		return fmt.Errorf("Central acknowledged rotation but local completion could not be persisted; retry ACK after restart: %w", err)
	}

	a.logger.Info(
		"Central confirmed Agent credential rotation",
		"delivery_id", rotation.DeliveryID,
		"credential_id", acknowledgement.CredentialID,
		"key_version", acknowledgement.KeyVersion,
		"status", acknowledgement.Status,
	)

	return nil
}
