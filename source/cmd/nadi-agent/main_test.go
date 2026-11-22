package main

import (
	"strings"
	"testing"
)

func TestReadEnrollmentToken(t *testing.T) {
	token, err := readEnrollmentToken(strings.NewReader("  abc123  \n"))
	if err != nil {
		t.Fatalf("readEnrollmentToken returned error: %v", err)
	}

	if token != "abc123" {
		t.Fatalf("expected trimmed token, got %q", token)
	}
}

func TestReadEnrollmentTokenRejectsEmptyInput(t *testing.T) {
	_, err := readEnrollmentToken(strings.NewReader(" \n"))
	if err == nil {
		t.Fatal("expected empty token input to fail")
	}
}

func TestReadEnrollmentTokenRejectsOversizedInput(t *testing.T) {
	_, err := readEnrollmentToken(strings.NewReader(strings.Repeat("x", 257)))
	if err == nil {
		t.Fatal("expected oversized token input to fail")
	}
}
