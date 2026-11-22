package telemetry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSendPersistsBatchUntilLocalAcknowledgementCompletes(t *testing.T) {
	var received []Batch
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", request.Method)
		}
		if request.URL.Path != EndpointPath {
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer credential" {
			t.Fatal("Agent credential header was not set")
		}
		if request.Header.Get("X-NADI-Agent-ID") != "agent-1" {
			t.Fatal("Agent ID header was not set")
		}
		if request.Header.Get("X-NADI-Request-ID") == "" {
			t.Fatal("request ID header was not set")
		}

		var batch Batch
		if err := json.NewDecoder(request.Body).Decode(&batch); err != nil {
			t.Fatalf("decode batch: %v", err)
		}
		received = append(received, batch)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"success":true,"data":{"success":true,"batch_id":"` + batch.BatchID + `","status":"accepted","record_count":1,"accepted_count":1,"duplicate_count":0,"rejected_count":0,"replayed":false}}`))
	}))
	defer server.Close()

	directory := t.TempDir()
	sender, err := NewSender(server.URL, directory, time.Second)
	if err != nil {
		t.Fatalf("create sender: %v", err)
	}
	sender.client = server.Client()
	applicationID := "11111111-1111-4111-8111-111111111111"
	inputs := []InputRecord{{
		Stream:        "application_metrics",
		ObservedAt:    time.Now().UTC(),
		CollectedAt:   time.Now().UTC(),
		ApplicationID: &applicationID,
		Data:          json.RawMessage(`{"status":"available"}`),
		Source: Source{
			Kind:          "application",
			ApplicationID: applicationID,
			EventID:       "source-event-1",
		},
	}}

	acknowledged, err := sender.Send(context.Background(), "agent-1", "22222222-2222-4222-8222-222222222222", "credential", inputs)
	if err != nil {
		t.Fatalf("send batch: %v", err)
	}
	if len(acknowledged) != 1 || acknowledged[0].EventID != "source-event-1" {
		t.Fatalf("unexpected acknowledged sources: %#v", acknowledged)
	}
	if _, err := os.Stat(filepath.Join(directory, "pending-batch.json")); err != nil {
		t.Fatalf("pending batch removed before local spool acknowledgement: %v", err)
	}

	_, err = sender.Send(context.Background(), "agent-1", "22222222-2222-4222-8222-222222222222", "credential", nil)
	if err != nil {
		t.Fatalf("replay pending batch: %v", err)
	}
	if len(received) != 2 || received[0].BatchID != received[1].BatchID || received[0].Records[0].RecordID != received[1].Records[0].RecordID {
		t.Fatal("retry did not preserve batch and record identifiers")
	}
	if received[0].Records[0].Sequence != 1 {
		t.Fatalf("unexpected first sequence: %d", received[0].Records[0].Sequence)
	}

	if err := sender.Complete(); err != nil {
		t.Fatalf("complete batch: %v", err)
	}
	if _, err := os.Stat(filepath.Join(directory, "pending-batch.json")); !os.IsNotExist(err) {
		t.Fatalf("pending batch still exists after completion: %v", err)
	}
}

func TestBuildPendingUsesIndependentPersistentSequences(t *testing.T) {
	sender, err := NewSender("https://central.example", t.TempDir(), time.Second)
	if err != nil {
		t.Fatalf("create sender: %v", err)
	}
	applicationID := "11111111-1111-4111-8111-111111111111"
	input := InputRecord{
		Stream:        "application_metrics",
		ObservedAt:    time.Now().UTC(),
		CollectedAt:   time.Now().UTC(),
		ApplicationID: &applicationID,
		Data:          json.RawMessage(`{}`),
	}

	first, err := sender.buildPending("22222222-2222-4222-8222-222222222222", []InputRecord{input})
	if err != nil {
		t.Fatalf("build first batch: %v", err)
	}
	if first.Batch.Records[0].Sequence != 1 {
		t.Fatalf("expected first sequence 1, got %d", first.Batch.Records[0].Sequence)
	}
	second, err := sender.buildPending("22222222-2222-4222-8222-222222222222", []InputRecord{input})
	if err != nil {
		t.Fatalf("build second batch: %v", err)
	}
	if second.Batch.Records[0].Sequence != 2 {
		t.Fatalf("expected second sequence 2, got %d", second.Batch.Records[0].Sequence)
	}
}

func TestBuildPendingRejectsApplicationStreamWithoutApplicationID(t *testing.T) {
	sender, err := NewSender("https://central.example", t.TempDir(), time.Second)
	if err != nil {
		t.Fatalf("create sender: %v", err)
	}
	_, err = sender.buildPending("22222222-2222-4222-8222-222222222222", []InputRecord{{
		Stream: "application_metrics",
		Data:   json.RawMessage(`{}`),
	}})
	if err == nil {
		t.Fatal("expected application_id validation error")
	}
}

func TestBuildPendingAllowsOptionalApplicationScopeForNginxVhost(t *testing.T) {
	sender, err := NewSender("https://central.example", t.TempDir(), time.Second)
	if err != nil {
		t.Fatalf("create sender: %v", err)
	}
	applicationID := "11111111-1111-4111-8111-111111111111"
	pending, err := sender.buildPending("22222222-2222-4222-8222-222222222222", []InputRecord{{
		Stream:        "nginx_vhost_metrics",
		ApplicationID: &applicationID,
		Data:          json.RawMessage(`{"requests":1}`),
	}})
	if err != nil {
		t.Fatalf("build Nginx vhost telemetry: %v", err)
	}
	if pending.Batch.Records[0].ApplicationID == nil || *pending.Batch.Records[0].ApplicationID != applicationID {
		t.Fatal("Nginx vhost application scope was not preserved")
	}
}

func TestBuildPendingRejectsUnsupportedStream(t *testing.T) {
	sender, err := NewSender("https://central.example", t.TempDir(), time.Second)
	if err != nil {
		t.Fatalf("create sender: %v", err)
	}
	_, err = sender.buildPending("22222222-2222-4222-8222-222222222222", []InputRecord{{
		Stream: "invented_stream",
		Data:   json.RawMessage(`{}`),
	}})
	if err == nil {
		t.Fatal("expected unsupported stream validation error")
	}
}

func TestBuildPendingRejectsServerMetricsWithApplicationID(t *testing.T) {
	sender, err := NewSender("https://central.example", t.TempDir(), time.Second)
	if err != nil {
		t.Fatalf("create sender: %v", err)
	}
	applicationID := "11111111-1111-4111-8111-111111111111"
	_, err = sender.buildPending("22222222-2222-4222-8222-222222222222", []InputRecord{{
		Stream:        "server_metrics",
		ApplicationID: &applicationID,
		Data:          json.RawMessage(`{}`),
	}})
	if err == nil {
		t.Fatal("expected server_metrics scope validation error")
	}
}
