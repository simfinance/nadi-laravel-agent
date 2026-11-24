# Privileged Provisioning Boundary

Status: Fixed three-operation protocol accepted and Laravel-side adapter implemented. The root-owned helper remains a separately installed prerequisite and is not shipped or bootstrapped by this Composer package.

## Security boundary

The Laravel application and Composer package run as the application deployment user. They must never execute the Laravel application, Composer scripts, or package-owned writable files as root.

System provisioning is performed only by a separately installed, root-owned helper at:

```text
/usr/libexec/nadi/nadi-provision
```

The helper is installed and maintained by the server administrator or a trusted OS provisioning channel. The Composer package must not create, replace, or bootstrap this helper through sudo.

The administrator must create a dedicated sudoers entry for the exact Unix account that runs `php artisan nadi:install`. Example template (replace `<installer-user>` with the verified account; do not copy the placeholder literally):

```sudoers
<installer-user> ALL=(root) NOPASSWD: /usr/libexec/nadi/nadi-provision --request-stdin
```

The sudoers file must be root-owned, mode `0440`, and validated with `visudo -cf`. Do not grant a wildcard, shell, general root command, or arbitrary helper argument. The Bridge adapter uses `sudo -n`; if this exact authorization is absent, installation stops without trying an interactive sudo prompt.

The helper must be owned by root, must not be writable by group or other users, and must not resolve through a symbolic link. The sudoers policy must authorize only this exact helper invocation, without a shell, wildcard command, or arbitrary executable argument.

## Invocation protocol

The Bridge may invoke the helper only through a fixed executable argument vector equivalent to:

```text
/usr/bin/sudo -n -- /usr/libexec/nadi/nadi-provision --request-stdin
```

No shell is used. The request is one bounded JSON document on standard input. The helper must reject unknown fields, unknown operations, duplicate JSON keys, trailing JSON values, oversized input, and unsupported protocol versions.

Protocol version 1 uses three operation-specific request envelopes over the same fixed helper invocation. Each request is a separate bounded JSON document on standard input. The helper rejects unknown fields, unknown operations, duplicate JSON keys, trailing JSON values, oversized requests, and unsupported protocol versions.

### `install_agent` — install only

```json
{
    "protocol_version": 1,
    "operation": "install_agent",
    "release": {
        "manifest_base64": "<exact signed manifest bytes>",
        "signature_base64": "<detached Ed25519 signature>",
        "artifact_path": "<private staging file path>"
    },
    "applications": [
        {
            "application_key": "<stable application key>",
            "base_path": "<canonical Laravel application path>",
            "runtime_uid": 1001,
            "runtime_gid": 1001
        }
    ]
}
```

This operation never receives the enrollment token or enrollment manifest. It installs trusted assets and a root-owned, non-secret application inventory, then runs `systemctl daemon-reload`. It does not enroll, enable, or start services.

### `enroll_agent` — one enrollment for the complete inventory

```json
{
    "protocol_version": 1,
    "operation": "enroll_agent",
    "enrollment": {
        "manifest_base64": "<base64 JSON manifest containing all 1-20 applications>",
        "token": "<one-time enrollment token>"
    }
}
```

The token is present only in the request standard input and is forwarded only to the fixed Agent binary's standard input. The helper persists a root-owned non-secret enrollment-attempt marker before invoking Agent. An uncertain attempt without a verified completion record is non-retryable and requires reconciliation.

### `start_agent` — fixed service activation

```json
{
    "protocol_version": 1,
    "operation": "start_agent"
}
```

No caller-selected service, unit, path, command, user, group, or file mode is accepted. The helper verifies the root-owned enrollment/broker completion record and Agent state file metadata, then enables and starts only the fixed NADI broker and Agent units.

The Bridge preflight discovers PHP-FPM pool identities from `/etc/php/*/fpm/pool.d/*.conf` and resolves one non-root UID/GID pair for each application target. The current application may infer its identity only when all discovered pools use one unique identity; otherwise its `nadi-agent.runtime_user` and optional `runtime_group` must be configured. Every additional application must declare `base_path`, `base_url`, `application_name`, `environment`, `runtime_user`, and `runtime_group` under `nadi-agent.additional_applications`. Each application must resolve to a distinct non-root runtime UID when per-application credential isolation is required. Shared UIDs represent a shared trust boundary, not isolated credentials. The example UID/GID 1001 is illustrative only. The helper must independently verify that each supplied runtime UID/GID still matches a PHP-FPM pool before registering the Bridge destination.

The helper must independently verify the exact manifest bytes, detached Ed25519 signature, trusted release key ID and fingerprint, selected Linux architecture, artifact SHA-256, and release version before installing any binary. It must open the staged artifact without following symlinks, copy it into a root-owned temporary file, and verify the copied bytes before atomic installation. It must not trust a digest supplied independently of the signed manifest.

