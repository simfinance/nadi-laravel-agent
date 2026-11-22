# NADI Agent

Standalone Go agent for NADI Central.

## Current milestone: Agent Credential Rotation v0.7.0-dev

- persistent Agent identity and local JSON state
- HTTPS Enrollment request to `POST /api/v1/nadi/agent/enroll`
- one-time Enrollment token supplied through `NADI_ENROLLMENT_TOKEN`
- application manifest supplied by the operator as JSON
- durable pending Enrollment request; retries reuse the exact request ID and body
- Agent and per-application credentials stored in the protected local state file
- Control Poll uses the persisted Agent credential and Central-provided poll URL/interval
- Ed25519 Activation Grant signature and identity validation
- durable verified-grant storage and Activation ACK retry with stable request IDs
- Runtime Activation Gate revalidates the latest Central application status and signed grant on every successful Control Poll
- monitoring access fails closed when an application is missing, inactive, suspended, unapproved, or has no matching valid acknowledged grant
- bounded local server-metrics spool with stable event IDs, server/agent identity, restrictive permissions, and explicit acknowledgement removal
- no telemetry upload or command execution; Central ingestion contract is not assumed
- credential delivery retrieve, atomic staging, new-credential ACK, and restart-safe retry

## Requirements

- Go 1.23 or newer to build from source
- Linux with `/etc/machine-id` or `/var/lib/dbus/machine-id`

## Configuration

| Environment variable | Default | Description |
|---|---|---|
| `NADI_CENTRAL_URL` | `https://nadi.pesantrenalirsyad.org` | Central base URL; HTTPS required |
| `NADI_AGENT_TOKEN` | empty | Optional explicit Bearer credential override |
| `NADI_ENROLLMENT_TOKEN` | empty | One-time Enrollment token, exactly 76 characters |
| `NADI_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error` |
| `NADI_POLL_INTERVAL_SECONDS` | `30` | Fallback Control Poll interval |
| `NADI_HTTP_TIMEOUT_SECONDS` | `10` | HTTP request timeout |
| `NADI_STATE_DIR` | `/var/lib/nadi-agent` | Protected local state directory |
| `NADI_ACTIVATION_PUBLIC_KEY` | `/etc/nadi-agent/keys/activation-public.pem` | Central Ed25519 public key in PKIX PEM format |
| `NADI_ACTIVATION_KEY_ID` | `nadi-central-2026-01` | Expected Activation Grant key ID |

## Enrollment manifest

Create a JSON file containing the applications to register. The Central endpoint accepts 1 to 20 applications.

```json
{
    "applications": [
        {
            "application_key": "example-production",
            "application_name": "Example Laravel Application",
            "environment": "production",
            "framework_version": "13.x",
            "php_version": "8.5",
            "base_url": "https://app.example.org"
        }
    ]
}
```

Each `base_url` must use HTTPS. `application_key` values must be unique in the manifest.

## Enroll

Do not put the Enrollment token directly in a command argument or shell history. Supply it through the environment, preferably from a protected secret-handling mechanism:

```bash
export NADI_STATE_DIR="$HOME/.local/state/nadi-agent"
read -rsp "Enrollment token: " NADI_ENROLLMENT_TOKEN
echo
export NADI_ENROLLMENT_TOKEN

./nadi-agent --enroll --manifest ./enrollment.json
unset NADI_ENROLLMENT_TOKEN
```

The pending request, including its token, is persisted in `state.json` with directory permission `0700` and file permission `0600`. Secrets are protected by local filesystem permissions; they are not encrypted at rest by this milestone. If the request times out or the process stops before saving Central's response, run the same command again. The Agent reuses the exact persisted request body and request ID. Do not edit or delete `state.json` while a pending Enrollment exists.

After successful Enrollment, the Agent credential and application credentials are stored in the protected state file. The pending request is removed. The Enrollment token is no longer needed for normal Agent startup.

## Run

```bash
./nadi-agent
```

If an enrolled Agent credential is present and unexpired, the Agent starts Control Poll immediately. Otherwise it waits without contacting Central.

## Build and test

```bash
gofmt -w ./cmd ./internal
go test ./...
go vet ./...
go build -o nadi-agent ./cmd/nadi-agent
```

## Activation Grant (v0.5.0-dev)

