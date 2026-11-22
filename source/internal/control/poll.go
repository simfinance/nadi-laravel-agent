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

const (
	PollPath    = "/api/v1/nadi/agent/control/poll"
	maxPollBody = 4 << 20
)

func (c *Client) Poll(ctx context.Context) (PollData, error) {
	if strings.TrimSpace(c.Token) == "" {
		return PollData{}, errors.New("agent bearer credential is not configured")
	}

	requestBody := bytes.NewBufferString("{}")
	pollURL := strings.TrimRight(c.BaseURL, "/") + PollPath
	if strings.TrimSpace(c.PollURL) != "" {
		pollURL = c.PollURL
	}

	if err := validatePollURL(c.BaseURL, pollURL); err != nil {
		return PollData{}, err
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		pollURL,
		requestBody,
	)
	if err != nil {
		return PollData{}, fmt.Errorf("create Control Poll request: %w", err)
	}

	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+c.Token)

	response, err := c.HTTP.Do(request)
	if err != nil {
		return PollData{}, fmt.Errorf("send Control Poll request: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxPollBody+1))
	if err != nil {
		return PollData{}, fmt.Errorf("read Control Poll response: %w", err)
	}

	if len(body) > maxPollBody {
		return PollData{}, fmt.Errorf("Control Poll response exceeds %d bytes", maxPollBody)
	}

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		message := strings.TrimSpace(string(body))
		if len(message) > 512 {
			message = message[:512]
		}
		return PollData{}, fmt.Errorf("Central Control Poll returned HTTP %d: %s", response.StatusCode, message)
	}

	var envelope PollEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return PollData{}, fmt.Errorf("decode Control Poll response: %w", err)
	}

	if !envelope.Success {
		message := strings.TrimSpace(envelope.Message)
		if message == "" {
			message = "Central rejected the Control Poll request"
		}
		return PollData{}, errors.New(message)
	}

	if err := ValidatePollData(envelope.Data); err != nil {
		return PollData{}, err
	}

	return envelope.Data, nil
}

func ValidatePollData(data PollData) error {
	if strings.TrimSpace(data.Agent.ID) == "" {
		return errors.New("Control Poll response is missing data.agent.id")
	}

	for index, application := range data.Applications {
		if strings.TrimSpace(application.ApplicationID) == "" {
			return fmt.Errorf("Control Poll application at index %d has an empty application_id", index)
		}

		if application.ActivationGrant == nil {
			continue
		}

		grant := application.ActivationGrant
		if grant.GrantID == "" || grant.GrantVersion < 1 || grant.KeyID == "" || grant.Signature == "" {
			return fmt.Errorf("Control Poll application %q contains an incomplete activation grant", application.ApplicationID)
		}

		if len(grant.Payload) == 0 || !json.Valid(grant.Payload) {
			return fmt.Errorf("Control Poll application %q contains an invalid activation grant payload", application.ApplicationID)
		}
	}

	return nil
}

func validatePollURL(baseURL string, pollURL string) error {
	base, err := url.Parse(baseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return errors.New("configured Central base URL is invalid")
	}

	target, err := url.Parse(pollURL)
	if err != nil || !target.IsAbs() || target.Host == "" {
		return errors.New("Control Poll URL must be an absolute URL")
	}

	if target.User != nil || target.Fragment != "" {
		return errors.New("Control Poll URL must not contain user information or a fragment")
	}

	if !strings.EqualFold(target.Scheme, base.Scheme) || !strings.EqualFold(target.Host, base.Host) {
		return errors.New("Control Poll URL origin does not match the configured Central origin")
	}

	if target.Scheme != "https" && target.Scheme != "http" {
		return errors.New("Control Poll URL scheme is not supported")
	}

	return nil
}
