# NADI Composer package release process

## Source release flow

1. Merge reviewed source changes into `main` in `simfinance/nadi-agent`.
2. Ensure `composer.json` package name remains `nadi/laravel-agent`.
3. Ensure the packaged AMD64/ARM64 assets are the intended immutable Agent
   release and `resources/agent/SHA256SUMS.txt` matches both files.
4. Complete the separate release-signing procedure. The Ed25519 private key
   must remain at its approved secure custody location and must never enter
   Git, GitHub Actions secrets, Composer archives, or customer servers.
5. Commit the signed manifest, detached signature, and approved public key to
   the package release tree only after the release artifacts are immutable.
6. Create an immutable Composer Git tag such as `v0.1.3`.
7. GitHub Actions validates Composer metadata, PHP lint, PHPUnit, packaged
   binary hashes, and the Composer dist contents.
8. Private Packagist indexes the Git tag through its GitHub integration.
9. Verify the package/version is visible to an authorized read-only consumer.

## Signing boundary

The current project decision keeps the release-signing private key outside the
GitHub repository and under Central-side custody. This workflow deliberately
contains no signing key, no key export, and no automatic signing step. The
signed manifest process must be completed through the approved key-custody
procedure; this repository workflow cannot sign on behalf of that authority.

## Release gates

A tag must not be considered production-ready until all are true:

- CI validation passes.
- Both binary architectures are present and match checksums.
- The signed manifest verifies against the embedded release public key.
- The installer uses the verified manifest and fails closed on any mismatch.
- Installer provisioning, enrollment, credential handoff, repair, update,
  rollback, and multi-application tests pass on supported targets.
- No Central source/schema/route change is assumed by this Agent release.