The Agent verifies Central Activation Grants locally using Ed25519. Configure the
Central public key on the Agent host and set `NADI_ACTIVATION_PUBLIC_KEY` to its
path (default: `/etc/nadi-agent/keys/activation-public.pem`). The expected key ID
can be set with `NADI_ACTIVATION_KEY_ID` (default: `nadi-central-2026-01`).

The public key must be an Ed25519 SubjectPublicKeyInfo PEM file. The Agent stores
verified grants and pending acknowledgements in its protected local state file.
Acknowledgement retries reuse the original request ID. Invalid grants are rejected
locally and are never acknowledged as successful. No monitoring collectors, telemetry,
or command execution are enabled in this milestone. A successful Activation ACK tells
Central that the Agent accepted the grant; it does not mean this Agent build is already
collecting monitoring data.


## Runtime Activation Gate (v0.6.0-dev)

After every successful Control Poll, the Agent persists the latest registration,
activation, and monitoring statuses for each enrolled application. It then evaluates
each application's Runtime Activation Gate against the current Poll response and the
locally persisted Activation Grant. The grant must match the current Central grant,
pass Ed25519 signature and identity verification, remain unexpired, and have an
accepted Activation ACK. The Agent installation state must also be `installed`.

If an application is absent from the Poll response, its local control statuses are
set to `unknown`. Missing, changed, expired, suspended, unapproved, or otherwise
invalid authorization keeps the gate closed. This milestone exposes the gate decision
in logs; it does not yet start monitoring collectors or send telemetry.


## v0.6.0-dev-fix1

- Runtime Activation Gate compares stored and polled signed payloads using canonical JSON, avoiding false mismatches caused only by JSON property order or whitespace.
- Activation Grant signature verification and identity, status, expiry, acknowledgement, and capability checks remain enforced.


## Server Metrics Engine (v0.6.1-dev)

The Agent collects server-level metrics once per server, not once per application. Collection is enabled only when at least one application has an open Runtime Activation Gate and its verified Activation Grant includes the `server_metrics` capability. If no eligible application remains, the CPU sampling baseline is reset and collection stops.

The Linux collector reads kernel-provided `/proc` data and filesystem statistics without executing shell commands or requiring root. It captures hostname, OS, architecture, logical CPU count and CPU utilization (available after the second sample), memory totals/availability/usage, mounted filesystem capacity, load averages, and uptime. The current milestone writes structured local logs only; it does not send telemetry to Central or persist metric samples. Collection is evaluated on the existing Control Poll cycle (normally every 30 seconds).


## Control Poll Origin Hardening (v0.6.3-dev)

- The Central-provided Control Poll URL must use the same scheme and host as the configured Central base URL.
- Poll URLs containing user information or fragments are rejected.
- Control Poll HTTP redirects are not followed, preventing the Agent credential from being forwarded to a redirect target.
- Cross-origin, malformed, and redirect-based Poll destinations fail closed before a successful Control Poll is accepted.


## Bounded Server Telemetry Spool (v0.6.4-dev)

When at least one application has an open Runtime Activation Gate with the `server_metrics` capability, the Agent collects one server snapshot and appends a structured `server_metrics` event to a local JSONL spool. The record contains a stable event ID, Central server ID, Central agent ID, timestamp, and server snapshot. Server-level records deliberately do not contain an application ID.

The spool is stored below the protected Agent state directory in `spool/telemetry.jsonl`, with directory mode `0700` and file mode `0600`. Initial bounded defaults are 10 MiB and 10,000 records; these are conservative implementation defaults and remain subject to pilot sizing. When full, the spool rejects the newest record rather than silently deleting older unacknowledged records. Records can be read in order and removed by event ID after a future ingestion acknowledgement.

If the Runtime Activation Gate closes for all applications or no active grant permits server metrics, the local server-metrics spool is cleared so previously queued server data is not retained for a later activation. Control Poll continues independently.

This milestone does not upload telemetry. The Agent-side spool is ready for a separately agreed Central ingestion contract, but no endpoint, request schema, gzip framing, or acknowledgement response is invented here.

## Agent Credential Rotation (v0.7.0-dev)

This milestone adds explicit credential delivery retrieval and ACK for an
already-enrolled Agent. Retrieve uses the currently active (old) Agent
credential. The returned credential is staged in the protected `state.json`
file using atomic replacement and restrictive `0700` directory / `0600` file
permissions. The Agent then switches to the new credential and sends ACK using
that new credential. The old credential is retained in the pending rotation
record until Central's ACK response is validated.

