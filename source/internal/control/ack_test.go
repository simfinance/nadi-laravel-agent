package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAcknowledgeSendsExpectedRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", request.Method)
		}
		if request.URL.Path != "/api/v1/nadi/agent/activation-grants/grant-123/ack" {
			t.Fatalf("unexpected ACK path: %s", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatal("missing bearer authorization")
		}

		var payload AcknowledgementRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.RequestID != "request-123" || payload.ApplicationID != "application-123" || payload.Result != "success" {
			t.Fatalf("unexpected acknowledgement payload: %+v", payload)
		}
		if payload.ErrorCode != nil || payload.Message != nil {
			t.Fatal("success acknowledgement must send null error_code and message")
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"success":true,"data":{"replayed":false}}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", 2*time.Second)
	defer client.HTTP.CloseIdleConnections()

	err := client.Acknowledge(context.Background(), "grant-123", AcknowledgementRequest{
		RequestID:     "request-123",
		ApplicationID: "application-123",
		Result:        "success",
	})
	if err != nil {
		t.Fatalf("expected ACK success, got: %v", err)
	}
}
