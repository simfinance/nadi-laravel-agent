package telemetry

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	EndpointPath        = "/api/v1/nadi/agent/telemetry/batches"
	MaximumBatchBytes   = 1024 * 1024
	MaximumRecordBytes  = 64 * 1024
	MaximumBatchRecords = 500
)

var (
	uuidPattern    = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-8][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
	allowedStreams = map[string]struct{}{
		"server_metrics":            {},
		"nginx_metrics":             {},
		"nginx_vhost_metrics":       {},
		"nginx_access_logs":         {},
		"nginx_error_logs":          {},
		"application_metrics":       {},
		"application_logs":          {},
		"application_events":        {},
		"nginx_http_requests":       {},
		"application_http_requests": {},
		"application_http_errors":   {},
	}
)

type InputRecord struct {
	Stream        string          `json:"stream"`
	ObservedAt    time.Time       `json:"observed_at"`
	CollectedAt   time.Time       `json:"collected_at"`
	ApplicationID *string         `json:"application_id"`
	MetricSet     *string         `json:"metric_set"`
	Data          json.RawMessage `json:"data"`
	Source        Source          `json:"source"`
}

type Source struct {
	Kind          string `json:"kind"`
	ApplicationID string `json:"application_id,omitempty"`
	EventID       string `json:"event_id"`
}

type Record struct {
	RecordID      string          `json:"record_id"`
	Stream        string          `json:"stream"`
	Sequence      uint64          `json:"sequence"`
	ObservedAt    time.Time       `json:"observed_at"`
	CollectedAt   time.Time       `json:"collected_at"`
	ApplicationID *string         `json:"application_id"`
	MetricSet     *string         `json:"metric_set"`
	Data          json.RawMessage `json:"data"`
}

type Batch struct {
	ProtocolVersion string    `json:"protocol_version"`
	BatchID         string    `json:"batch_id"`
	AgentInstanceID string    `json:"agent_instance_id"`
	SentAt          time.Time `json:"sent_at"`
	Records         []Record  `json:"records"`
}

type pendingBatch struct {
	Batch   Batch    `json:"batch"`
	Sources []Source `json:"sources"`
}

type sequenceState struct {
	Values map[string]uint64 `json:"values"`
}

type Ack struct {
	Success bool `json:"success"`
	Data    struct {
		Success        bool   `json:"success"`
		BatchID        string `json:"batch_id"`
		Status         string `json:"status"`
		RecordCount    int    `json:"record_count"`
		AcceptedCount  int    `json:"accepted_count"`
		DuplicateCount int    `json:"duplicate_count"`
		RejectedCount  int    `json:"rejected_count"`
		Replayed       bool   `json:"replayed"`
	} `json:"data"`
}

type Sender struct {
	baseURL string
	client  *http.Client
	dir     string
}

func NewSender(baseURL string, directory string, timeout time.Duration) (*Sender, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return nil, errors.New("telemetry Central URL must be an absolute HTTPS URL without userinfo or fragment")
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, fmt.Errorf("create telemetry sender state directory: %w", err)
	}
	if err := os.Chmod(directory, 0700); err != nil {
		return nil, fmt.Errorf("secure telemetry sender state directory: %w", err)
	}

	return &Sender{
		baseURL: strings.TrimRight(baseURL, "/"),
		client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		dir: directory,
	}, nil
}

