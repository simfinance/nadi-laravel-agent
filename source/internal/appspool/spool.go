package appspool

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nadi-agent/internal/appmonitor"
)

const (
	DefaultMaxBytes   int64 = 5 * 1024 * 1024
	DefaultMaxRecords       = 5000
	MaxRecordBytes          = 256 * 1024
)

var ErrFull = errors.New("application telemetry spool capacity reached")

type Record struct {
	EventID       string              `json:"event_id"`
	EventType     string              `json:"event_type"`
	ServerID      string              `json:"server_id"`
	AgentID       string              `json:"agent_id"`
	ApplicationID string              `json:"application_id"`
	Timestamp     time.Time           `json:"timestamp"`
	Payload       appmonitor.Snapshot `json:"payload"`
}

type Store struct {
	directory  string
	maxBytes   int64
	maxRecords int
}

func Open(directory string, maxBytes int64, maxRecords int) (*Store, error) {
	if strings.TrimSpace(directory) == "" || maxBytes <= 0 || maxRecords <= 0 {
		return nil, errors.New("application spool directory and positive limits are required")
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, fmt.Errorf("create application spool directory: %w", err)
	}
	if err := os.Chmod(directory, 0700); err != nil {
		return nil, fmt.Errorf("secure application spool directory: %w", err)
	}
	return &Store{directory: directory, maxBytes: maxBytes, maxRecords: maxRecords}, nil
}

func (s *Store) Append(serverID string, agentID string, applicationID string, snapshot appmonitor.Snapshot) (Record, error) {
	if strings.TrimSpace(serverID) == "" || strings.TrimSpace(agentID) == "" || strings.TrimSpace(applicationID) == "" {
		return Record{}, errors.New("server_id, agent_id, and application_id are required")
	}
	eventID, err := newEventID()
	if err != nil {
		return Record{}, err
	}
	record := Record{
		EventID:       eventID,
		EventType:     "application_metrics",
		ServerID:      serverID,
		AgentID:       agentID,
		ApplicationID: applicationID,
		Timestamp:     snapshot.Timestamp.UTC(),
		Payload:       snapshot,
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return Record{}, fmt.Errorf("encode application telemetry record: %w", err)
	}
	if len(encoded) > MaxRecordBytes {
		return Record{}, fmt.Errorf("application telemetry record exceeds %d bytes", MaxRecordBytes)
	}
	encoded = append(encoded, '\n')

	path := s.path(applicationID)
	size, count, err := measure(path)
	if err != nil {
		return Record{}, err
	}
	if count >= s.maxRecords || size+int64(len(encoded)) > s.maxBytes {
		return Record{}, ErrFull
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return Record{}, fmt.Errorf("open application telemetry spool: %w", err)
	}
	defer file.Close()
	if err := file.Chmod(0600); err != nil {
		return Record{}, fmt.Errorf("secure application telemetry spool: %w", err)
	}
	if _, err := file.Write(encoded); err != nil {
		return Record{}, fmt.Errorf("append application telemetry: %w", err)
	}
	if err := file.Sync(); err != nil {
		return Record{}, fmt.Errorf("sync application telemetry spool: %w", err)
	}
	return record, nil
}

func (s *Store) ReadBatch(applicationID string, limit int) ([]Record, error) {
	if strings.TrimSpace(applicationID) == "" || limit <= 0 {
		return nil, errors.New("application_id and positive batch limit are required")
	}
	file, err := os.Open(s.path(applicationID))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Record{}, nil
		}
		return nil, fmt.Errorf("open application spool for reading: %w", err)
	}
	defer file.Close()
	records := make([]Record, 0, limit)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), MaxRecordBytes)
	for scanner.Scan() {
		var record Record
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, fmt.Errorf("decode application spool record: %w", err)
		}
		if record.ApplicationID != applicationID {
			return nil, errors.New("application spool identity mismatch")
		}
		records = append(records, record)
		if len(records) == limit {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read application spool: %w", err)
	}
	return records, nil
}

func (s *Store) Acknowledge(applicationID string, eventIDs []string) error {
	if strings.TrimSpace(applicationID) == "" {
		return errors.New("application_id is required")
	}
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

	path := s.path(applicationID)
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("open application spool for acknowledgement: %w", err)
	}
	defer file.Close()

	temporaryFile, err := os.CreateTemp(s.directory, ".application-spool-*.tmp")
	if err != nil {
		return fmt.Errorf("create application spool replacement: %w", err)
	}
	temporaryPath := temporaryFile.Name()
	defer os.Remove(temporaryPath)
	if err := temporaryFile.Chmod(0600); err != nil {
		temporaryFile.Close()
		return fmt.Errorf("secure application spool replacement: %w", err)
	}

	writer := bufio.NewWriter(temporaryFile)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), MaxRecordBytes)
	for scanner.Scan() {
		var record Record
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			temporaryFile.Close()
			return fmt.Errorf("decode application spool during acknowledgement: %w", err)
		}
		if record.ApplicationID != applicationID {
			temporaryFile.Close()
			return errors.New("application spool identity mismatch during acknowledgement")
		}
		if _, remove := acknowledged[record.EventID]; remove {
			continue
		}
		if _, err := writer.Write(append(scanner.Bytes(), '\n')); err != nil {
			temporaryFile.Close()
			return fmt.Errorf("write retained application telemetry: %w", err)
		}
	}
	if err := scanner.Err(); err != nil {
		temporaryFile.Close()
		return fmt.Errorf("scan application spool during acknowledgement: %w", err)
	}
	if err := writer.Flush(); err != nil {
		temporaryFile.Close()
		return fmt.Errorf("flush application spool replacement: %w", err)
	}
	if err := temporaryFile.Sync(); err != nil {
		temporaryFile.Close()
		return fmt.Errorf("sync application spool replacement: %w", err)
	}
	if err := temporaryFile.Close(); err != nil {
		return fmt.Errorf("close application spool replacement: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace application spool: %w", err)
	}

	directory, err := os.Open(s.directory)
	if err != nil {
		return fmt.Errorf("open application spool directory for sync: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync application spool directory: %w", err)
	}
	return nil
}

func (s *Store) Clear(applicationID string) error {
	if strings.TrimSpace(applicationID) == "" {
		return errors.New("application_id is required")
	}
	if err := os.Remove(s.path(applicationID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("clear application telemetry spool: %w", err)
	}
	return nil
}

func (s *Store) path(applicationID string) string {
	digest := sha256.Sum256([]byte(applicationID))
	return filepath.Join(s.directory, hex.EncodeToString(digest[:])+".jsonl")
}

func measure(path string) (int64, int, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, 0, nil
		}
		return 0, 0, fmt.Errorf("open application spool for capacity check: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return 0, 0, fmt.Errorf("stat application spool: %w", err)
	}
	count := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), MaxRecordBytes)
	for scanner.Scan() {
		count++
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, fmt.Errorf("count application spool records: %w", err)
	}
	return info.Size(), count, nil
}

func newEventID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate application telemetry event ID: %w", err)
	}
	return hex.EncodeToString(value[:]), nil
}
