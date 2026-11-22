package appmonitor

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	CapabilityApplicationMetrics  = "application_metrics"
	CapabilityDatabaseMonitoring  = "database_monitoring"
	CapabilityQueueMonitoring     = "queue_monitoring"
	CapabilitySchedulerMonitoring = "scheduler_monitoring"
	BridgePrefix                  = "_nadi/v1"
	maximumResponseBytes          = 1024 * 1024
)

type Status string

const (
	StatusAvailable   Status = "available"
	StatusDegraded    Status = "degraded"
	StatusUnavailable Status = "unavailable"
	StatusUnsupported Status = "unsupported"
	StatusNotEnabled  Status = "not_enabled"
)

type EndpointResult struct {
	Status     Status          `json:"status"`
	HTTPStatus int             `json:"http_status,omitempty"`
	LatencyMS  float64         `json:"latency_ms,omitempty"`
	CheckedAt  time.Time       `json:"checked_at"`
	ErrorCode  string          `json:"error_code,omitempty"`
	Data       json.RawMessage `json:"data,omitempty"`
}

type ApplicationInfo struct {
	ApplicationName string `json:"application_name"`
	Environment     string `json:"environment"`
	LaravelVersion  string `json:"laravel_version"`
	PHPVersion      string `json:"php_version"`
}

type DatabaseInfo struct {
	Status    string  `json:"status"`
	Driver    string  `json:"driver,omitempty"`
	LatencyMS float64 `json:"latency_ms,omitempty"`
	CheckedAt string  `json:"checked_at,omitempty"`
	CacheHit  bool    `json:"cache_hit,omitempty"`
	ErrorCode string  `json:"error_code,omitempty"`
}

type Snapshot struct {
	Timestamp time.Time      `json:"timestamp"`
	Health    EndpointResult `json:"health"`
	Info      EndpointResult `json:"info"`
	Metrics   EndpointResult `json:"metrics"`
	Database  EndpointResult `json:"database"`
	Queue     EndpointResult `json:"queue"`
	Scheduler EndpointResult `json:"scheduler"`
}

type Collector struct {
	client *http.Client
}

