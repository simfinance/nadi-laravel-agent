# Privileged Provisioning Boundary

Status: Laravel-side fixed-command adapter implemented. The root-owned helper is still a separately installed prerequisite and is not shipped or bootstrapped by this Composer package.

## Security boundary

The Laravel application and Composer package run as the application deployment user. They must never execute the Laravel application, Composer scripts, or package-owned writable files as root.

System provisioning is performed only by a separately installed, root-owned helper at:

```text
/usr/libexec/nadi/nadi-provision
```

The helper is installed and maintained by the server administrator or a trusted OS provisioning channel. The Composer package must not create, replace, or bootstrap this helper through sudo.

The helper must be owned by root, must not be writable by group or other users, and must not resolve through a symbolic link. The sudoers policy must authorize only this exact helper invocation, without a shell, wildcard command, or arbitrary executable argument.

## Invocation protocol

The Bridge may invoke the helper only through a fixed executable argument vector equivalent to:

```text
/usr/bin/sudo -n -- /usr/libexec/nadi/nadi-provision --request-stdin
```

No shell is used. The request is one bounded JSON document on standard input. The helper must reject unknown fields, unknown operations, duplicate JSON keys, trailing JSON values, oversized input, and unsupported protocol versions.

Protocol version 1 request envelope:

```json
{
    "protocol_version": 1,
    "operation": "install_agent",
    "release": {
        "manifest_base64": "<exact signed manifest bytes>",
        "signature_base64": "<detached Ed25519 signature>",
        "artifact_path": "<private staging file path>"
    },
    "application": {
        "application_key": "<stable application key>",
        "application_name": "<application name>",
        "environment": "<application environment>",
        "base_url": "<HTTPS application URL>"
    }
}
```

The helper must independently verify the exact manifest bytes, detached Ed25519 signature, trusted release key ID and fingerprint, selected Linux architecture, artifact SHA-256, and release version before installing any binary. It must open the staged artifact without following symlinks, copy it into a root-owned temporary file, and verify the copied bytes before atomic installation. It must not trust a digest supplied independently of the signed manifest.

The Activation Grant public key and systemd unit are trust-sensitive resources. The helper must install known-good copies from its own root-owned build, not copy these files from the writable Laravel package tree.

## Allowed provisioning effects

The helper's `install_agent` operation may only:

- Create the dedicated `nadi-agent` system user and group if absent, after validating that any existing account matches the expected non-login service-account policy.
- Create and secure the fixed Agent directories under `/etc/nadi-agent`, `/var/lib/nadi-agent`, and `/opt/nadi/bin`.
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

The helper should expose a separate fixed operation for enabling and starting the known `nadi-agent.service` unit. It must not accept an arbitrary unit name.

## Unresolved application credential handoff

The Go Agent persists each application's Central-issued application credential in its private Agent state. The Laravel Bridge also requires the corresponding application credential for its private `/_nadi/v1` endpoint.

A secure, narrowly scoped credential handoff from Agent to Bridge has not yet been defined. The implementation must not solve this by reading or parsing `state.json`, exposing credentials in process output, writing credentials into command-line arguments, or placing credentials in environment variables.

This is a release-blocking integration dependency. Define a dedicated authenticated local handoff (for example, a narrowly scoped Unix-domain socket protocol with peer-credential checks and atomic Bridge-side credential storage) before connecting enrollment to the customer installer.

## Fail-closed requirements

Until the separately installed helper, its exact sudoers policy, helper implementation and protocol tests, runtime identity discovery, enrollment handoff, rollback, and post-install verification are implemented and tested:

- `php artisan nadi:install` verifies and stages the signed Agent release, then fails closed if the root-owned helper is missing or untrusted.
- No sudo command is attempted.
- No Agent enrollment is submitted.
- No systemd service is enabled or started.
- No operational monitoring or telemetry is started.
