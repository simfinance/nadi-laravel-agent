package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"nadi-agent/internal/activation"
	"nadi-agent/internal/control"
	"nadi-agent/internal/state"
)

func (a *App) processActivationGrants(ctx context.Context, store *state.Store, current state.State, pollData control.PollData) error {
	if err := a.flushPendingAcknowledgements(ctx, store, &current); err != nil {
		a.logger.Warn("Pending Activation Grant acknowledgements remain queued", "error", err)
	}

	var verifier *activation.Verifier
	for _, application := range pollData.Applications {
		if application.ActivationGrant == nil {
			continue
		}
		if application.RegistrationStatus != "approved" {
			a.logger.Warn("Activation Grant ignored because application registration is not approved", "application_id", application.ApplicationID)
			continue
		}

		if grantAlreadyAcknowledged(current, application.ActivationGrant.GrantID) {
			continue
		}

		if verifier == nil {
			newVerifier, verifyErr := activation.NewVerifier(a.config.ActivationPublicKeyPath, a.config.ActivationKeyID)
			if verifyErr != nil {
				return fmt.Errorf("initialize Activation Grant verifier: %w", verifyErr)
			}
			verifier = newVerifier
		}

		payload, err := verifier.Verify(*application.ActivationGrant, application, current, time.Now().UTC())
		if err != nil {
			a.logger.Error("Activation Grant rejected; monitoring remains disabled", "application_id", application.ApplicationID, "grant_id", application.ActivationGrant.GrantID, "error", err)
			continue
		}

		stored := state.StoredActivationGrant{
			GrantID:       application.ActivationGrant.GrantID,
			GrantVersion:  application.ActivationGrant.GrantVersion,
			KeyID:         application.ActivationGrant.KeyID,
			ApplicationID: application.ApplicationID,
			Payload:       append([]byte(nil), application.ActivationGrant.Payload...),
			Signature:     application.ActivationGrant.Signature,
			ExpiresAt:     mustParseTime(payload.ExpiresAt),
			ValidatedAt:   time.Now().UTC(),
		}
		current = upsertGrant(current, stored)

		acknowledgement, err := newSuccessAcknowledgement(stored.GrantID, stored.ApplicationID)
		if err != nil {
			return err
		}
		current.PendingAcknowledgements = upsertAcknowledgement(current.PendingAcknowledgements, acknowledgement)

		if err := store.Save(current); err != nil {
			return fmt.Errorf("persist verified Activation Grant and acknowledgement: %w", err)
		}

		a.logger.Info("Activation Grant verified and persisted", "application_id", stored.ApplicationID, "grant_id", stored.GrantID, "grant_version", stored.GrantVersion, "capabilities", payload.Capabilities)
	}

	if err := a.flushPendingAcknowledgements(ctx, store, &current); err != nil {
		a.logger.Warn("Activation Grant acknowledgement delivery failed; retry will continue", "error", err)
	}

	return nil
}

func (a *App) flushPendingAcknowledgements(ctx context.Context, store *state.Store, current *state.State) error {
	for len(current.PendingAcknowledgements) > 0 {
		pending := current.PendingAcknowledgements[0]
		request := control.AcknowledgementRequest{
			RequestID:     pending.RequestID,
			ApplicationID: pending.ApplicationID,
			Result:        pending.Result,
			ErrorCode:     pending.ErrorCode,
			Message:       pending.Message,
		}

		if err := a.controlHTTP.Acknowledge(ctx, pending.GrantID, request); err != nil {
			return err
		}

		current.PendingAcknowledgements = current.PendingAcknowledgements[1:]
		for index := range current.ActivationGrants {
			if current.ActivationGrants[index].GrantID == pending.GrantID {
				current.ActivationGrants[index].Acknowledged = true
			}
		}
		if err := store.Save(*current); err != nil {
			return fmt.Errorf("persist acknowledgement completion: %w", err)
		}

		a.logger.Info("Activation Grant acknowledgement accepted by Central", "grant_id", pending.GrantID, "application_id", pending.ApplicationID, "request_id", pending.RequestID)
	}
	return nil
}

func newSuccessAcknowledgement(grantID string, applicationID string) (state.PendingAcknowledgement, error) {
	requestID, err := newUUID()
	if err != nil {
		return state.PendingAcknowledgement{}, err
	}
	if grantID == "" || applicationID == "" {
		return state.PendingAcknowledgement{}, errors.New("grant_id and application_id are required for acknowledgement")
	}
	return state.PendingAcknowledgement{
		GrantID:       grantID,
		ApplicationID: applicationID,
		RequestID:     requestID,
		Result:        "success",
	}, nil
}

func upsertGrant(current state.State, grant state.StoredActivationGrant) state.State {
	for index, existing := range current.ActivationGrants {
		if existing.GrantID == grant.GrantID {
			current.ActivationGrants[index] = grant
			return current
		}
	}
	current.ActivationGrants = append(current.ActivationGrants, grant)
	return current
}

func upsertAcknowledgement(current []state.PendingAcknowledgement, acknowledgement state.PendingAcknowledgement) []state.PendingAcknowledgement {
	for _, existing := range current {
		if existing.GrantID == acknowledgement.GrantID {
			return current
		}
	}
	return append(current, acknowledgement)
}

func grantAlreadyAcknowledged(current state.State, grantID string) bool {
	for _, existing := range current.ActivationGrants {
		if existing.GrantID == grantID && existing.Acknowledged {
			return true
		}
	}
	return false
}

func mustParseTime(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339Nano, value)
	return parsed.UTC()
}

func newUUID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate acknowledgement request ID: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := make([]byte, 36)
	hex.Encode(encoded[0:8], value[0:4])
	encoded[8] = '-'
	hex.Encode(encoded[9:13], value[4:6])
	encoded[13] = '-'
	hex.Encode(encoded[14:18], value[6:8])
	encoded[18] = '-'
	hex.Encode(encoded[19:23], value[8:10])
	encoded[23] = '-'
	hex.Encode(encoded[24:36], value[10:16])
	return string(encoded), nil
}
