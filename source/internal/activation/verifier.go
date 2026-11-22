package activation

import (
	"bytes"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"nadi-agent/internal/control"
	"nadi-agent/internal/state"
)

const maximumClockSkew = 5 * time.Minute

type Payload struct {
	GrantID           string   `json:"grant_id"`
	GrantVersion      int      `json:"grant_version"`
	KeyID             string   `json:"key_id"`
	RegistrationID    string   `json:"registration_id"`
	ApplicationID     string   `json:"application_id"`
	AgentID           string   `json:"agent_id"`
	ServerID          string   `json:"server_id"`
	IssuedAt          string   `json:"issued_at"`
	ExpiresAt         string   `json:"expires_at"`
	Capabilities      []string `json:"capabilities"`
	MonitoringEnabled bool     `json:"monitoring_enabled"`
}

type Verifier struct {
	publicKey ed25519.PublicKey
	keyID     string
}

func NewVerifier(publicKeyPath string, keyID string) (*Verifier, error) {
	if strings.TrimSpace(keyID) == "" {
		return nil, errors.New("activation key ID must not be empty")
	}

	content, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read Activation Grant public key: %w", err)
	}

	publicKey, err := parsePublicKey(content)
	if err != nil {
		return nil, err
	}

	return &Verifier{publicKey: publicKey, keyID: keyID}, nil
}

func parsePublicKey(content []byte) (ed25519.PublicKey, error) {
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == ed25519.PublicKeySize {
		return append(ed25519.PublicKey(nil), trimmed...), nil
	}

	block, _ := pem.Decode(trimmed)
	if block == nil {
		return nil, errors.New("Activation Grant public key is not valid PEM or raw Ed25519 key")
	}

	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse Activation Grant Ed25519 public key: %w", err)
	}

	publicKey, ok := parsed.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("Activation Grant public key has type %T; Ed25519 is required", parsed)
	}

	return publicKey, nil
}

func (v *Verifier) Verify(grant control.ActivationGrant, application control.Application, agentState state.State, now time.Time) (Payload, error) {
	var payload Payload
	if err := json.Unmarshal(grant.Payload, &payload); err != nil {
		return payload, fmt.Errorf("decode signed Activation Grant payload: %w", err)
	}

	if payload.GrantID == "" || payload.GrantID != grant.GrantID {
		return payload, errors.New("Activation Grant payload grant_id does not match the envelope")
	}
	if payload.GrantVersion < 1 || payload.GrantVersion != grant.GrantVersion {
		return payload, errors.New("Activation Grant payload grant_version does not match the envelope")
	}
	if payload.KeyID == "" || payload.KeyID != grant.KeyID || payload.KeyID != v.keyID {
		return payload, errors.New("Activation Grant key_id does not match the configured key ID")
	}
	if payload.RegistrationID == "" || payload.ApplicationID != application.ApplicationID || payload.RegistrationID != registrationID(agentState, application.ApplicationID) {
		return payload, errors.New("Activation Grant application or registration identity mismatch")
	}
	if payload.AgentID == "" || payload.AgentID != agentState.CentralAgentID {
		return payload, errors.New("Activation Grant agent_id does not match this Agent")
	}
	if payload.ServerID == "" || payload.ServerID != agentState.CentralServerID {
		return payload, errors.New("Activation Grant server_id does not match this server")
	}
	if !payload.MonitoringEnabled || !application.MonitoringEnabled {
		return payload, errors.New("Activation Grant does not enable monitoring in both signed payload and Control Poll response")
	}
	if len(payload.Capabilities) == 0 {
		return payload, errors.New("Activation Grant contains no capabilities")
	}

	issuedAt, err := time.Parse(time.RFC3339Nano, payload.IssuedAt)
	if err != nil {
		return payload, fmt.Errorf("Activation Grant issued_at is invalid: %w", err)
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, payload.ExpiresAt)
	if err != nil {
		return payload, fmt.Errorf("Activation Grant expires_at is invalid: %w", err)
	}
	if issuedAt.After(now.Add(maximumClockSkew)) {
		return payload, errors.New("Activation Grant issued_at is in the future")
	}
	if !expiresAt.After(now) {
		return payload, errors.New("Activation Grant has expired")
	}

	// The envelope expires_at is not part of the signed payload and Central may
	// serialize it using a different application timezone. The signed payload
	// is the authoritative expiry and has already been parsed and validated
	// above. Do not reject a cryptographically valid grant based on this
	// unsigned duplicate field.

	signature, err := base64.StdEncoding.DecodeString(grant.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return payload, errors.New("Activation Grant signature is not valid base64 Ed25519 signature data")
	}

	canonical, err := CanonicalJSON(grant.Payload)
	if err != nil {
		return payload, fmt.Errorf("canonicalize Activation Grant payload: %w", err)
	}
	if !ed25519.Verify(v.publicKey, canonical, signature) {
		return payload, errors.New("Activation Grant Ed25519 signature verification failed")
	}

	return payload, nil
}

func registrationID(agentState state.State, applicationID string) string {
	for _, application := range agentState.Applications {
		if application.ApplicationID == applicationID {
			return application.RegistrationID
		}
	}
	return ""
}

func CanonicalJSON(raw []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := ensureSingleJSONValue(decoder); err != nil {
		return nil, err
	}

	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}

	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n")), nil
}

func ensureSingleJSONValue(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("Activation Grant payload contains multiple JSON values")
		}
		return err
	}
	return nil
}
