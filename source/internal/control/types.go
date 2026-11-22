package control

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type PollEnvelope struct {
	Success bool     `json:"success"`
	Message string   `json:"message,omitempty"`
	Data    PollData `json:"data"`
}

type PollData struct {
	Agent        AgentInfo     `json:"agent"`
	Control      ControlInfo   `json:"control"`
	Applications []Application `json:"applications"`
	Commands     []Command     `json:"commands"`
	ServerTime   string        `json:"server_time"`
}

type AgentInfo struct {
	ID                string `json:"id"`
	AgentInstanceID   string `json:"agent_instance_id"`
	Status            string `json:"status"`
	InstallationState string `json:"installation_state"`
	AgentVersion      string `json:"agent_version"`
	ProtocolVersion   string `json:"protocol_version"`
}

type ControlInfo struct {
	PollIntervalSeconds int `json:"poll_interval_seconds"`
}

type Application struct {
	ApplicationID      string           `json:"application_id"`
	Name               string           `json:"name"`
	RegistrationStatus string           `json:"registration_status"`
	ActivationStatus   string           `json:"activation_status"`
	MonitoringStatus   string           `json:"monitoring_status"`
	Endpoint           Endpoint         `json:"endpoint"`
	MonitoringEnabled  bool             `json:"monitoring_enabled"`
	ActivationGrant    *ActivationGrant `json:"activation_grant"`
}

type Endpoint struct {
	BaseURL string `json:"base_url"`
}

func (e *Endpoint) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		*e = Endpoint{}
		return nil
	}

	if trimmed[0] == '"' {
		var baseURL string
		if err := json.Unmarshal(trimmed, &baseURL); err != nil {
			return fmt.Errorf("decode endpoint URL string: %w", err)
		}

		e.BaseURL = baseURL
		return nil
	}

	type endpointAlias Endpoint
	var decoded endpointAlias
	if err := json.Unmarshal(trimmed, &decoded); err != nil {
		return fmt.Errorf("decode endpoint object: %w", err)
	}

	*e = Endpoint(decoded)
	return nil
}

type ActivationGrant struct {
	GrantID      string          `json:"grant_id"`
	GrantVersion int             `json:"grant_version"`
	KeyID        string          `json:"key_id"`
	Payload      json.RawMessage `json:"payload"`
	Signature    string          `json:"signature"`
	ExpiresAt    string          `json:"expires_at"`
}

type Command struct {
	CommandID     string          `json:"command_id"`
	ApplicationID string          `json:"application_id"`
	CommandType   string          `json:"command_type"`
	Payload       json.RawMessage `json:"payload"`
	IssuedAt      string          `json:"issued_at"`
	ExpiresAt     string          `json:"expires_at"`
}
