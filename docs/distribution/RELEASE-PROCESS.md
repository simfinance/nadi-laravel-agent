# NADI Composer package release process

## Source release flow

1. Merge reviewed Laravel Bridge source changes into `main` in
   `simfinance/nadi-laravel-agent`.
2. Build and approve the Go Agent release in its separate repository,
   `simfinance/nadi-agent`, then copy only the intended immutable Linux
   AMD64/ARM64 release binaries into this package's `resources/agent/` tree.
3. Ensure `composer.json` package name remains `nadi/laravel-agent`.
4. Ensure `resources/agent/SHA256SUMS.txt` matches both packaged binaries.
5. Complete the separate release-signing procedure. The Ed25519 private key
   must remain at its approved Central-side custody location and must never
   enter Git, GitHub Actions secrets, Composer archives, or customer servers.
6. Commit the signed manifest, detached signature, and approved public key to
   the package release tree only after the release artifacts are immutable.
7. Create an immutable Composer Git tag only after all production release gates
   have passed.
8. GitHub Actions validates Composer metadata, PHP lint, PHPUnit, trust-anchor
   fingerprint, packaged binary hashes, and Composer dist contents.
9. Private Packagist indexes the Git tag through its GitHub integration.
10. Verify the package/version is visible to an authorized consumer.

## Release trust anchor

The package contains only the Ed25519 release public key at
`resources/agent/release-signing-public.pem`. The matching private key remains
under Central-side custody. The configured key ID is
`nadi-agent-release-2026-01`; CI checks the public key's SHA-256 DER
fingerprint against the approved value.

The Bridge verifier accepts a detached Ed25519 signature encoded as Base64 and
verifies the exact manifest JSON bytes before parsing the JSON. The signed
manifest must identify its `key_id` and include schema version, product,
version, protocol version, release timestamp, and artifact entries with OS,
architecture, filename, HTTPS URL, and SHA-256 digest.

This describes the Bridge verifier's current validation contract. The Central
manifest endpoint, response envelope, signature header/field, exact serialization
rules, and immutable artifact hosting must be confirmed against the actual
Central implementation before the downloader is connected. Do not assume that
a proposed endpoint or header already exists.

## Signing boundary

The release-signing private key is never copied to GitHub, GitHub Actions,
Composer archives, or customer servers. Signing must be completed through the
approved Central key-custody procedure. This repository workflow does not sign
on behalf of that authority.

## Release gates

A tag must not be considered production-ready until all are true:

- CI validation passes.
- Both binary architectures are present and match checksums.
- The signed manifest verifies against the embedded release public key and key ID.
- The installer downloads only HTTPS artifacts and verifies the signed manifest
  before trusting artifact metadata, then verifies each binary SHA-256 before
  execution.
- Installer provisioning, enrollment, credential handoff, repair, update,
  rollback, and multi-application tests pass on supported targets.
- No Central source/schema/route change is assumed by this Agent release.
