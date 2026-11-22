package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

const CurrentSchemaVersion = 1

var ErrNotFound = errors.New("agent state not found")

type PendingEnrollment struct {
	RequestID string `json:"request_id"`
	Body      string `json:"body"`
}

type StoredActivationGrant struct {
	GrantID       string          `json:"grant_id"`
	GrantVersion  int             `json:"grant_version"`
	KeyID         string          `json:"key_id"`
	ApplicationID string          `json:"application_id"`
	Payload       json.RawMessage `json:"payload"`
	Signature     string          `json:"signature"`
	ExpiresAt     time.Time       `json:"expires_at"`
	ValidatedAt   time.Time       `json:"validated_at"`
	Acknowledged  bool            `json:"acknowledged"`
}

type PendingAcknowledgement struct {
	GrantID       string  `json:"grant_id"`
	ApplicationID string  `json:"application_id"`
	RequestID     string  `json:"request_id"`
	Result        string  `json:"result"`
	ErrorCode     *string `json:"error_code"`
	Message       *string `json:"message"`
}

type EnrolledApplication struct {
	ApplicationID         string `json:"application_id"`
	ApplicationKey        string `json:"application_key"`
	ApplicationName       string `json:"application_name"`
	Environment           string `json:"environment"`
	RegistrationID        string `json:"registration_id"`
	RegistrationStatus    string `json:"registration_status"`
	ActivationStatus      string `json:"activation_status"`
	MonitoringStatus      string `json:"monitoring_status"`
	Endpoint              string `json:"endpoint"`
	ApplicationCredential string `json:"application_credential"`
}

type CredentialRotation struct {
	DeliveryID         string    `json:"delivery_id"`
	CredentialID       string    `json:"credential_id"`
	PreviousCredential string    `json:"previous_credential"`
	NewCredential      string    `json:"new_credential"`
	ExpiresAt          time.Time `json:"expires_at"`
	AckStarted         bool      `json:"ack_started"`
	KeyVersion         int       `json:"key_version,omitempty"`
}

type State struct {
	SchemaVersion             int                      `json:"schema_version"`
	AgentInstanceID           string                   `json:"agent_instance_id"`
	Hostname                  string                   `json:"hostname"`
	MachineFingerprint        string                   `json:"machine_fingerprint"`
	CentralAgentID            string                   `json:"central_agent_id,omitempty"`
	CentralServerID           string                   `json:"central_server_id,omitempty"`
	AgentCredential           string                   `json:"agent_credential,omitempty"`
	AgentCredentialID         string                   `json:"agent_credential_id,omitempty"`
	AgentCredentialKeyVersion int                      `json:"agent_credential_key_version,omitempty"`
	CredentialRotation        *CredentialRotation      `json:"credential_rotation,omitempty"`
	AgentCredentialExpiresAt  time.Time                `json:"agent_credential_expires_at,omitempty"`
	ControlPollURL            string                   `json:"control_poll_url,omitempty"`
	PollIntervalSeconds       int                      `json:"poll_interval_seconds,omitempty"`
	Applications              []EnrolledApplication    `json:"applications,omitempty"`
	ActivationGrants          []StoredActivationGrant  `json:"activation_grants,omitempty"`
	PendingAcknowledgements   []PendingAcknowledgement `json:"pending_acknowledgements,omitempty"`
	PendingEnrollment         *PendingEnrollment       `json:"pending_enrollment,omitempty"`
	EnrollmentStatus          string                   `json:"enrollment_status"`
	CreatedAt                 time.Time                `json:"created_at"`
	UpdatedAt                 time.Time                `json:"updated_at"`
}

type Store struct {
	directory string
	lockFile  *os.File
	mutex     sync.Mutex
	closed    bool
}

func Open(directory string) (*Store, error) {
	if directory == "" {
		return nil, errors.New("state directory must not be empty")
	}

	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}

	if err := os.Chmod(directory, 0700); err != nil {
		return nil, fmt.Errorf("secure state directory: %w", err)
	}

	lockPath := filepath.Join(directory, "agent.lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open agent lock: %w", err)
	}

	if err := lockFile.Chmod(0600); err != nil {
		lockFile.Close()
		return nil, fmt.Errorf("secure agent lock: %w", err)
	}

	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lockFile.Close()
		return nil, fmt.Errorf("another NADI Agent process may already be running: %w", err)
	}

	return &Store{
		directory: directory,
		lockFile:  lockFile,
	}, nil
}

func (s *Store) Load() (State, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.closed {
		return State{}, errors.New("state store is closed")
	}

	path := filepath.Join(s.directory, "state.json")
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{}, ErrNotFound
		}

		return State{}, fmt.Errorf("read agent state: %w", err)
	}

	var current State
	if err := json.Unmarshal(content, &current); err != nil {
		return State{}, fmt.Errorf("decode agent state: %w", err)
	}

	if current.SchemaVersion != CurrentSchemaVersion {
		return State{}, fmt.Errorf(
			"unsupported agent state schema version %d; expected %d",
			current.SchemaVersion,
			CurrentSchemaVersion,
		)
	}

	if current.AgentInstanceID == "" {
		return State{}, errors.New("agent state has an empty agent_instance_id")
	}

	return current, nil
}

func (s *Store) Save(current State) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.closed {
		return errors.New("state store is closed")
	}

	if current.AgentInstanceID == "" {
		return errors.New("agent_instance_id must not be empty")
	}

	current.SchemaVersion = CurrentSchemaVersion
	current.UpdatedAt = time.Now().UTC()

	content, err := json.MarshalIndent(current, "", "    ")
	if err != nil {
		return fmt.Errorf("encode agent state: %w", err)
	}

	content = append(content, '\n')

	temporaryFile, err := os.CreateTemp(s.directory, ".state-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary state file: %w", err)
	}

	temporaryPath := temporaryFile.Name()
	defer os.Remove(temporaryPath)

	if err := temporaryFile.Chmod(0600); err != nil {
		temporaryFile.Close()
		return fmt.Errorf("secure temporary state file: %w", err)
	}

	if _, err := temporaryFile.Write(content); err != nil {
		temporaryFile.Close()
		return fmt.Errorf("write temporary state file: %w", err)
	}

	if err := temporaryFile.Sync(); err != nil {
		temporaryFile.Close()
		return fmt.Errorf("sync temporary state file: %w", err)
	}

	if err := temporaryFile.Close(); err != nil {
		return fmt.Errorf("close temporary state file: %w", err)
	}

	destination := filepath.Join(s.directory, "state.json")
	if err := os.Rename(temporaryPath, destination); err != nil {
		return fmt.Errorf("replace agent state: %w", err)
	}

	directoryFile, err := os.Open(s.directory)
	if err != nil {
		return fmt.Errorf("open state directory for sync: %w", err)
	}
	defer directoryFile.Close()

	if err := directoryFile.Sync(); err != nil {
		return fmt.Errorf("sync state directory: %w", err)
	}

	return nil
}

func (s *Store) Close() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true

	unlockErr := syscall.Flock(int(s.lockFile.Fd()), syscall.LOCK_UN)
	closeErr := s.lockFile.Close()

	if unlockErr != nil {
		return fmt.Errorf("release agent process lock: %w", unlockErr)
	}

	if closeErr != nil {
		return fmt.Errorf("close agent process lock: %w", closeErr)
	}

	return nil
}
