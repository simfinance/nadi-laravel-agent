package enrollment

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientPostsEnrollmentWithoutBearerCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", request.Method)
		}
		if request.URL.Path != EnrollPath {
			t.Errorf("unexpected Enrollment path: %s", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "" {
			t.Errorf("Enrollment request must not send an Agent Bearer credential")
		}
		if request.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected Content-Type: %s", request.Header.Get("Content-Type"))
		}

		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusCreated)
		fmt.Fprint(writer, `{"success":true,"message":"ok","data":{}}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, 2*time.Second)
	defer client.CloseIdleConnections()

	body, err := client.Enroll(context.Background(), []byte(`{"request_id":"test"}`))
	if err != nil {
		t.Fatalf("Enroll returned error: %v", err)
	}
	if !strings.Contains(string(body), `"success":true`) {
		t.Fatalf("unexpected response body: %s", body)
	}
}