func (s *Sender) Send(ctx context.Context, agentID string, instanceID string, credential string, inputs []InputRecord) ([]Source, error) {
	if strings.TrimSpace(agentID) == "" || strings.TrimSpace(instanceID) == "" || strings.TrimSpace(credential) == "" {
		return nil, errors.New("Agent ID, Agent instance ID, and credential are required for telemetry delivery")
	}
	if !uuidPattern.MatchString(instanceID) {
		return nil, errors.New("Agent instance ID must be a UUID")
	}

	pending, err := s.loadPending()
	if err != nil {
		return nil, err
	}
	if pending == nil {
		if len(inputs) == 0 {
			return nil, nil
		}
		pending, err = s.buildPending(instanceID, inputs)
		if err != nil {
			return nil, err
		}
		if err := s.savePending(*pending); err != nil {
			return nil, err
		}
	}

	body, err := json.Marshal(pending.Batch)
	if err != nil {
		return nil, fmt.Errorf("encode pending telemetry batch: %w", err)
	}
	if len(body) > MaximumBatchBytes {
		return nil, fmt.Errorf("pending telemetry batch exceeds %d bytes", MaximumBatchBytes)
	}

	requestID, err := newUUID()
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+EndpointPath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create telemetry request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+credential)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-NADI-Agent-ID", agentID)
	request.Header.Set("X-NADI-Request-ID", requestID)

	response, err := s.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("send telemetry batch: %w", err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 64*1024+1))
	if err != nil || len(responseBody) > 64*1024 {
		return nil, errors.New("Central telemetry response is unreadable or oversized")
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Central telemetry endpoint returned HTTP %d", response.StatusCode)
	}

	var ack Ack
	if err := json.Unmarshal(responseBody, &ack); err != nil {
		return nil, fmt.Errorf("decode Central telemetry acknowledgement: %w", err)
	}
	if !ack.Success || !ack.Data.Success || ack.Data.Status != "accepted" || ack.Data.BatchID != pending.Batch.BatchID || ack.Data.RejectedCount != 0 {
		return nil, errors.New("Central telemetry acknowledgement did not confirm acceptance of the pending batch")
	}
	if ack.Data.RecordCount != len(pending.Batch.Records) || ack.Data.AcceptedCount+ack.Data.DuplicateCount != len(pending.Batch.Records) {
		return nil, errors.New("Central telemetry acknowledgement counts do not match the pending batch")
	}
	return pending.Sources, nil
}

func (s *Sender) Complete() error {
	return s.removePending()
}

func (s *Sender) PendingSources() ([]Source, bool, error) {
	pending, err := s.loadPending()
	if err != nil {
		return nil, false, err
	}
	if pending == nil {
		return nil, false, nil
	}
	return append([]Source(nil), pending.Sources...), true, nil
}

func (s *Sender) DiscardPending() error {
	return s.removePending()
}

