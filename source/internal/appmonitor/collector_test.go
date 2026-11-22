package appmonitor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCollectUsesOnlyGrantedCapabilities(t *testing.T) {
	calls := make([]string, 0)
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls = append(calls, request.URL.Path)
		if request.Header.Get("Authorization") != "Bearer app-secret" {
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/_nadi/v1/health":
			writer.Write([]byte(`{"success":true,"data":{"status":"ok","timestamp":"2026-11-21T00:00:00Z"}}`))
		case "/_nadi/v1/info":
			writer.Write([]byte(`{"success":true,"data":{"application_name":"Portal","environment":"production","laravel_version":"12.0","php_version":"8.4"}}`))
		case "/_nadi/v1/database":
			writer.WriteHeader(http.StatusServiceUnavailable)
			writer.Write([]byte(`{"success":false,"data":{"status":"failed","error_code":"database_connection_failed","latency_ms":12.4,"checked_at":"2026-11-21T00:00:00Z"}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	collector := &Collector{client: server.Client()}
	now := time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)
	snapshot := collector.Collect(
		context.Background(),
		server.URL+"/_nadi/v1",
		"app-secret",
		[]string{CapabilityDatabaseMonitoring},
		now,
	)

	if snapshot.Database.Status != StatusDegraded {
		t.Fatalf("expected degraded database status, got %q", snapshot.Database.Status)
	}
	if snapshot.Health.Status != StatusNotEnabled || snapshot.Info.Status != StatusNotEnabled {
		t.Fatal("application metrics endpoints were called without application_metrics capability")
	}
	if len(calls) != 1 || calls[0] != "/_nadi/v1/database" {
		t.Fatalf("unexpected endpoint calls: %#v", calls)
	}
}

func TestCollectApplicationMetricsAndUnsupportedQueueScheduler(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer app-secret" {
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(request.URL.Path, "/health") {
			writer.Write([]byte(`{"success":true,"data":{"status":"ok"}}`))
			return
		}
		if strings.HasSuffix(request.URL.Path, "/info") {
			writer.Write([]byte(`{"success":true,"data":{"application_name":"Portal","environment":"production","laravel_version":"12.0","php_version":"8.4"}}`))
			return
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()

	collector := &Collector{client: server.Client()}
	snapshot := collector.Collect(
		context.Background(),
		server.URL,
		"app-secret",
		[]string{CapabilityApplicationMetrics, CapabilityQueueMonitoring, CapabilitySchedulerMonitoring},
		time.Now().UTC(),
	)
	if snapshot.Health.Status != StatusAvailable || snapshot.Info.Status != StatusAvailable {
		t.Fatalf("expected health and info to be available: %#v %#v", snapshot.Health, snapshot.Info)
	}
	if snapshot.Queue.Status != StatusUnsupported || snapshot.Scheduler.Status != StatusUnsupported {
		t.Fatal("queue and scheduler must be reported unsupported until Bridge implements them")
	}

	var info ApplicationInfo
	if err := json.Unmarshal(snapshot.Info.Data, &info); err != nil {
		t.Fatalf("decode application info: %v", err)
	}
	if info.LaravelVersion != "12.0" || info.PHPVersion != "8.4" {
		t.Fatalf("unexpected application info: %#v", info)
	}
}

func TestEndpointURLDoesNotDuplicateBridgePrefix(t *testing.T) {
	got, err := endpointURL("https://app.example.test/_nadi/v1", "health")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://app.example.test/_nadi/v1/health" {
		t.Fatalf("unexpected endpoint URL: %s", got)
	}
}

func TestValidateBaseURLRejectsNonHTTPSAndUserInfo(t *testing.T) {
	for _, value := range []string{
		"http://app.example.test",
		"https://user:pass@app.example.test",
		"https://app.example.test/#fragment",
	} {
		if _, err := validateBaseURL(value); err == nil {
			t.Fatalf("expected URL to be rejected: %s", value)
		}
	}
}

func TestCollectDoesNotFollowRedirects(t *testing.T) {
	redirected := false
	destination := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		redirected = true
		writer.Write([]byte(`{"success":true,"data":{}}`))
	}))
	defer destination.Close()

	source := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, destination.URL+"/_nadi/v1/health", http.StatusFound)
	}))
	defer source.Close()

	client := source.Client()
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	collector := &Collector{client: client}
	snapshot := collector.Collect(
		context.Background(),
		source.URL,
		"app-secret",
		[]string{CapabilityApplicationMetrics},
		time.Now().UTC(),
	)
	if snapshot.Health.Status != StatusUnavailable || snapshot.Health.HTTPStatus != http.StatusFound {
		t.Fatalf("expected redirect to be rejected: %#v", snapshot.Health)
	}
	if redirected {
		t.Fatal("collector followed an application endpoint redirect")
	}
}
