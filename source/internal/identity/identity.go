package identity

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"nadi-agent/internal/state"
)

var ErrMachineIdentityChanged = errors.New("machine identity does not match the persisted NADI Agent state")

func Ensure(store *state.Store) (state.State, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return state.State{}, fmt.Errorf("read server hostname: %w", err)
	}

	fingerprint, err := machineFingerprint()
	if err != nil {
		return state.State{}, err
	}

	current, err := store.Load()
	if err == nil {
		if current.MachineFingerprint != fingerprint {
			return state.State{}, ErrMachineIdentityChanged
		}

		if current.Hostname != hostname {
			current.Hostname = hostname
			if err := store.Save(current); err != nil {
				return state.State{}, err
			}
		}

		return current, nil
	}

	if !errors.Is(err, state.ErrNotFound) {
		return state.State{}, err
	}

	instanceID, err := newUUID()
	if err != nil {
		return state.State{}, fmt.Errorf("generate agent instance ID: %w", err)
	}

	now := time.Now().UTC()
	current = state.State{
		AgentInstanceID:    instanceID,
		Hostname:           hostname,
		MachineFingerprint: fingerprint,
		EnrollmentStatus:   "unregistered",
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := store.Save(current); err != nil {
		return state.State{}, err
	}

	return current, nil
}

func machineFingerprint() (string, error) {
	paths := []string{
		"/etc/machine-id",
		"/var/lib/dbus/machine-id",
	}

	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		machineID := strings.TrimSpace(string(content))
		if machineID == "" {
			continue
		}

		digest := sha256.Sum256([]byte(machineID))
		return hex.EncodeToString(digest[:]), nil
	}

	return "", errors.New("stable machine ID not found; expected /etc/machine-id or /var/lib/dbus/machine-id")
}

func newUUID() (string, error) {
	var value [16]byte

	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}

	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80

	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		value[0:4],
		value[4:6],
		value[6:8],
		value[8:10],
		value[10:16],
	), nil
}
