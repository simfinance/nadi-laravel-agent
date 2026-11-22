# NADI Agent + Laravel Bridge Composer Integration

This directory is an integration candidate assembled from the Agent v0.7.5-dev
and Laravel Bridge v0.3.1-dev source packages.

## Package identity

The repository root is the Composer package `nadi/laravel-agent`.
The Go Agent source remains under `source/`; the prebuilt Linux binaries are
bundled under `resources/agent/` for later signed-release verification.

## Current status

This integration is a source-layout step only. It is NOT a production installer.
The existing `nadi:install` command intentionally fails closed until all of the
following are implemented and tested:

1. A signed release manifest generated with the dedicated Ed25519 release key.
2. Verification of the manifest signature and binary SHA-256 before execution.
3. A narrow, auditable OS provisioning adapter and systemd lifecycle.
4. Secure enrollment and per-application credential handoff.
5. Multi-application registration and lifecycle handling.
6. End-to-end tests on supported Laravel/PHP/OS combinations.

No Central source, schema, route, or infrastructure is modified by this package.

## Intended application workflow

```bash
composer require nadi/laravel-agent
php artisan nadi:install
```

The workflow above is the target interface, not a claim that this candidate is
already installable in production. Private Composer repository distribution and
package authentication must also be configured by the package publisher.

## Architecture layout

- `source/`: Go Agent source code.
- `resources/agent/linux-amd64/`: Linux x86_64 Agent binary.
- `resources/agent/linux-arm64/`: Linux ARM64 Agent binary.
- `src/`, `config/`, `routes/`, `tests/`: Laravel Bridge Composer package.
