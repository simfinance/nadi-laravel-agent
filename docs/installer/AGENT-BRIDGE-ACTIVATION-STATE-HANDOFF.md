# Agent-to-Bridge Activation State Handoff

Status: design contract only. This document does not implement or enable activation.

## Purpose

The Bridge currently has independent request checks for the global enabled flag, source IP allowlist, and Central-issued application credential. Those checks do not prove that Central has approved the application or that monitoring has been activated.

The Bridge must fail closed until it has a fresh, authenticated activation-state assertion for its own application. Possession of an application credential, a successful health response, Agent enrollment, or a running systemd service is not sufficient.

## Authority and trust model

- NADI Central remains authoritative for approval, monitoring status, and Activation Grant issuance.
- The Go Agent remains responsible for validating the signed Activation Grant, configured activation key ID, grant time window, application identity, required capabilities, and grant acknowledgement.
- The Bridge must not independently infer approval from the presence of a credential or from a successful Agent process.
- The Bridge may accept activation state only through the separately installed root-owned local handoff broker described by the credential handoff architecture.
- The broker must authenticate the `nadi-agent` peer using kernel Unix-socket peer credentials. The Bridge must not accept activation-state writes from an HTTP request or from the Laravel application runtime user.

## State assertion

The broker may publish a per-application activation assertion only after receiving an authenticated state update from the Agent that reflects the Agent Runtime Activation Gate's effective result.

Protocol version 1 state assertion:

```json
{
    "protocol_version": 1,
    "operation": "publish_activation_state",
    "application_id": "central-application-id",
    "application_key": "stable-bridge-application-key",
    "activation_state": "active",
    "monitoring_enabled": true,
    "grant_acknowledged": true,
    "capabilities": [
        "application_metrics",
        "database_monitoring"
    ],
    "observed_at": "2026-11-22T07:00:00Z",
    "valid_until": "2026-11-22T07:05:00Z"
}
```

Allowed `activation_state` values are:

- `pending_approval`
- `pending_activation`
- `active`
- `suspended`
- `revoked`
- `unknown`

The broker must validate the schema, application mapping, bounded capability list, timestamp syntax, maximum assertion lifetime, and monotonic update/version semantics. The Agent peer identity is authenticated by the socket, not by fields in the message.

The assertion is an internal status handoff, not a replacement Activation Grant and not a new Central credential. The broker must never accept a caller-supplied assertion as proof of Central approval unless the Agent has authenticated that state through its existing Control Poll and Runtime Activation Gate logic.

## Storage and Bridge consumption

The broker writes one state file per registered application under a fixed root-owned runtime directory, for example:

```text
/run/nadi/bridge-state/<application-key>.json
```

The actual filename and Bridge-readable access mechanism must be fixed by root-owned application mapping, not chosen by request input. The broker must use atomic replacement, reject symlinks and unexpected file types, and never expose secrets in this state file.

The Bridge reads only its own configured state file. It must validate the schema, application identity, state value, timestamps, and freshness on every authorization decision or through a cache whose maximum lifetime does not exceed the assertion validity window. Missing, malformed, stale, or mismatched state is inactive.

A positive authorization decision requires all of the following:

- `activation_state` equals `active`.
- `monitoring_enabled` is exactly `true`.
- `grant_acknowledged` is exactly `true`.
- `valid_until` is in the future and `observed_at` is not unreasonably in the future.
- The requested endpoint capability is present in the assertion.
- The assertion application identity matches the current Bridge installation.

An empty capability list grants no endpoint access. Capabilities must be allowlisted by the Bridge implementation; unknown capabilities are ignored or cause rejection, never expanded into access.

## Capability mapping

Central Activation Grant capability names are the canonical values carried in the activation assertion. The Bridge performs an explicit, allowlisted translation to endpoint capabilities:

| Central grant capability | Bridge endpoint capabilities |
| --- | --- |
| `application_metrics` | `health`, `metrics`, `info` |
| `database_monitoring` | `database` |
| `queue_monitoring` | `queue` |
| `scheduler_monitoring` | `scheduler` |
| `application_logs` | `application_logs` |
| `server_metrics` | No Bridge endpoint access |
| `system_logs` | No Bridge endpoint access |

No capability is inferred from another capability. `server_metrics` is server-scoped and never authorizes a Laravel Bridge endpoint. Unknown Central capabilities deny the assertion. The Bridge reader maps only the exact Central capability names above.

The Agent and Bridge must use this same mapping. The broker stores and republishes the Central grant capability list unchanged; it must not broaden or translate permissions.


## Fail-closed behavior

- No activation assertion: deny.
- Unknown or pending state: deny.
- Suspended or revoked state: deny.
- Monitoring disabled: deny.
- Grant not acknowledged: deny.
- Expired or stale assertion: deny.
- Application identity mismatch: deny.
- Capability absent or unknown: deny.
- Broker or runtime state unavailable: deny.

The middleware should return a generic non-secret authorization error. It must not disclose grant payloads, internal paths, Central responses, or Agent state.

The Bridge's existing global enabled flag, source-IP allowlist, and constant-time application credential comparison remain additional independent checks. Activation does not replace any of them.

## Revocation and freshness

The Agent must publish state changes after each successful Control Poll and whenever the effective Activation Gate changes. A transition away from active must be published immediately. The broker must atomically replace the previous state.

The Bridge must treat assertions as leases, not permanent authorization. The maximum validity period is five minutes for protocol version 1. A stale active assertion must automatically become inactive even if no explicit revocation message arrives.

The Agent must not extend `valid_until` beyond the freshness interval merely because its local process remains running. Only a fresh authenticated Central Control Poll and a still-valid Activation Grant can refresh the active assertion.

## Required tests

- Default configuration denies every endpoint when no assertion exists.
- Missing, malformed, stale, expired, future-dated, or identity-mismatched assertion is denied.
- Pending, suspended, revoked, monitoring-disabled, and unacknowledged states are denied.
- Active state with a missing endpoint capability is denied.
- Active state with the matching capability proceeds to the existing independent IP and credential checks.
- A valid credential without active state is denied.
- Active state without a valid credential is denied.
- Active state without an allowed source IP is denied.
- State replacement is atomic and stale state expires without a revocation event.
- No activation payload, credential, or internal filesystem path appears in responses or logs.

## Integration gate

The Go Agent must expose the effective Runtime Activation Gate state through the authenticated local handoff. The broker must validate and publish that state. The Bridge must implement and test the capability-aware gate.

Until all three parts and end-to-end tests exist, the installer must remain fail-closed. This document alone does not authorize enabling Bridge endpoints or starting monitoring.