func NewCollector(timeout time.Duration) *Collector {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	return &Collector{
		client: &http.Client{
			Transport: transport,
			Timeout:   timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (c *Collector) Collect(
	ctx context.Context,
	baseURL string,
	credential string,
	capabilities []string,
	now time.Time,
) Snapshot {
	snapshot := Snapshot{
		Timestamp: now.UTC(),
		Health:    notEnabled(now),
		Info:      notEnabled(now),
		Metrics:   notEnabled(now),
		Database:  notEnabled(now),
		Queue:     notEnabled(now),
		Scheduler: notEnabled(now),
	}
	if hasCapability(capabilities, CapabilityQueueMonitoring) {
		snapshot.Queue = unsupported(now, "bridge_queue_endpoint_returns_501")
	}
	if hasCapability(capabilities, CapabilitySchedulerMonitoring) {
		snapshot.Scheduler = unsupported(now, "bridge_scheduler_endpoint_returns_501")
	}

	if !hasCapability(capabilities, CapabilityApplicationMetrics) &&
		!hasCapability(capabilities, CapabilityDatabaseMonitoring) &&
		!hasCapability(capabilities, CapabilityQueueMonitoring) &&
		!hasCapability(capabilities, CapabilitySchedulerMonitoring) {
		return snapshot
	}

	if _, err := validateBaseURL(baseURL); err != nil {
		failure := endpointFailure(now, "invalid_application_endpoint")
		if hasCapability(capabilities, CapabilityApplicationMetrics) {
			snapshot.Health = failure
			snapshot.Info = failure
			snapshot.Metrics = failure
		}
		if hasCapability(capabilities, CapabilityDatabaseMonitoring) {
			snapshot.Database = failure
		}
		return snapshot
	}

	if strings.TrimSpace(credential) == "" {
		failure := endpointFailure(now, "application_credential_unavailable")
		if hasCapability(capabilities, CapabilityApplicationMetrics) {
			snapshot.Health = failure
			snapshot.Info = failure
			snapshot.Metrics = failure
		}
		if hasCapability(capabilities, CapabilityDatabaseMonitoring) {
			snapshot.Database = failure
		}
		return snapshot
	}

	if hasCapability(capabilities, CapabilityApplicationMetrics) {
		snapshot.Health = c.get(ctx, baseURL, credential, "health", now)
		snapshot.Info = c.get(ctx, baseURL, credential, "info", now)
		snapshot.Metrics = unsupported(now, "bridge_metrics_endpoint_returns_501")
	}
	if hasCapability(capabilities, CapabilityDatabaseMonitoring) {
		snapshot.Database = c.get(ctx, baseURL, credential, "database", now)
	}
	return snapshot
}

func (c *Collector) get(
	ctx context.Context,
	baseURL string,
	credential string,
	endpoint string,
	now time.Time,
) EndpointResult {
	startedAt := time.Now()
	checkedAt := now.UTC()

	requestURL, err := endpointURL(baseURL, endpoint)
	if err != nil {
		return endpointFailure(checkedAt, "invalid_application_endpoint")
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return endpointFailure(checkedAt, "request_creation_failed")
	}
	request.Header.Set("Authorization", "Bearer "+credential)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "NADI-Agent-Laravel-Monitor/1")

	response, err := c.client.Do(request)
	if err != nil {
		return EndpointResult{
			Status:    StatusUnavailable,
			CheckedAt: checkedAt,
			ErrorCode: classifyRequestError(err),
			LatencyMS: elapsedMilliseconds(startedAt),
		}
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maximumResponseBytes+1))
	if err != nil || len(body) > maximumResponseBytes {
		return EndpointResult{
			Status:     StatusUnavailable,
			HTTPStatus: response.StatusCode,
			CheckedAt:  checkedAt,
			LatencyMS:  elapsedMilliseconds(startedAt),
			ErrorCode:  "invalid_or_oversized_response",
		}
	}

	var envelope struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
		Error   struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return EndpointResult{
			Status:     StatusUnavailable,
			HTTPStatus: response.StatusCode,
			CheckedAt:  checkedAt,
			LatencyMS:  elapsedMilliseconds(startedAt),
			ErrorCode:  "invalid_json_response",
		}
	}

	result := EndpointResult{
		HTTPStatus: response.StatusCode,
		CheckedAt:  checkedAt,
		LatencyMS:  elapsedMilliseconds(startedAt),
		Data:       append(json.RawMessage(nil), envelope.Data...),
	}

	if response.StatusCode >= 200 && response.StatusCode < 300 && envelope.Success {
		result.Status = StatusAvailable
		return result
	}

	if response.StatusCode == http.StatusServiceUnavailable && endpoint == "database" && len(envelope.Data) > 0 {
		result.Status = StatusDegraded
		var database DatabaseInfo
		if json.Unmarshal(envelope.Data, &database) == nil && database.ErrorCode != "" {
			result.ErrorCode = database.ErrorCode
		}
		return result
	}

	result.Status = StatusUnavailable
	result.ErrorCode = envelope.Error.Code
	if result.ErrorCode == "" {
		result.ErrorCode = fmt.Sprintf("http_status_%d", response.StatusCode)
	}
	return result
}

func validateBaseURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return nil, errors.New("application endpoint must be an absolute HTTPS URL without userinfo or fragment")
	}
	return parsed, nil
}

func endpointURL(baseURL string, endpoint string) (string, error) {
	parsed, err := validateBaseURL(baseURL)
	if err != nil {
		return "", err
	}
	if endpoint == "" || strings.Contains(endpoint, "/") || endpoint == "." || endpoint == ".." {
		return "", errors.New("invalid Bridge endpoint")
	}
	basePath := strings.TrimRight(parsed.Path, "/")
	if !strings.HasSuffix(basePath, "/"+BridgePrefix) {
		basePath += "/" + BridgePrefix
	}
	parsed.Path = basePath + "/" + endpoint
	parsed.RawPath = ""
	parsed.RawQuery = ""
	return parsed.String(), nil
}

func hasCapability(capabilities []string, required string) bool {
	for _, capability := range capabilities {
		if capability == required {
			return true
		}
	}
	return false
}

func notEnabled(now time.Time) EndpointResult {
	return EndpointResult{Status: StatusNotEnabled, CheckedAt: now.UTC()}
}

func unsupported(now time.Time, code string) EndpointResult {
	return EndpointResult{Status: StatusUnsupported, CheckedAt: now.UTC(), ErrorCode: code}
}

func endpointFailure(now time.Time, code string) EndpointResult {
	return EndpointResult{Status: StatusUnavailable, CheckedAt: now.UTC(), ErrorCode: code}
}

func classifyRequestError(err error) string {
	var urlError *url.Error
	if errors.As(err, &urlError) {
		if strings.Contains(strings.ToLower(urlError.Err.Error()), "certificate") ||
			strings.Contains(strings.ToLower(urlError.Err.Error()), "tls") {
			return "tls_verification_failed"
		}
		if strings.Contains(strings.ToLower(urlError.Err.Error()), "redirect") {
			return "redirect_rejected"
		}
	}
	return "request_failed"
}

func elapsedMilliseconds(startedAt time.Time) float64 {
	return float64(time.Since(startedAt).Microseconds()) / 1000
}
