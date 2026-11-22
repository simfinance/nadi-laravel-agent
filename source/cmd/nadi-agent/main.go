package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"nadi-agent/internal/app"
	"nadi-agent/internal/config"
	"nadi-agent/internal/logging"
	"nadi-agent/internal/version"
)

func main() {
	showVersion := flag.Bool("version", false, "Print NADI Agent version")
	enroll := flag.Bool("enroll", false, "Enroll this Agent with NADI Central")
	manifestPath := flag.String("manifest", "", "Path to the Enrollment application manifest JSON")
	enrollmentTokenStdin := flag.Bool("enrollment-token-stdin", false, "Read the one-time Enrollment token from stdin")
	rotateCredential := flag.Bool("rotate-credential", false, "Retrieve and acknowledge a pending Agent credential rotation")
	deliveryID := flag.String("delivery-id", "", "Central credential delivery ID to retrieve")
	flag.Parse()

	if *showVersion {
		fmt.Printf("NADI Agent %s\n", version.Agent)
		return
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(1)
	}

	if *rotateCredential && (*enroll || *manifestPath != "" || *enrollmentTokenStdin) {
		fmt.Fprintln(os.Stderr, "--rotate-credential cannot be combined with Enrollment options")
		os.Exit(2)
	}

	if *rotateCredential && strings.TrimSpace(*deliveryID) == "" {
		fmt.Fprintln(os.Stderr, "--rotate-credential requires --delivery-id")
		os.Exit(2)
	}

	if !*rotateCredential && strings.TrimSpace(*deliveryID) != "" {
		fmt.Fprintln(os.Stderr, "--delivery-id requires --rotate-credential")
		os.Exit(2)
	}

	if *enrollmentTokenStdin && !*enroll {
		fmt.Fprintln(os.Stderr, "--enrollment-token-stdin requires --enroll")
		os.Exit(2)
	}

	if *enrollmentTokenStdin && strings.TrimSpace(cfg.EnrollmentToken) != "" {
		fmt.Fprintln(os.Stderr, "do not combine --enrollment-token-stdin with NADI_ENROLLMENT_TOKEN")
		os.Exit(2)
	}

	if *enrollmentTokenStdin {
		token, readErr := readEnrollmentToken(os.Stdin)
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "unable to read Enrollment token from stdin: %v\\n", readErr)
			os.Exit(1)
		}

		cfg.EnrollmentToken = token
	}

	logger := logging.New(cfg.LogLevel)

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	application := app.New(cfg, logger)

	if *rotateCredential {
		if err := application.RotateCredential(ctx, *deliveryID); err != nil {
			logger.Error("NADI Agent credential rotation did not complete", "error", err)
			os.Exit(1)
		}
		return
	}

	if *enroll {
		if err := application.Enroll(ctx, *manifestPath); err != nil {
			logger.Error("NADI Agent Enrollment failed", "error", err)
			os.Exit(1)
		}
		return
	}

	if *manifestPath != "" {
		fmt.Fprintln(os.Stderr, "--manifest requires --enroll")
		os.Exit(2)
	}

	if err := application.Run(ctx); err != nil {
		logger.Error("NADI Agent stopped with an error", "error", err)
		os.Exit(1)
	}
}

func readEnrollmentToken(reader io.Reader) (string, error) {
	tokenBytes, err := io.ReadAll(io.LimitReader(reader, 257))
	if err != nil {
		return "", err
	}

	if len(tokenBytes) > 256 {
		return "", fmt.Errorf("Enrollment token input exceeds the allowed size")
	}

	token := strings.TrimSpace(string(tokenBytes))
	if token == "" {
		return "", fmt.Errorf("Enrollment token input is empty")
	}

	return token, nil
}