func (s *Sender) buildPending(instanceID string, inputs []InputRecord) (*pendingBatch, error) {
	if len(inputs) > MaximumBatchRecords {
		inputs = inputs[:MaximumBatchRecords]
	}
	sequences, err := s.loadSequences()
	if err != nil {
		return nil, err
	}
	batchID, err := newUUID()
	if err != nil {
		return nil, err
	}
	batch := Batch{
		ProtocolVersion: "1.0",
		BatchID:         batchID,
		AgentInstanceID: instanceID,
		SentAt:          time.Now().UTC(),
		Records:         make([]Record, 0, len(inputs)),
	}
	sources := make([]Source, 0, len(inputs))
	for _, input := range inputs {
		if _, supported := allowedStreams[input.Stream]; !supported {
			return nil, fmt.Errorf("unsupported Central telemetry stream %q", input.Stream)
		}
		if len(input.Data) == 0 || !json.Valid(input.Data) {
			return nil, errors.New("telemetry input contains invalid data JSON")
		}
		if strings.HasPrefix(input.Stream, "application_") && (input.ApplicationID == nil || !uuidPattern.MatchString(*input.ApplicationID)) {
			return nil, errors.New("application telemetry requires a valid application_id UUID")
		}
		if input.Stream == "server_metrics" && input.ApplicationID != nil {
			return nil, errors.New("server_metrics must not contain application_id")
		}
		if input.ApplicationID != nil && !uuidPattern.MatchString(*input.ApplicationID) {
			return nil, errors.New("telemetry application_id must be a UUID")
		}
		if input.MetricSet != nil && len(*input.MetricSet) > 100 {
			return nil, errors.New("telemetry metric_set exceeds 100 characters")
		}
		if len(input.Data) > MaximumRecordBytes {
			return nil, fmt.Errorf("telemetry data for stream %s exceeds %d bytes", input.Stream, MaximumRecordBytes)
		}
		scope := "server"
		if input.ApplicationID != nil {
			scope = *input.ApplicationID
		}
		sequenceKey := scope + "|" + input.Stream
		sequences.Values[sequenceKey]++
		recordID, err := newUUID()
		if err != nil {
			return nil, err
		}
		observedAt := input.ObservedAt.UTC()
		collectedAt := input.CollectedAt.UTC()
		if observedAt.IsZero() {
			observedAt = collectedAt
		}
		if collectedAt.IsZero() {
			collectedAt = time.Now().UTC()
		}
		record := Record{
			RecordID:      recordID,
			Stream:        input.Stream,
			Sequence:      sequences.Values[sequenceKey],
			ObservedAt:    observedAt,
			CollectedAt:   collectedAt,
			ApplicationID: input.ApplicationID,
			MetricSet:     input.MetricSet,
			Data:          append(json.RawMessage(nil), input.Data...),
		}
		encodedRecord, err := json.Marshal(record)
		if err != nil {
			return nil, fmt.Errorf("encode telemetry record: %w", err)
		}
		if len(encodedRecord) > MaximumRecordBytes {
			return nil, fmt.Errorf("telemetry record for stream %s exceeds %d bytes", input.Stream, MaximumRecordBytes)
		}
		batch.Records = append(batch.Records, record)
		sources = append(sources, input.Source)
	}
	encoded, err := json.Marshal(batch)
	if err != nil {
		return nil, err
	}
	if len(encoded) > MaximumBatchBytes {
		return nil, fmt.Errorf("telemetry batch exceeds %d bytes; batch splitting is required", MaximumBatchBytes)
	}
	if err := s.saveSequences(sequences); err != nil {
		return nil, err
	}
	return &pendingBatch{Batch: batch, Sources: sources}, nil
}

func (s *Sender) loadPending() (*pendingBatch, error) {
	content, err := os.ReadFile(filepath.Join(s.dir, "pending-batch.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read pending telemetry batch: %w", err)
	}
	var pending pendingBatch
	if err := json.Unmarshal(content, &pending); err != nil {
		return nil, fmt.Errorf("decode pending telemetry batch: %w", err)
	}
	return &pending, nil
}

func (s *Sender) savePending(pending pendingBatch) error {
	return s.writeJSON("pending-batch.json", pending)
}

func (s *Sender) removePending() error {
	path := filepath.Join(s.dir, "pending-batch.json")
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove acknowledged telemetry batch: %w", err)
	}
	return syncDirectory(s.dir)
}

func (s *Sender) loadSequences() (sequenceState, error) {
	content, err := os.ReadFile(filepath.Join(s.dir, "sequences.json"))
	if errors.Is(err, os.ErrNotExist) {
		return sequenceState{Values: make(map[string]uint64)}, nil
	}
	if err != nil {
		return sequenceState{}, fmt.Errorf("read telemetry sequences: %w", err)
	}
	var current sequenceState
	if err := json.Unmarshal(content, &current); err != nil {
		return sequenceState{}, fmt.Errorf("decode telemetry sequences: %w", err)
	}
	if current.Values == nil {
		current.Values = make(map[string]uint64)
	}
	return current, nil
}

func (s *Sender) saveSequences(current sequenceState) error {
	return s.writeJSON("sequences.json", current)
}

func (s *Sender) writeJSON(name string, value any) error {
	content, err := json.MarshalIndent(value, "", "    ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", name, err)
	}
	content = append(content, '\n')
	temporary, err := os.CreateTemp(s.dir, ".telemetry-*.tmp")
	if err != nil {
		return fmt.Errorf("create %s: %w", name, err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, filepath.Join(s.dir, name)); err != nil {
		return fmt.Errorf("replace %s: %w", name, err)
	}
	return syncDirectory(s.dir)
}

func syncDirectory(directory string) error {
	file, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}

func newUUID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate telemetry UUID: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(value[:])
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32], nil
}
