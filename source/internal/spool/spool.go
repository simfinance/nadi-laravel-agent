package spool

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nadi-agent/internal/servermetrics"
)

const (
	DefaultMaxBytes   int64 = 10 * 1024 * 1024
	DefaultMaxRecords       = 10000
	MaxRecordBytes          = 256 * 1024
)

var ErrFull = errors.New("telemetry spool capacity reached")

type Record struct {
	EventID   string                 `json:"event_id"`
	EventType string                 `json:"event_type"`
	ServerID  string                 `json:"server_id"`
	AgentID   string                 `json:"agent_id"`
	Timestamp time.Time              `json:"timestamp"`
	Payload   servermetrics.Snapshot `json:"payload"`
}

type Store struct {
	directory  string
	path       string
	maxBytes   int64
	maxRecords int
}

func Open(directory string, maxBytes int64, maxRecords int) (*Store, error) {
	if strings.TrimSpace(directory) == "" {
		return nil, errors.New("spool directory must not be empty")
	}
	if maxBytes <= 0 || maxRecords <= 0 {
		return nil, errors.New("spool limits must be positive")
	}

	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, fmt.Errorf("create telemetry spool directory: %w", err)
	}
	if err := os.Chmod(directory, 0700); err != nil {
		return nil, fmt.Errorf("secure telemetry spool directory: %w", err)
	}

	return &Store{
		directory:  directory,
		path:       filepath.Join(directory, "telemetry.jsonl"),
		maxBytes:   maxBytes,
		maxRecords: maxRecords,
	}, nil
}

func (s *Store) Append(serverID string, agentID string, snapshot servermetrics.Snapshot) (Record, error) {
	if strings.TrimSpace(serverID) == "" || strings.TrimSpace(agentID) == "" {
		return Record{}, errors.New("server_id and agent_id are required for telemetry")
	}

	eventID, err := newEventID()
	if err != nil {
		return Record{}, err
	}

	record := Record{
		EventID:   eventID,
		EventType: "server_metrics",
		ServerID:  serverID,
		AgentID:   agentID,
		Timestamp: snapshot.Timestamp.UTC(),
		Payload:   snapshot,
	}

	encoded, err := json.Marshal(record)
	if err != nil {
		return Record{}, fmt.Errorf("encode telemetry record: %w", err)
	}
	if len(encoded) > MaxRecordBytes {
		return Record{}, fmt.Errorf("telemetry record exceeds %d bytes", MaxRecordBytes)
	}
	encoded = append(encoded, '\n')

	currentSize, currentRecords, err := s.measure()
	if err != nil {
		return Record{}, err
	}
	if currentRecords >= s.maxRecords || currentSize+int64(len(encoded)) > s.maxBytes {
		return Record{}, ErrFull
	}

	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return Record{}, fmt.Errorf("open telemetry spool: %w", err)
	}
	defer file.Close()

	if err := file.Chmod(0600); err != nil {
		return Record{}, fmt.Errorf("secure telemetry spool file: %w", err)
	}
	if _, err := file.Write(encoded); err != nil {
		return Record{}, fmt.Errorf("append telemetry record: %w", err)
	}
	if err := file.Sync(); err != nil {
		return Record{}, fmt.Errorf("sync telemetry spool: %w", err)
	}

	return record, nil
}

func (s *Store) ReadBatch(limit int) ([]Record, error) {
	if limit <= 0 {
		return nil, errors.New("batch limit must be positive")
	}

	file, err := os.Open(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Record{}, nil
		}
		return nil, fmt.Errorf("open telemetry spool for reading: %w", err)
	}
	defer file.Close()

	records := make([]Record, 0, limit)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), MaxRecordBytes)
	for scanner.Scan() {
		var record Record
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, fmt.Errorf("decode telemetry spool record: %w", err)
		}
		records = append(records, record)
		if len(records) == limit {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read telemetry spool: %w", err)
	}

	return records, nil
}

func (s *Store) Acknowledge(eventIDs []string) error {
	if len(eventIDs) == 0 {
		return nil
	}

	acknowledged := make(map[string]struct{}, len(eventIDs))
	for _, eventID := range eventIDs {
		if strings.TrimSpace(eventID) == "" {
			return errors.New("event_id must not be empty")
		}
		acknowledged[eventID] = struct{}{}
	}

	file, err := os.Open(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("open telemetry spool for acknowledgement: %w", err)
	}
	defer file.Close()

	temporaryFile, err := os.CreateTemp(s.directory, ".telemetry-*.tmp")
	if err != nil {
		return fmt.Errorf("create telemetry spool replacement: %w", err)
	}
	temporaryPath := temporaryFile.Name()
	defer os.Remove(temporaryPath)

	if err := temporaryFile.Chmod(0600); err != nil {
		temporaryFile.Close()
		return fmt.Errorf("secure telemetry spool replacement: %w", err)
	}

	writer := bufio.NewWriter(temporaryFile)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), MaxRecordBytes)
	for scanner.Scan() {
		var record Record
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			temporaryFile.Close()
			return fmt.Errorf("decode telemetry spool during acknowledgement: %w", err)
		}
		if _, remove := acknowledged[record.EventID]; remove {
			continue
		}
		if _, err := writer.Write(append(scanner.Bytes(), '\n')); err != nil {
			temporaryFile.Close()
			return fmt.Errorf("write retained telemetry record: %w", err)
		}
	}
	if err := scanner.Err(); err != nil {
		temporaryFile.Close()
		return fmt.Errorf("scan telemetry spool during acknowledgement: %w", err)
	}
	if err := writer.Flush(); err != nil {
		temporaryFile.Close()
		return fmt.Errorf("flush telemetry spool replacement: %w", err)
	}
	if err := temporaryFile.Sync(); err != nil {
		temporaryFile.Close()
		return fmt.Errorf("sync telemetry spool replacement: %w", err)
	}
	if err := temporaryFile.Close(); err != nil {
		return fmt.Errorf("close telemetry spool replacement: %w", err)
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		return fmt.Errorf("replace telemetry spool: %w", err)
	}

	directory, err := os.Open(s.directory)
	if err != nil {
		return fmt.Errorf("open telemetry spool directory: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync telemetry spool directory: %w", err)
	}

	return nil
}

func (s *Store) Clear() error {
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("clear telemetry spool: %w", err)
	}

	directory, err := os.Open(s.directory)
	if err != nil {
		return fmt.Errorf("open telemetry spool directory: %w", err)
	}
	defer directory.Close()

	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync telemetry spool directory after clear: %w", err)
	}

	return nil
}

func (s *Store) measure() (int64, int, error) {
	file, err := os.Open(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, 0, nil
		}
		return 0, 0, fmt.Errorf("open telemetry spool for capacity check: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return 0, 0, fmt.Errorf("stat telemetry spool: %w", err)
	}

	count := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), MaxRecordBytes)
	for scanner.Scan() {
		count++
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, fmt.Errorf("count telemetry spool records: %w", err)
	}

	return info.Size(), count, nil
}

func newEventID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate telemetry event ID: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := make([]byte, 36)
	hex.Encode(encoded[0:8], value[0:4])
	encoded[8] = '-'
	hex.Encode(encoded[9:13], value[4:6])
	encoded[13] = '-'
	hex.Encode(encoded[14:18], value[6:8])
	encoded[18] = '-'
	hex.Encode(encoded[19:23], value[8:10])
	encoded[23] = '-'
	hex.Encode(encoded[24:36], value[10:16])
	return string(encoded), nil
}
