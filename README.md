# NADI Laravel Agent Bridge

Private Composer package that integrates a Laravel application with the NADI Agent and NADI Central.

## Package

- Composer name: `nadi/laravel-agent`
- Laravel auto-discovery provider: `Nadi\\LaravelAgent\\NadiAgentServiceProvider`
- Installer command: `php artisan nadi:install`
- Supported PHP baseline: PHP 8.2+
- Bundled Agent targets: Linux AMD64 and Linux ARM64

## Installation

The package is distributed through the private NADI Composer repository. The deployment environment must be provisioned with the repository endpoint and Composer authentication before running:

```bash
composer require nadi/laravel-agent
php artisan nadi:install
```

Do not upload this repository or its ZIP archive to a customer application server. Customers install the package through Composer; Agent binaries are delivered as package resources.

## Package layout

- `src/`: Laravel Bridge, installer, security verifier, and monitoring code.
- `config/nadi-agent.php`: package configuration.
- `routes/api.php`: Bridge API routes.
- `resources/agent/`: versioned Linux Agent binaries and SHA-256 manifest.
- `tests/`: package tests (excluded from Composer dist).
- `docs/distribution/`: maintainer-only Private Packagist and release procedures.

## Release safety

The installer intentionally fails closed until the signed release downloader, privileged provisioning adapter, enrollment flow, secure application-credential handoff, and rollback/post-install verification are complete. A successful Composer installation does not mean the Agent installer is production-ready.

Never commit release-signing private keys, activation credentials, enrollment tokens, Agent credentials, or customer secrets to this repository.

## Maintainer validation

```bash
composer validate --no-check-publish
bash scripts/verify-package-artifacts.sh
composer update --prefer-dist --no-interaction
composer lint
composer test
composer archive --format=zip
```

See `docs/distribution/PRIVATE-PACKAGIST-SETUP.md` and `docs/distribution/RELEASE-PROCESS.md` for publisher operations.
