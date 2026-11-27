# ADR-0003: Installer-Managed Per-Application Bridge Configuration

- Status: Accepted
- Date: 2026-11-27
- Scope: NADI Agent provisioning and Laravel Bridge runtime configuration
- Authority: NADI Master Architecture Technical Specification v1.0; NADI Agent-side Monitoring Specification v1.0; owner approval in the NADI project conversation on 2026-11-27
- Central repository: Read-only; no Central API or schema changes

## Context

The Laravel Bridge package currently defaults to disabled operation, an empty source-IP allowlist, and unset per-application identity, credential, and activation-state paths. The Agent provisioning helper already reconciles Central-issued application IDs and keys with the local PHP-FPM inventory and installs the root-owned broker mapping, credential directory, activation-state destination, and exact broker systemd write-path.

The remaining gap is how the Laravel Bridge obtains its own application identity and exact external credential/activation paths without writing to a customer's .env, Laravel config, PHP-FPM pool, Nginx, or other application configuration.

The Master Architecture requires configuration, state, credential, spool, and permission to be separated per application. It also assigns Central source-IP allowlist/NAT verification to customer infrastructure. The Bridge must remain denied when the allowlist or activation assertion is missing.

## Decision

The installer provisions one root-owned JSON configuration file per registered Laravel application. The file is outside the application tree and contains only non-secret Bridge identity and path metadata.

Canonical file path:

/var/lib/nadi-bridge/config/<sha256(canonical-application-base-path)>.json

The Bridge derives the filename from realpath(base_path()). It does not accept a caller-selected path and does not need an application-side environment variable or published Laravel config.

Schema version 1 has exactly these fields:

- schema_version: integer 1
- enabled: boolean; set to true only after confirmed enrollment and successful provisioning
- application_id: Central-issued application UUID
- application_key: stable application key reconciled from the Agent enrollment summary
- allowed_ips: non-empty list of exact IPv4/IPv6 addresses explicitly supplied by the installer operator after infrastructure verifies Central's actual source/NAT addresses; CIDR ranges and DNS-derived values are not accepted
- credential_file: exact canonical path /var/lib/nadi-bridge/<application_id>/credential
- activation_state_file: exact canonical path /run/nadi/bridge-state/<application_id>.json

No credential, token, Activation Grant payload, private key, .env value, or business data is stored in this configuration file.

## Ownership and isolation

- /var/lib/nadi-bridge/config: root:root, mode 0711.
- Each application configuration file: root:<that application's PHP-FPM primary GID>, mode 0640.
- The file is written atomically and journaled by the trusted provisioning helper.
- The Bridge reads the file only when its effective GID matches the file's configured runtime GID; it verifies root ownership, exact mode, regular-file type, non-symlink path ancestry, and opened-descriptor identity.
- PHP-FPM primary GIDs must be distinct across the applications in one Agent inventory. The installer fails closed if GIDs are shared because a shared group would allow one application runtime to read another application's root-owned configuration file.
- The application runtime cannot modify the file or list the configuration directory. It can read only the file whose deterministic path corresponds to its own canonical application base path.

The existing credential remains runtime_uid:runtime_gid, mode 0600; the activation assertion remains root:runtime_gid, mode 0640. No additional Bridge group is created.

## Source-IP allowlist

The installer must request the Central source IP address(es) from the operator and state clearly that the values must first be verified by the customer's infrastructure team against the actual NAT/source address. Values are exact IP literals; the installer validates syntax and uniqueness but does not claim to verify network routing or NAT.

The installer must not infer source IPs from Central DNS, URL resolution, or the configured Central hostname. Missing or invalid allowlist input stops installation before enrollment is submitted. The allowlist remains an independent Bridge authorization check and does not replace application credential or Activation Gate validation.

## Bridge runtime behavior

The Service Provider loads the deterministic external file and overlays the package's runtime configuration for that Laravel process. If the file is missing, unreadable, malformed, has unexpected fields, wrong ownership/mode/GID, a symlinked path, or mismatched fixed paths, Bridge configuration remains disabled and all private endpoints deny access.

The following checks remain independent and mandatory:

1. Bridge enabled configuration.
2. Exact Central source-IP allowlist.
3. Per-application credential file and constant-time credential comparison.
4. Fresh root-owned activation assertion for the same application.
5. Capability authorization for the requested endpoint.

A valid external configuration file or credential alone never opens the Activation Gate.

## Lifecycle, rollback, and RESTORE

- The Agent helper creates the configuration file only after Central enrollment has been confirmed and the application identity has been reconciled.
- The configuration file is included in the enrollment/RESTORE journal, expected-target validation, integrity checks, and rollback fingerprint handling.
- RESTORE must preserve an existing valid file byte-for-byte. It must reject an identity/path/ownership mismatch and must not silently replace a configuration file that differs from the transaction's expected content.
- Enrollment recovery may reconcile only resources proven to belong to its persisted journal. An ambiguous or externally changed resource remains preserved and requires recovery.
- Changing the verified Central source-IP allowlist or PHP-FPM identity is an explicit installer/configuration update decision; it is not silently applied during RESTORE.

## Consequences

- Customer-facing installation remains the approved Composer + Artisan flow.
- No Laravel application configuration, .env, PHP-FPM, Nginx, firewall, Supervisor, or Central code is modified.
- Bridge configuration becomes deterministic and isolated per application.
- Shared PHP-FPM primary GIDs become an explicit installation blocker until infrastructure provides distinct runtime groups.
- Infrastructure remains responsible for verifying the actual Central source/NAT IPs before installation.
- The implementation must add Agent helper tests for generation, journaling, rollback, and RESTORE verification, plus Bridge tests for secure file loading and fail-closed behavior.

## Approval

The project owner approved the installer-managed external per-application configuration approach on 2026-11-27. This ADR records the approved implementation contract; it does not authorize production deployment, sudoers installation, monitoring activation, or changes to Central.
