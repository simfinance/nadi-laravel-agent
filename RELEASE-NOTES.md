# NADI Agent v0.7.5-dev

## Central telemetry delivery foundation

- Added JSON batch delivery to the Central telemetry endpoint.
- Persists batch payload, batch ID, record IDs, and per-scope/per-stream sequence counters.
- Retries preserve the exact pending batch and record identifiers.
- Validates the Central acceptance acknowledgement before acknowledging local spool records.
- Enforces the official 11-stream allowlist and Central's application-scope rules, including optional application IDs for Nginx streams and mandatory application IDs for `application_*` streams.
- Validates UUIDs and `metric_set` length before creating a batch.
- Preserves the existing Activation Gate and Control Poll lifecycle.
- Added an optional systemd unit and a simpiav2 x86_64 installer that does not enroll, activate, or start the service automatically.

## Verified locally

- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- Linux AMD64 and ARM64 cross-builds
- Binary checksum verification

## Limitations — not production-ready

- HTTP status-specific stop/quarantine/retry policy is incomplete; in particular 401, 403, 409, 410, 413, 422, and 429 need explicit operational handling.
- Automatic batch splitting for HTTP 413 and pre-send size splitting are not implemented.
- Nginx-specific streams and HTTP request/error event streams are not yet collected or emitted.
- HTTP event redaction is not implemented because those event streams are not emitted by this build.
- Laravel Bridge metrics, queue, and scheduler endpoints that return HTTP 501 remain unsupported; the Agent does not fabricate their data.
- No live Agent-to-Central E2E or Central storage verification has been performed.

This is an installable development/pilot candidate for controlled validation, not a production release.
