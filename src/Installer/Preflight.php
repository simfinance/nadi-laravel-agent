<?php

namespace Nadi\LaravelAgent\Installer;

use RuntimeException;

class Preflight
{
    public function inspect(string $applicationPath): array
    {
        $applicationUrl = (string) config('app.url');
        $parsedUrl = parse_url($applicationUrl);

        $checks = [
            'php' => PHP_VERSION_ID >= 80200,
            'linux' => PHP_OS_FAMILY === 'Linux',
            'application_path' => is_dir($applicationPath) && is_file($applicationPath . '/artisan'),
            'application_url_https' => is_array($parsedUrl)
                && ($parsedUrl['scheme'] ?? null) === 'https'
                && ! empty($parsedUrl['host']),
            'application_name' => trim((string) config('app.name')) !== '',
            'application_environment' => trim((string) config('app.env')) !== '',
            'systemd' => is_executable('/usr/bin/systemctl') || is_executable('/bin/systemctl'),
            'sodium' => extension_loaded('sodium'),
            'posix' => extension_loaded('posix'),
        ];

        $failed = array_keys(array_filter(
            $checks,
            static fn (bool $passed): bool => ! $passed
        ));

        return [
            'checks' => $checks,
            'passed' => $failed === [],
            'failed' => $failed,
            'architecture' => php_uname('m'),
            'hostname' => gethostname() ?: 'unknown',
        ];
    }

    public function assertSupported(array $result): void
    {
        if (! $result['passed']) {
            throw new RuntimeException(
                'NADI preflight failed: ' . implode(', ', $result['failed'])
            );
        }

        if (! in_array($result['architecture'], ['x86_64', 'aarch64', 'arm64'], true)) {
            throw new RuntimeException(
                'Unsupported CPU architecture: ' . $result['architecture']
            );
        }
    }
}
