# NADI Agent v0.7.4-dev

## Laravel application monitoring foundation

- Added a per-application HTTPS client for the NADI Laravel Bridge.
- Requests use the enrolled application credential, never the Agent credential.
- TLS certificate verification remains enabled; TLS 1.2 is the minimum.
- Redirects are rejected to prevent application credential forwarding.
- Application collection is gated by the verified per-application Activation Grant capabilities.
- Implemented collection of Laravel health, runtime information, and database probe results.
- HTTP 503 database probe responses with structured Bridge data are represented as degraded, not as a successful probe.
- Added explicit unsupported statuses for metrics, queue, and scheduler endpoints that return HTTP 501 in Laravel Bridge v0.3.1-dev.
- Added bounded, permission-restricted, per-application JSONL spools with read, acknowledgement, and clear operations.
- A closed application gate clears only that application's local telemetry spool. Control Poll remains independent.

## Not included

- No telemetry upload to Central. Central ingestion request/response, batch, sequence, idempotency, and acknowledgement contract is not yet available in the Agent source baseline.
- No queue worker heartbeat implementation. The current Bridge queue endpoint returns HTTP 501, and the Agent enrollment manifest does not contain a verified Laravel filesystem path for safely attributing worker processes to an application.
- No scheduler heartbeat/event implementation. The current Bridge scheduler endpoint returns HTTP 501.
- No change to NADI Central or Laravel Bridge source/configuration.
