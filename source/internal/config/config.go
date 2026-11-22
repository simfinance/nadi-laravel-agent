package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	CentralURL              string
	AgentToken              string
	EnrollmentToken         string
	LogLevel                string
	PollInterval            time.Duration
	HTTPTimeout             time.Duration
	StateDir                string
	ActivationPublicKeyPath string
	ActivationKeyID         string
}

func Load() (Config, error) {
	cfg := Config{
		CentralURL:              strings.TrimRight(envOrDefault("NADI_CENTRAL_URL", "https://nadi.pesantrenalirsyad.org"), "/"),
		AgentToken:              strings.TrimSpace(os.Getenv("NADI_AGENT_TOKEN")),
		EnrollmentToken:         strings.TrimSpace(os.Getenv("NADI_ENROLLMENT_TOKEN")),
		LogLevel:                strings.ToLower(envOrDefault("NADI_LOG_LEVEL", "info")),
		PollInterval:            30 * time.Second,
		HTTPTimeout:             10 * time.Second,
		StateDir:                envOrDefault("NADI_STATE_DIR", "/var/lib/nadi-agent"),
		ActivationPublicKeyPath: envOrDefault("NADI_ACTIVATION_PUBLIC_KEY", "/etc/nadi-agent/keys/activation-public.pem"),
		ActivationKeyID:         envOrDefault("NADI_ACTIVATION_KEY_ID", "nadi-central-2026-01"),
	}

	if value := strings.TrimSpace(os.Getenv("NADI_POLL_INTERVAL_SECONDS")); value != "" {
		seconds, err := strconv.Atoi(value)
		if err != nil || seconds < 1 {
			return Config{}, errors.New("NADI_POLL_INTERVAL_SECONDS must be a positive integer")
		}

		cfg.PollInterval = time.Duration(seconds) * time.Second
	}

	if value := strings.TrimSpace(os.Getenv("NADI_HTTP_TIMEOUT_SECONDS")); value != "" {
		seconds, err := strconv.Atoi(value)
		if err != nil || seconds < 1 {
			return Config{}, errors.New("NADI_HTTP_TIMEOUT_SECONDS must be a positive integer")
		}

		cfg.HTTPTimeout = time.Duration(seconds) * time.Second
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) Validate() error {
	parsedURL, err := url.ParseRequestURI(c.CentralURL)
	if err != nil {
		return fmt.Errorf("NADI_CENTRAL_URL is invalid: %w", err)
	}

	if parsedURL.Scheme != "https" || parsedURL.Host == "" {
		return errors.New("NADI_CENTRAL_URL must be an absolute HTTPS URL")
	}

	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return errors.New("NADI_LOG_LEVEL must be debug, info, warn, or error")
	}

	if c.PollInterval <= 0 {
		return errors.New("poll interval must be positive")
	}

	if c.HTTPTimeout <= 0 {
		return errors.New("HTTP timeout must be positive")
	}

	if strings.TrimSpace(c.StateDir) == "" {
		return errors.New("NADI_STATE_DIR must not be empty")
	}

	return nil
}

func envOrDefault(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	return value
}
