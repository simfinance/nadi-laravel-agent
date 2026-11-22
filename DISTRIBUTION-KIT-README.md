# NADI Laravel Agent Distribution Kit

This repository contains the Laravel Bridge Composer package `nadi/laravel-agent`. It is a maintainer source repository, not a customer ZIP installer.

## Repository boundary

- `simfinance/nadi-agent` remains the standalone Go Agent repository.
- `simfinance/nadi-laravel-agent` contains this Laravel Bridge package and the Agent binaries needed by the package installer.

## Customer installation contract

After NADI maintainers connect this private repository to Private Packagist and provision Composer repository authentication in the deployment environment, customer installation is:

```bash
composer require nadi/laravel-agent
php artisan nadi:install
```

Customers do not upload ZIP files, install Go manually, register Laravel routes manually, or edit package source.

## Maintainer checklist

1. Keep the Go Agent source and its build/release lifecycle in `simfinance/nadi-agent`.
2. Connect this repository to Private Packagist as a private VCS package.
3. Provision a package-scoped Composer credential through deployment secrets or the global Composer home; never commit credentials.
4. Require GitHub Actions validation to pass before tagging.
5. Complete the signed release manifest, trusted public-key integration, secure download/provisioning, enrollment, app-credential handoff, multi-app lifecycle, post-install verification, and rollback before any production release.

The current `nadi:install` command deliberately fails closed. Composer distribution setup is not equivalent to production readiness.
