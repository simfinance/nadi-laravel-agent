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
	"strings"
)

const maxAcknowledgementResponse = 1 << 20

type AcknowledgementRequest struct {
	RequestID     string  `json:"request_id"`
	ApplicationID string  `json:"application_id"`
	Result        string  `json:"result"`
	ErrorCode     *string `json:"error_code"`
	Message       *string `json:"message"`
}

func (c *Client) Acknowledge(ctx context.Context, grantID string, payload AcknowledgementRequest) error {
	if strings.TrimSpace(c.Token) == "" {
		return errors.New("agent bearer credential is not configured")
	}
	if strings.TrimSpace(grantID) == "" {
		return errors.New("grant ID must not be empty")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode Activation Grant acknowledgement: %w", err)
	}

	endpoint := strings.TrimRight(c.BaseURL, "/") + "/api/v1/nadi/agent/activation-grants/" + url.PathEscape(grantID) + "/ack"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create Activation Grant acknowledgement request: %w", err)
	}

	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+c.Token)

	response, err := c.HTTP.Do(request)
	if err != nil {
		return fmt.Errorf("send Activation Grant acknowledgement: %w", err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxAcknowledgementResponse+1))
	if err != nil {
		return fmt.Errorf("read Activation Grant acknowledgement response: %w", err)
	}
	if len(responseBody) > maxAcknowledgementResponse {
		return fmt.Errorf("Activation Grant acknowledgement response exceeds %d bytes", maxAcknowledgementResponse)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		message := strings.TrimSpace(string(responseBody))
		if len(message) > 512 {
			message = message[:512]
		}
		return fmt.Errorf("Central Activation Grant acknowledgement returned HTTP %d: %s", response.StatusCode, message)
	}

	var envelope struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		return fmt.Errorf("decode Activation Grant acknowledgement response: %w", err)
	}
	if !envelope.Success {
		if strings.TrimSpace(envelope.Message) == "" {
			envelope.Message = "Central rejected the Activation Grant acknowledgement"
		}
		return errors.New(envelope.Message)
	}

	return nil
}
