# Agent-to-Bridge Application Credential Handoff

Status: source protocol implemented across the Agent broker and Bridge authorization path. This document records the contract; it does not claim that the privileged helper is packaged, installed, or production-enabled.

## Purpose

The Go Agent receives a Central-issued application credential during Agent enrollment and persists it in its private state. The Laravel Bridge needs the same application credential to authorize requests to its private `/_nadi/v1` endpoints.

The Bridge must not read, parse, copy, or otherwise inspect the Go Agent `state.json`. The Agent state contains Agent credentials, application credentials, activation data, and other sensitive control state.

## Security principals

- `nadi-agent`: non-root systemd service identity. It is the only authorized credential producer.
- Laravel application runtime user: owns and reads only its own Bridge credential file.
- Root-owned handoff broker: validates the Agent peer identity and writes credentials only to pre-registered Bridge credential destinations.
- NADI Central: authoritative issuer of application credentials and application approval/activation state.

The handoff broker is a separately installed root-owned component. It is not shipped or bootstrapped through Composer and is not a general-purpose file-writing service.

## Transport

Use a local Unix-domain stream socket at:

```text
/run/nadi/credential-handoff.sock
```

The socket is created by a root-owned systemd socket unit. The broker must obtain peer credentials from the kernel (for Linux, `SO_PEERCRED`) and accept requests only from the configured `nadi-agent` UID. Do not trust a UID, username, PID, or role asserted in the JSON request.

The socket directory and socket must not be writable by untrusted users. The broker must reject symlinked socket paths and must bind only the fixed path above.

The protocol is one bounded UTF-8 JSON request per connection, with a maximum request size of 16 KiB and a maximum response size of 4 KiB. Require exactly one JSON object and reject duplicate keys, unknown fields, trailing values, unsupported protocol versions, and malformed UTF-8. Close the connection after one request and response. No network listener is permitted.

## Request contract

Protocol version 1 request:

```json
{
    "protocol_version": 1,
    "operation": "publish_application_credential",
    "application_id": "central-application-id",
    "application_key": "stable-bridge-application-key",
    "credential": "central-issued-application-credential"
}
```

The broker must:

1. Authenticate the peer using kernel-provided Unix socket credentials.
2. Validate that `application_id` and `application_key` match one pre-registered application mapping created during installation.
3. Reject unknown, duplicate, or ambiguous application mappings.
4. Validate the credential as a non-empty bounded opaque secret; do not assume its format beyond the Central contract.
5. Never log the request body, credential, or credential-derived values.
6. Atomically write the credential to the exact registered destination for that application only.
7. Set owner to the registered Laravel runtime UID and GID; set file mode to `0600`. The PHP-FPM runtime must be able to read the credential as its own user. Do not introduce a separate Bridge group unless a separately reviewed design requires it.
8. Use a same-directory temporary file, restrictive mode from creation, fsync, atomic rename, and directory fsync.
9. Avoid following symlinks at the destination and reject unexpected pre-existing file types.
10. Return only a bounded status and non-secret application identifier.

The broker must not accept a destination path, owner, group, mode, filename, command, or arbitrary operation from the Agent request. Those values come only from root-owned installation metadata.

## Response contract

Success:

```json
{
    "protocol_version": 1,
    "success": true,
    "application_id": "central-application-id",
    "credential_version": 1
}
```

Failure:

```json
{
    "protocol_version": 1,
    "success": false,
    "error_code": "application_mapping_not_found"
}
```

Error responses must not contain secrets, file contents, filesystem paths, stack traces, or raw request data.

## Application mapping

The privileged installer creates a root-owned mapping for each Laravel application. It binds:

- Central `application_id`
- stable `application_key`
- canonical Laravel base path
- Laravel runtime UID and GID
- exact credential destination under that application's private storage

The mapping is not writable by the Laravel runtime user or `nadi-agent`. The broker must canonicalize and validate the base path at registration time and must not derive a destination path directly from request data.

The credential destination must be outside the public webroot. The Bridge configuration's `nadi-agent.credential_file` must resolve to this exact path.

## Lifecycle and revocation

- Publish only credentials returned by a successful Central enrollment or an authenticated Central credential rotation.
- A repeated publish for the same application replaces the previous credential atomically.
- A rejected, unapproved, suspended, or revoked application must not be made operational merely because a credential exists.
- Credential publication does not open the Bridge activation gate.
- Bridge request authorization must remain fail-closed when `nadi-agent.enabled` is false, the source IP is not allowlisted, or the credential file is absent/unreadable.
- Activation state must be enforced independently of credential presence. Credential possession alone is not proof of application approval or monitoring activation.
- Credential rotation must publish the new credential only after the Agent's rotation protocol reaches the appropriate accepted state; retain no unnecessary historical copies.

## Audit and tests

Required tests include:

- Reject non-`nadi-agent` peer UIDs.
- Reject invalid JSON, duplicate keys, trailing JSON, unknown fields, oversized requests, and unknown operations.
- Reject unknown or mismatched application IDs/keys.
- Reject symlinked destination paths and non-regular existing files.
- Verify atomic replacement, owner, group, and mode.
- Verify no secret appears in logs, process arguments, environment variables, responses, or error messages.
- Verify an application credential cannot be published to another application's destination.
- Verify credential publication alone does not enable Bridge routes or monitoring.
- Verify revocation and rotation behavior.

## Integration gate

This protocol requires coordinated implementation in the Go Agent and the separately installed privileged broker. The Laravel Bridge repository alone cannot safely complete the producer, peer-authentication, and root-owned destination mapping.

The Agent producer and root-owned broker source, Bridge-side credential authorization, and activation-state checks are implemented on the current feature branches. The privileged lifecycle now provisions the broker mapping after confirmed enrollment. Remaining integration gates include trusted helper packaging/installation, final per-application Bridge environment identity provisioning, root/non-root and systemd tests, credential rotation/revocation validation, and full end-to-end tests. Until those gates pass, the customer installer must remain fail-closed and must not claim production-ready secure enrollment.


## Installer-managed Bridge application configuration — 27 November 2026

ADR-0003 defines the external per-application configuration contract. The trusted Agent helper writes one root-owned JSON file at:

`/var/lib/nadi-bridge/config/<sha256(canonical-application-base-path)>.json`

The Laravel Service Provider derives this exact filename from the canonical application base path and reads it without requiring changes to the application's `.env`, Laravel config, PHP-FPM pool, or Nginx configuration.

The file contains only the Central-issued application ID/key, exact source-IP allowlist, enabled state, and fixed credential/activation-state paths. It contains no credential or grant payload. Ownership is `root:<application PHP-FPM primary GID>`, mode `0640`; the config directory is `root:root`, mode `0711`. PHP-FPM primary GIDs must be unique across applications on the same Agent so one runtime cannot read another application's config.

The installer prompts for exact Central source IP addresses and requires the operator to confirm they were verified by infrastructure against the actual source/NAT address. It accepts exact IPv4/IPv6 literals only; it does not infer addresses from DNS or the Central URL. Missing or invalid input stops before enrollment is submitted.

Missing, unreadable, malformed, mismatched, symlinked, or incorrectly owned configuration keeps the Bridge disabled. Credential authorization, source-IP allowlist, and the fresh acknowledged Activation Gate remain independent checks.