The Activation Grant public key and systemd unit are trust-sensitive resources. The helper must install known-good copies from its own root-owned build, not copy these files from the writable Laravel package tree.

## Allowed provisioning effects

The helper's `install_agent` operation may only:

- Create the dedicated `nadi-agent` system user and group if absent, after validating that any existing account matches the expected non-login service-account policy.
- Create and secure the canonical Agent directories `/opt/nadi`, `/etc/nadi`, `/var/lib/nadi`, `/var/log/nadi`, and `/run/nadi`, as required by Master Architecture section 10.4. The Bridge credential root `/var/lib/nadi-bridge/{application_id}` remains a separate per-application security boundary.
- Install the independently verified Agent binary atomically as `/opt/nadi/bin/nadi-agent`.
- Install the helper-bundled Activation Grant public key and systemd unit.
- Write a strictly validated Agent environment file containing only non-secret configuration.
- Run `systemctl daemon-reload`.
- Return a bounded machine-readable result without credentials or secret material.

The helper must not accept arbitrary destination paths, arbitrary usernames, arbitrary unit names, arbitrary commands, shell fragments, or caller-selected file modes/ownership.

This operation must not enroll the Agent, start monitoring, collect telemetry, or enable automatic remediation.

## Enrollment and service lifecycle

Enrollment must execute as the dedicated `nadi-agent` user, never as root. The one-time enrollment token must be delivered over standard input, must not appear in argv or environment variables, and must not be logged or persisted by the helper.

After enrollment, the Bridge must verify a successful process exit and a non-secret enrollment result. It must not read, display, return, or copy the Agent `state.json`; that file contains Agent and per-application credentials.

The service may be enabled and started after enrollment succeeds so the Agent can perform Control Poll. Monitoring remains blocked by the Go Agent Runtime Activation Gate until Central approval, a valid signed Activation Grant, and accepted Grant acknowledgement are all present.

The accepted lifecycle exposes separate fixed `install_agent`, `enroll_agent`, and `start_agent` operations. The helper must not accept an arbitrary unit name.

## Unresolved application credential handoff

The Go Agent persists each application's Central-issued application credential in its private Agent state. The Laravel Bridge also requires the corresponding application credential for its private `/_nadi/v1` endpoint.

A secure, narrowly scoped credential handoff from Agent to Bridge has not yet been defined. The implementation must not solve this by reading or parsing `state.json`, exposing credentials in process output, writing credentials into command-line arguments, or placing credentials in environment variables.

This is a release-blocking integration dependency. Define a dedicated authenticated local handoff (for example, a narrowly scoped Unix-domain socket protocol with peer-credential checks and atomic Bridge-side credential storage) before connecting enrollment to the customer installer.

## Fail-closed requirements

Until the separately installed helper, its exact sudoers policy, helper implementation and protocol tests, independent runtime identity verification, enrollment handoff, rollback, and post-install verification are implemented and tested:

- `php artisan nadi:install` verifies and stages the signed Agent release, then fails closed if the root-owned helper is missing or untrusted.
- No sudo command is attempted.
- No Agent enrollment is submitted.
- No systemd service is enabled or started.
- No operational monitoring or telemetry is started.


## Protocol clarification required before helper integration

The protocol v1 example currently places the enrollment token inside the `install_agent` request envelope. However, the allowed-effects section above states that `install_agent` must not enroll the Agent, and the service lifecycle section requires a separate fixed operation for enabling and starting `nadi-agent.service`. These requirements are not yet represented by one consistent operation contract.

Accepted resolution: use three fixed operations under the same exact `--request-stdin` invocation: `install_agent` (verify and install only), `enroll_agent` (run the Agent enrollment as `nadi-agent`, token delivered only on stdin, then consume only the non-secret summary and configure broker mappings), and `start_agent` (enable/start only the fixed NADI services after verifying the registered state and broker mapping). Each operation must have a strict operation-specific schema and bounded outcome response. A failed or uncertain `enroll_agent` result must be non-retryable unless the Agent confirms that the exact persisted request can safely be replayed.

The project owner approved this contract clarification on 2026-11-24. The Agent helper and Bridge adapter are being updated to implement it consistently. The helper remains unavailable to production until trusted packaging, rollback, and integration validation are complete.

The detailed proposed operation contract is recorded in [Agent ADR-0002](https://github.com/simfinance/nadi-agent/blob/feature/bridge-activation-publisher/docs/adr/ADR-0002-provisioning-helper-operation-lifecycle.md). It is accepted for implementation; acceptance does not waive release validation.
