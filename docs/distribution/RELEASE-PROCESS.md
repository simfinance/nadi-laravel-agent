# NADI Composer package release process

## Source release flow

1. Merge reviewed Laravel Bridge source changes into \`main\` in
   \`simfinance/nadi-laravel-agent\`.
2. Build and approve the Go Agent release in its separate repository,
   \`simfinance/nadi-agent\`, then publish immutable Linux AMD64/ARM64 release
   assets.
3. Ensure the published artifact SHA-256 values match the approved release
   checksums and the package's \`resources/agent/SHA256SUMS.txt\` when bundled
   binaries are included in a package release.
4. Publish the release manifest through the Central release-management flow.
   The manifest must refer only to immutable HTTPS artifact URLs and actual
   published binaries.
5. Keep the Ed25519 release-signing private key under Central-side custody.
   It must never enter Git, GitHub Actions secrets, Composer archives, or
   customer servers.
6. Ensure the package's embedded public key and configured key ID match the
   Central release-signing key.
7. Complete the separate Composer package release gates before creating an
   immutable Composer Git tag.
8. GitHub Actions validates Composer metadata, PHP lint, PHPUnit, trust-anchor
   fingerprint, packaged binary hashes, and Composer dist contents.
9. Private Packagist indexes the package tag through its GitHub integration.
10. Verify the package/version is visible to an authorized consumer.

## Central manifest contract

The Bridge retrieves the manifest using:

\`GET https://nadi.pesantrenalirsyad.org/api/v1/nadi/releases/manifest\`

Central returns the raw JSON manifest in the response body and a detached
Ed25519 signature, Base64 encoded, in the response header:

\`X-NADI-Manifest-Signature\`

The signature is calculated over the exact response-body bytes. The Bridge
must verify those bytes before parsing or trusting manifest fields.

The manifest uses schema version \`1\`, product \`nadi-agent\`, the configured
release key ID, a semantic version, an integer \`protocol_version\`, an ISO 8601
UTC \`released_at\` timestamp, and artifact entries containing OS, architecture,
filename, HTTPS URL, and lowercase SHA-256 digest.

The Bridge currently supports Linux \`amd64\` and \`arm64\`. PHP architecture
names \`x86_64\` and \`aarch64\` are normalized to the corresponding manifest
architecture identifiers.

## Release trust anchor

The package contains only the Ed25519 release public key at
\`resources/agent/release-signing-public.pem\`. The matching private key remains
under Central-side custody. The configured key ID is
\`nadi-agent-release-2026-01\`; CI checks the public key's SHA-256 DER
fingerprint against the approved value.

The Bridge verifier validates the detached signature before parsing JSON,
then validates the manifest schema, release identity, timestamp, artifact
metadata, HTTPS URLs, and SHA-256 format. The artifact downloader verifies the
binary SHA-256 before placing it in private staging. Staged files are kept
non-executable until a separate, approved provisioning flow is implemented.

## Signing boundary

The release-signing private key is never copied to GitHub, GitHub Actions,
Composer archives, or customer servers. Signing is completed through the
approved Central key-custody procedure. The Bridge only receives the public
trust anchor and signed manifest response.

## Installer safety boundary

The current \`nadi:install\` flow performs preflight, verifies the Central
manifest signature, selects the host's Linux architecture, downloads the
matching artifact over HTTPS, and verifies its SHA-256 before placing it in
private staging. It deliberately stops before executing the Agent because the
privileged provisioning adapter and secure enrollment handoff are not yet
connected.

No enrollment, activation, monitoring, or telemetry is started by this
release-download flow. The installer must remain fail-closed until provisioning,
enrollment, credential handoff, repair, update, rollback, and multi-application
tests pass on supported targets.

## Release gates

A tag must not be considered production-ready until all are true:

- CI validation passes.
- Both binary architectures are available and match approved checksums.
- The Central manifest signature verifies against the embedded public key and
  expected key ID.
- Artifact URLs are HTTPS and downloadable by customer servers without
  private GitHub credentials.
- The installer verifies the signed manifest before trusting artifact metadata,
  then verifies each binary SHA-256 before execution.
- Installer provisioning, enrollment, credential handoff, repair, update,
  rollback, and multi-application tests pass on supported targets.
- No Central source/schema/route change is assumed by this Agent release.
