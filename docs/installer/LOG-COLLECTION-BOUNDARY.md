# Log Collection Boundary: Laravel Application and Nginx

Status: Laravel application error summary is implemented in the Bridge. Nginx error-log collection remains an Agent/System Log Engine integration dependency.

## Laravel application error log

The Bridge exposes the private endpoint:

```text
GET /_nadi/v1/logs/application
```

The endpoint is protected by the existing application credential, source-IP allowlist, and the per-application activation gate. It requires the `application_logs` capability.

The reader resolves the active Laravel logging channel and supports `single`, `daily`, and `stack` channels containing those drivers. It only reads regular files inside the application's resolved `storage/logs` directory. It rejects paths outside that directory and symbolic-link log files.

Resource limits:

- Maximum 1 MiB read from the end of the log file.
- Maximum 100 error records per response.
- Default response limit is 50 records.
- Only ERROR, CRITICAL, ALERT, and EMERGENCY records are returned.
- Raw log messages and stack traces are never returned.
- The response contains timestamp, severity, exception type when recognized, and a stable non-secret fingerprint.

The reader does not create a Laravel health probe or run before activation. It does not change the application's logging configuration.

## Nginx error log

Nginx error logs are server-level logs and must be collected by the Go NADI Log Engine, not by PHP-FPM or the Laravel Bridge. The Bridge must not attempt to read `/var/log/nginx` directly: the application runtime normally has no permission to read those files, and granting the PHP-FPM user broad log-reading privileges would violate least privilege.

The Agent-side collector must:

- Read only administrator-registered Nginx error-log paths; never recursively scan `/var/log`.
- Use read-only access and bounded tail reads.
- Avoid Nginx configuration changes, reloads, or service restarts.
- Redact secrets, query strings, credentials, and personal/business data before telemetry leaves the server.
- Bound record size, batch size, and collection frequency.
- Run only when at least one application on the server has a valid active grant and the corresponding `system_logs` capability is granted.
- Stop collection when all eligible application grants are inactive, expired, suspended, or revoked.
- Preserve per-application isolation and avoid assigning another application's grant to this server-level data without an explicit Central scope rule.

The current Go Agent Log Engine does not yet have a completed Nginx collector or a privileged, narrowly scoped log-access handoff. Therefore Nginx error logs are not yet available end-to-end. Do not add the `nadi-agent` service user to the broad `adm` group as a workaround.

## Activation and privacy

No application or Nginx log may be read, buffered, spooled, or transmitted before approval and a valid activation grant. The Bridge route-level activation gate enforces this for Laravel logs. The Agent Log Engine must enforce the equivalent gate for Nginx logs.

No raw log line, stack trace, credential, secret, SQL text, request body, or unredacted PII may be included in telemetry.