The state file is protected by local filesystem permissions; this milestone
does not provide encryption at rest. Do not copy it outside the Agent's
protected state directory or expose it to application/PHP-FPM users.

To process the specific pending delivery supplied by Central, stop the running
Agent service first so the state lock is released, then run as the dedicated
`nadi-agent` OS user:

```bash
sudo systemctl stop nadi-agent
sudo -u nadi-agent /opt/nadi/bin/nadi-agent --rotate-credential --delivery-id 3b26f2d6-448a-4c39-b224-359a30c4b38e
sudo systemctl start nadi-agent
```

Use the actual installed binary path and state directory configured on the
server. The command requires `NADI_AGENT_TOKEN` to be unset so the persisted
old credential is used. It never prints either credential. If the ACK request
fails, the new credential and pending rotation state remain persisted; starting
the normal Agent resumes ACK using the new credential before Control Poll.

If the delivery expires before credential switch, the old credential remains
active locally and the expired staged delivery is discarded. If expiry occurs
after ACK has started, the result may be ambiguous (Central may have processed
ACK while the response was lost); both credentials are retained and the Agent
pauses Control Poll pending Central-side recovery. Do not force another ACK or
create another delivery until Central confirms the credential state.

The GET payload currently does not include `key_version` or `delivery_status`.
The Agent therefore validates delivery ID, credential ID, token presence, and
expiry from GET, then validates Agent ID, credential ID, positive key version,
active status, and credential expiry from the ACK response. No GET fields are
invented.


## Server Network Metrics (v0.7.2-dev)

The server-level collector now reads Linux `/proc/net/dev` counters and records receive/transmit bytes, packets, errors, and dropped packets per non-loopback interface. Loopback is excluded. These are cumulative kernel counters, not calculated rates. Collection remains behind the existing Runtime Activation Gate and the resulting snapshot is persisted to the bounded local spool.

This milestone does not add telemetry upload, application-level collectors, batching, or an ingestion contract. The Central telemetry endpoint and request/acknowledgement schema must be verified before transport is implemented.


## v0.7.3-dev — Process Inspection and Nginx Monitoring

This development milestone adds a reusable Linux process inspection package and a read-only Nginx monitoring collector. Nginx data is embedded in the existing server metrics snapshot and therefore follows the existing Activation Gate and bounded local spool path.

Collected Nginx information includes:

- Process status (`running`, `stopped`, `not_detected`, or `unknown`).
- systemd service state when `systemctl` is available. The collector only runs `systemctl show`; it does not change service state.
- Master and worker process counts.
- Per-process resident memory and cumulative CPU time ticks from `/proc`. CPU percentage is intentionally not fabricated or inferred from a single sample.
- Collector timestamp and partial/unavailable status.
- HTTP/HTTPS availability and Nginx metrics endpoint are explicitly reported as unavailable until an approved endpoint is configured.

The collector does not edit Nginx configuration, enable `stub_status`, install exporters, probe unconfigured URLs, reload/restart Nginx, or perform remediation.

## Laravel Application Monitoring (v0.7.4-dev)

The Agent now includes a per-application HTTPS client for the enrolled Laravel Bridge endpoint. It uses each application's own stored application credential and the latest verified Activation Grant capabilities. The Agent credential is never used for Bridge requests.

Implemented Bridge collection:

- `/_nadi/v1/health` when `application_metrics` is granted.
- `/_nadi/v1/info` when `application_metrics` is granted, including Laravel and PHP versions.
- `/_nadi/v1/database` when `database_monitoring` is granted.

The client requires HTTPS, verifies TLS certificates, rejects redirects, applies the configured HTTP timeout, and limits response bodies. Per-application records are stored under `spool/applications/` with a hashed application-ID filename and restrictive filesystem permissions. Each application has an independent spool and acknowledgement/clear operations.

In Laravel Bridge v0.3.1-dev, `/metrics`, `/queue`, and `/scheduler` return HTTP 501. The Agent reports these capabilities as `unsupported`; it does not invent metrics or modify the Bridge. Queue worker attribution also requires an application filesystem path or another verified application identity source, which is not present in the current Agent enrollment state.

This milestone still does not upload telemetry to Central. The Central ingestion contract, including batch schema, sequence, idempotency, and response acknowledgement, must be available before transport is implemented.
