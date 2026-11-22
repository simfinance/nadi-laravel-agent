package activation

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"nadi-agent/internal/control"
	"nadi-agent/internal/state"
)

func TestCanonicalJSONSortsObjectKeysAndPreservesArrayOrder(t *testing.T) {
	actual, err := CanonicalJSON([]byte(`{"z":1,"a":{"y":2,"x":3},"items":[2,1]}`))
	if err != nil {
		t.Fatal(err)
	}

	expected := `{"a":{"x":3,"y":2},"items":[2,1],"z":1}`
	if string(actual) != expected {
		t.Fatalf("unexpected canonical JSON\nexpected: %s\nactual:   %s", expected, actual)
	}
}

func TestVerifierAcceptsValidEd25519Grant(t *testing.T) {
	verifier, grant, application, agentState, now := createSignedGrant(t)

	if _, err := verifier.Verify(grant, application, agentState, now); err != nil {
		t.Fatalf("expected valid grant, got: %v", err)
	}
}

func TestVerifierRejectsExpiredGrant(t *testing.T) {
	verifier, grant, application, agentState, now := createSignedGrant(t)
	now = now.Add(48 * time.Hour)

	if _, err := verifier.Verify(grant, application, agentState, now); err == nil {
		t.Fatal("expected expired grant to be rejected")
	}
}

func TestVerifierAcceptsEnvelopeExpiryTruncatedToSeconds(t *testing.T) {
	verifier, grant, application, agentState, now := createSignedGrant(t)
	grant.ExpiresAt = now.Add(24 * time.Hour).Format(time.RFC3339)

	if _, err := verifier.Verify(grant, application, agentState, now); err != nil {
		t.Fatalf("expected second-precision envelope expiry to be accepted, got: %v", err)
	}
}

func TestVerifierAcceptsCentralEnvelopeSerializedWithJakartaOffset(t *testing.T) {
	verifier, grant, application, agentState, now := createSignedGrant(t)
	grant.ExpiresAt = now.Add(24 * time.Hour).Format("2006-01-02T15:04:05Z")
	parsed, err := time.Parse(time.RFC3339Nano, grant.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	grant.ExpiresAt = parsed.Add(-7 * time.Hour).Format(time.RFC3339)

	if _, err := verifier.Verify(grant, application, agentState, now); err != nil {
		t.Fatalf("expected signed payload expiry to remain authoritative, got: %v", err)
	}
}

func TestVerifierAcceptsCentralSQLDateTimeEnvelopeExpiry(t *testing.T) {
	verifier, grant, application, agentState, now := createSignedGrant(t)
	grant.ExpiresAt = now.Add(24 * time.Hour).Format("2006-01-02 15:04:05.000000")

	if _, err := verifier.Verify(grant, application, agentState, now); err != nil {
		t.Fatalf("expected Central UTC SQL datetime expiry to be accepted, got: %v", err)
	}
}

func TestVerifierAcceptsCentralJakartaOffsetForUTCPayload(t *testing.T) {
	verifier, grant, application, agentState, now := createSignedGrant(t)
	grant.ExpiresAt = now.Add(24 * time.Hour).Format("2006-01-02T15:04:05+07:00")

	if _, err := verifier.Verify(grant, application, agentState, now); err != nil {
		t.Fatalf("expected Central UTC wall-clock timestamp with Jakarta offset to be accepted, got: %v", err)
	}
}

func TestVerifierIgnoresUnsignedEnvelopeExpiryMismatch(t *testing.T) {
	verifier, grant, application, agentState, now := createSignedGrant(t)
	grant.ExpiresAt = now.Add(24*time.Hour + 7*time.Hour).Format(time.RFC3339)

	if _, err := verifier.Verify(grant, application, agentState, now); err != nil {
		t.Fatalf("expected valid signed payload to remain authoritative when unsigned envelope expiry differs, got: %v", err)
	}
}

func TestVerifierRejectsApplicationIdentityMismatch(t *testing.T) {
	verifier, grant, application, agentState, now := createSignedGrant(t)
	application.ApplicationID = "different-application"

	if _, err := verifier.Verify(grant, application, agentState, now); err == nil {
		t.Fatal("expected application identity mismatch to be rejected")
	}
}

func createSignedGrant(t *testing.T) (*Verifier, control.ActivationGrant, control.Application, state.State, time.Time) {
	t.Helper()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	publicKeyDER, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		t.Fatal(err)
	}

	publicKeyPath := filepath.Join(t.TempDir(), "activation-public.pem")
	if err := os.WriteFile(publicKeyPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicKeyDER}), 0600); err != nil {
		t.Fatal(err)
	}

	verifier, err := NewVerifier(publicKeyPath, "nadi-central-2026-01")
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	payload := map[string]any{
		"grant_id":           "grant-123",
		"grant_version":      1,
		"key_id":             "nadi-central-2026-01",
		"registration_id":    "registration-123",
		"application_id":     "application-123",
		"agent_id":           "agent-123",
		"server_id":          "server-123",
		"issued_at":          now.Format(time.RFC3339),
		"expires_at":         now.Add(24*time.Hour + 585134*time.Microsecond).Format(time.RFC3339Nano),
		"capabilities":       []string{"server_metrics", "application_metrics"},
		"monitoring_enabled": true,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := CanonicalJSON(payloadBytes)
	if err != nil {
		t.Fatal(err)
	}

	signature := ed25519.Sign(privateKey, canonical)
	grant := control.ActivationGrant{
		GrantID:      "grant-123",
		GrantVersion: 1,
		KeyID:        "nadi-central-2026-01",
		Payload:      payloadBytes,
		Signature:    base64.StdEncoding.EncodeToString(signature),
		ExpiresAt:    now.Add(24 * time.Hour).Format(time.RFC3339),
	}

	application := control.Application{ApplicationID: "application-123", MonitoringEnabled: true}
	agentState := state.State{
		CentralAgentID:  "agent-123",
		CentralServerID: "server-123",
		Applications: []state.EnrolledApplication{{
			ApplicationID:  "application-123",
			RegistrationID: "registration-123",
		}},
	}

	return verifier, grant, application, agentState, now
}
