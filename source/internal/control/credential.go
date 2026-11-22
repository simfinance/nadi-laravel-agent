package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const maxCredentialResponse = 64 << 10

type CredentialDelivery struct {
	DeliveryID   string `json:"delivery_id"`
	CredentialID string `json:"credential_id"`
	Token        string `json:"token"`
	ExpiresAt    string `json:"expires_at"`
}

type credentialDeliveryEnvelope struct {
	Success bool               `json:"success"`
	Message string             `json:"message,omitempty"`
	Data    CredentialDelivery `json:"data"`
}

type CredentialAcknowledgement struct {
	AgentID      string
	CredentialID string
	KeyVersion   int
	Status       string
	ExpiresAt    time.Time
}

type credentialAcknowledgementEnvelope struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Data    struct {
		AgentID      string          `json:"agent_id"`
		CredentialID string          `json:"credential_id"`
		KeyVersion   json.RawMessage `json:"key_version"`
		Status       string          `json:"status"`
		ExpiresAt    string          `json:"expires_at"`
	} `json:"data"`
}

func (c *Client) RetrieveCredentialDelivery(ctx context.Context, deliveryID string) (CredentialDelivery, error) {
	if strings.TrimSpace(c.Token) == "" {
		return CredentialDelivery{}, errors.New("Agent credential is not configured")
	}
	if strings.TrimSpace(deliveryID) == "" || strings.Contains(deliveryID, "/") {
		return CredentialDelivery{}, errors.New("credential delivery ID is invalid")
	}

	endpoint, err := c.credentialDeliveryURL(deliveryID, "")
	if err != nil {
		return CredentialDelivery{}, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return CredentialDelivery{}, fmt.Errorf("create credential delivery request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+c.Token)

	response, err := c.HTTP.Do(request)
	if err != nil {
		return CredentialDelivery{}, fmt.Errorf("send credential delivery request: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxCredentialResponse+1))
	if err != nil {
		return CredentialDelivery{}, fmt.Errorf("read credential delivery response: %w", err)
	}
	if len(body) > maxCredentialResponse {
		return CredentialDelivery{}, errors.New("credential delivery response exceeds the allowed size")
	}
	if response.StatusCode != http.StatusOK {
		return CredentialDelivery{}, fmt.Errorf("Central credential delivery retrieve returned HTTP %d", response.StatusCode)
	}

	var envelope credentialDeliveryEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return CredentialDelivery{}, errors.New("Central credential delivery response is malformed")
	}
	if !envelope.Success {
		return CredentialDelivery{}, errors.New("Central did not confirm credential delivery retrieval")
	}

	delivery := envelope.Data
	if delivery.DeliveryID != deliveryID {
		return CredentialDelivery{}, errors.New("credential delivery identity mismatch")
	}
	if strings.TrimSpace(delivery.CredentialID) == "" || strings.TrimSpace(delivery.Token) == "" {
		return CredentialDelivery{}, errors.New("credential delivery is missing required credential fields")
	}

	expiresAt, err := time.Parse(time.RFC3339Nano, delivery.ExpiresAt)
	if err != nil || !expiresAt.After(time.Now().UTC()) {
		return CredentialDelivery{}, errors.New("credential delivery is expired or has an invalid expiry")
	}

	return delivery, nil
}

func (c *Client) AcknowledgeCredentialRotation(ctx context.Context, deliveryID string, expectedAgentID string, expectedCredentialID string) (CredentialAcknowledgement, error) {
	if strings.TrimSpace(c.Token) == "" {
		return CredentialAcknowledgement{}, errors.New("new Agent credential is not configured")
	}

	endpoint, err := c.credentialDeliveryURL(deliveryID, "/ack")
	if err != nil {
		return CredentialAcknowledgement{}, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBufferString("{}"))
	if err != nil {
		return CredentialAcknowledgement{}, fmt.Errorf("create credential rotation acknowledgement: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+c.Token)

	response, err := c.HTTP.Do(request)
	if err != nil {
		return CredentialAcknowledgement{}, fmt.Errorf("send credential rotation acknowledgement: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxCredentialResponse+1))
	if err != nil {
		return CredentialAcknowledgement{}, fmt.Errorf("read credential rotation acknowledgement: %w", err)
	}
	if len(body) > maxCredentialResponse {
		return CredentialAcknowledgement{}, errors.New("credential acknowledgement response exceeds the allowed size")
	}
	if response.StatusCode != http.StatusOK {
		return CredentialAcknowledgement{}, fmt.Errorf("Central credential rotation acknowledgement returned HTTP %d", response.StatusCode)
	}

	var envelope credentialAcknowledgementEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return CredentialAcknowledgement{}, errors.New("Central credential acknowledgement response is malformed")
	}
	if !envelope.Success {
		return CredentialAcknowledgement{}, errors.New("Central did not confirm credential rotation acknowledgement")
	}

	keyVersion, err := parseKeyVersion(envelope.Data.KeyVersion)
	if err != nil {
		return CredentialAcknowledgement{}, err
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, envelope.Data.ExpiresAt)
	if err != nil || !expiresAt.After(time.Now().UTC()) {
		return CredentialAcknowledgement{}, errors.New("credential acknowledgement has an invalid or expired credential expiry")
	}
	acknowledgement := CredentialAcknowledgement{
		AgentID:      envelope.Data.AgentID,
		CredentialID: envelope.Data.CredentialID,
		KeyVersion:   keyVersion,
		Status:       envelope.Data.Status,
		ExpiresAt:    expiresAt.UTC(),
	}
	if acknowledgement.AgentID != expectedAgentID {
		return CredentialAcknowledgement{}, errors.New("credential acknowledgement Agent identity mismatch")
	}
	if acknowledgement.CredentialID != expectedCredentialID {
		return CredentialAcknowledgement{}, errors.New("credential acknowledgement credential identity mismatch")
	}
	if acknowledgement.KeyVersion < 1 || !strings.EqualFold(acknowledgement.Status, "active") {
		return CredentialAcknowledgement{}, errors.New("credential acknowledgement did not confirm an active credential version")
	}

	return acknowledgement, nil
}

func (c *Client) credentialDeliveryURL(deliveryID string, suffix string) (string, error) {
	base, err := url.Parse(c.BaseURL)
	if err != nil || base.Scheme != "https" || base.Host == "" || base.User != nil || base.Fragment != "" {
		return "", errors.New("configured Central origin is invalid for credential delivery")
	}

	endpoint := strings.TrimRight(c.BaseURL, "/") + "/api/v1/nadi/agent/credentials/deliveries/" + url.PathEscape(deliveryID) + suffix
	target, err := url.Parse(endpoint)
	if err != nil || target.User != nil || target.Fragment != "" || !strings.EqualFold(target.Scheme, base.Scheme) || !strings.EqualFold(target.Host, base.Host) {
		return "", errors.New("credential delivery URL origin validation failed")
	}

	return target.String(), nil
}

func parseKeyVersion(raw json.RawMessage) (int, error) {
	if len(raw) == 0 {
		return 0, errors.New("credential acknowledgement is missing key_version")
	}

	var textValue string
	if raw[0] == '"' {
		if err := json.Unmarshal(raw, &textValue); err != nil {
			return 0, errors.New("credential acknowledgement key_version is invalid")
		}
	} else {
		textValue = string(raw)
	}

	version, err := strconv.Atoi(textValue)
	if err != nil || version < 1 {
		return 0, errors.New("credential acknowledgement key_version is invalid")
	}

	return version, nil
}
