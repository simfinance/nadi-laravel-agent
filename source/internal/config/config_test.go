package config

import (
	"testing"
	"time"
)

func TestValidateRequiresHTTPSCentralURL(t *testing.T) {
	cfg := Config{
		CentralURL:   "http://central.example.test",
		LogLevel:     "info",
		PollInterval: 30 * time.Second,
		HTTPTimeout:  10 * time.Second,
		StateDir:     "/tmp/nadi-agent-test",
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("expected HTTP Central URL to be rejected")
	}
}

func TestValidateAcceptsHTTPSCentralURL(t *testing.T) {
	cfg := Config{
		CentralURL:   "https://central.example.test",
		LogLevel:     "info",
		PollInterval: 30 * time.Second,
		HTTPTimeout:  10 * time.Second,
		StateDir:     "/tmp/nadi-agent-test",
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid configuration, got: %v", err)
	}
}
