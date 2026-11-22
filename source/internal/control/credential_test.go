package control

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRetrieveCredentialDeliveryUsesOldCredentialAndValidatesIdentity(t *testing.T) {
	expiry := time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339Nano)
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", request.Method)
		}
		if request.Header.Get("Authorization") != "Bearer old-secret" {
			t.Fatal("retrieve request did not use the old credential")
		}
		if request.URL.Path != "/api/v1/nadi/agent/credentials/deliveries/delivery-1" {
			t.Fatalf("unexpected delivery path: %s", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(writer, `{"success":true,"message":"ok","data":{"delivery_id":"delivery-1","credential_id":"credential-2","token":"new-secret","expires_at":%q}}`, expiry)
	}))
	defer server.Close()

	client := NewClient(server.URL, "old-secret", time.Second)
	client.HTTP = server.Client()

	delivery, err := client.RetrieveCredentialDelivery(context.Background(), "delivery-1")
	if err != nil {
		t.Fatalf("retrieve delivery: %v", err)
	}
	if delivery.Token != "new-secret" || delivery.CredentialID != "credential-2" {
		t.Fatalf("unexpected delivery result: %+v", delivery)
	}
}

func TestAcknowledgeCredentialRotationUsesNewCredentialAndValidatesResponse(t *testing.T) {
	expiry := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339Nano)
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", request.Method)
		}
		if request.Header.Get("Authorization") != "Bearer new-secret" {
			t.Fatal("ACK request did not use the new credential")
		}
		if request.URL.Path != "/api/v1/nadi/agent/credentials/deliveries/delivery-1/ack" {
			t.Fatalf("unexpected ACK path: %s", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(writer, `{"success":true,"message":"Credential rotation acknowledged successfully.","data":{"agent_id":"agent-1","credential_id":"credential-2","key_version":"2","status":"active","expires_at":%q}}`, expiry)
	}))
	defer server.Close()

	client := NewClient(server.URL, "new-secret", time.Second)
	client.HTTP = server.Client()

	acknowledgement, err := client.AcknowledgeCredentialRotation(context.Background(), "delivery-1", "agent-1", "credential-2")
	if err != nil {
		t.Fatalf("acknowledge credential rotation: %v", err)
	}
	if acknowledgement.KeyVersion != 2 || acknowledgement.Status != "active" {
		t.Fatalf("unexpected ACK result: %+v", acknowledgement)
	}
}

func TestRetrieveCredentialDeliveryRejectsMismatchedDeliveryID(t *testing.T) {
	expiry := time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339Nano)
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		fmt.Fprintf(writer, `{"success":true,"data":{"delivery_id":"other-delivery","credential_id":"credential-2","token":"new-secret","expires_at":%q}}`, expiry)
	}))
	defer server.Close()

	client := NewClient(server.URL, "old-secret", time.Second)
	client.HTTP = server.Client()
	if _, err := client.RetrieveCredentialDelivery(context.Background(), "delivery-1"); err == nil {
		t.Fatal("expected mismatched delivery ID to be rejected")
	}
}
