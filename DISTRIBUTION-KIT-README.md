# NADI Private Composer Distribution Integration Kit

This is a repository-maintainer integration kit for `simfinance/nadi-agent`.
It is NOT a customer/server installation bundle.

## What this kit adds

- Private Packagist VCS distribution instructions for `nadi/laravel-agent`.
- GitHub Actions validation for PHP syntax, PHPUnit, Composer archive layout,
  and AMD64/ARM64 binary checksums.
- Composer dist filtering so customer installs receive Laravel runtime files
  and the two Agent binaries, not Go source, tests, scripts, or CI internals.
- Release process documentation that preserves the Ed25519 private-key
  custody boundary.

## Publisher actions still required

1. Merge this source tree into the private GitHub repository
   `simfinance/nadi-agent` (the repository root must contain `composer.json`).
2. In Private Packagist, connect GitHub and add `simfinance/nadi-agent` as a
   private VCS package. Confirm the package name `nadi/laravel-agent`.
3. Provision a read-only Private Packagist Composer credential and repository
   endpoint through the deployment platform's Composer home/secret injection.
4. Push changes to `main`; GitHub Actions validates the package.
5. Before creating any production Composer tag, complete signed release
   manifest/public-key integration and all installer blockers documented in
   `docs/distribution/`.

Private Packagist account creation, GitHub OAuth authorization, package
registration, and token issuance require an authorized human account action.
This environment has no authenticated GitHub or Private Packagist connector, so
those account operations have not been performed here.

## End-user contract

Once publisher setup and deployment-side Composer authentication are in place,
customer application installation remains:

```bash
composer require nadi/laravel-agent
php artisan nadi:install
```

No customer ZIP upload, binary upload, Go installation, or manual Laravel
route registration is part of this distribution model.

## Important limitation

The current candidate's `nadi:install` intentionally fails closed because the
release public key is empty and the signed release downloader/provisioner is not
complete. This kit establishes the Composer distribution path and CI guardrails;
it does not mark the installer production-ready.
