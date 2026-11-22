# Private Composer distribution — NADI Laravel Agent

## Decision

The package is distributed as the VCS package `nadi/laravel-agent` from the
private GitHub repository `simfinance/nadi-laravel-agent`, indexed by Private Packagist.
Private Packagist is the Composer registry; GitHub remains the source of truth.
No ZIP or Agent binary is uploaded manually to a customer server.

## Publisher setup (one-time)

1. Create or select the NADI organization in Private Packagist.
2. Connect the GitHub organization/account that owns `simfinance/nadi-laravel-agent`.
3. Add the private GitHub repository `simfinance/nadi-laravel-agent` as a package.
4. Confirm the detected Composer package name is exactly `nadi/laravel-agent`.
5. Enable automatic updates/webhooks for Git tags and commits.
6. Grant package read access only to the intended deployment/customer identities.
7. Do not add a public mirror or publish the proprietary package to packagist.org.

Private Packagist UI labels and available plans can change; confirm the current
organization/repository integration in the account before enabling production.

## Consumer Composer configuration

Private package access requires both repository discovery and authentication.
For a managed deployment, provision these once in the deployment user's
Composer home (or inject equivalent `COMPOSER_REPOSITORIES` / `COMPOSER_AUTH`
configuration through the deployment system). Do not put tokens in the project
repository, `.env`, shell history, or application files.

Repository URL pattern:

```text
https://repo.packagist.com/<PRIVATE_PACKAGIST_ORGANIZATION>/
```

The exact organization slug, authentication username, and token must be copied
from the NADI Private Packagist account. Never guess or commit them.

Example one-time global setup, performed by the server/deployment administrator
rather than by the Laravel application user:

```bash
composer config --global repositories.nadi composer https://repo.packagist.com/<PRIVATE_PACKAGIST_ORGANIZATION>/
composer config --global --auth http-basic.repo.packagist.com <PRIVATE_PACKAGIST_USERNAME> <PRIVATE_PACKAGIST_TOKEN>
composer config --global repositories.packagist.org false
```

Use an organization **update** token (scoped to the required package/team) or an
authorized user token for the initial `composer require`, because this command
updates the dependency graph. Private Packagist read-only organization tokens
are intended for installs using an existing `composer.lock`, not for a fresh
`composer require`. Store the generated username/token exactly as shown on
the Private Packagist authentication page; do not assume the username value. Prefer
an ephemeral deployment secret and restrict the Composer home files to the
owning deployment user. Do not copy the token into the Laravel project.

Once the repository and authentication are pre-provisioned, the application
installation interface remains exactly:

```bash
composer require nadi/laravel-agent
php artisan nadi:install
```

Important: a private Composer repository cannot be consumed anonymously. If a
server has neither global repository configuration nor valid read credentials,
`composer require` cannot discover or download this private package. This is a
registry access prerequisite, not an extra NADI application installation step.

## Versioning

- Composer package versions are Git tags, for example `v0.1.3`.
- Do not add a hard-coded `version` field to `composer.json`; Composer derives
  the version from the VCS tag.
- Keep the Composer package version separate from the embedded Agent version.
- Create immutable tags; never move or overwrite a released tag.
- Private Packagist should index tags automatically through its Git integration.

## Package archive contents

`.gitattributes` and `archive.exclude` keep development-only Go source, duplicate
binary paths, tests, scripts, CI files, and internal documentation out of the
Composer dist archive. Runtime PHP files and the two packaged Agent architecture
assets under `resources/agent/` remain in the archive.

## Current release blockers

This integration kit does not make the candidate production-ready. Before a
production tag is created, the package must also contain a valid embedded
release public key, a signed immutable release manifest, and a complete
installer that verifies the signature and artifact digest before provisioning.
The current `config/nadi-agent.php` release public key is empty and
`nadi:install` intentionally fails closed. Do not publish a production tag or
claim the two-command installation is operational until those blockers pass.
