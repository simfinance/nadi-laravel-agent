<?php

namespace Nadi\LaravelAgent\Installer;

use RuntimeException;

class Preflight
{
    public function inspect(string $applicationPath): array
    {
        $applicationUrl = (string) config('app.url');
        $parsedUrl = parse_url($applicationUrl);
        $phpFpmIdentity = $this->detectPhpFpmRuntimeIdentity();

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
            'php_fpm_runtime_identity' => $phpFpmIdentity !== null,
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
            'php_fpm_identity' => $phpFpmIdentity,
        ];
    }

    public function detectPhpFpmRuntimeIdentity(string $configurationRoot = '/etc/php'): ?array
    {
        if (
            ! function_exists('posix_getpwnam')
            || ! function_exists('posix_getgrnam')
            || ! is_dir($configurationRoot)
            || is_link($configurationRoot)
        ) {
            return null;
        }

        $poolFiles = glob(
            rtrim($configurationRoot, DIRECTORY_SEPARATOR)
            . DIRECTORY_SEPARATOR
            . '*'
            . DIRECTORY_SEPARATOR
            . 'fpm'
            . DIRECTORY_SEPARATOR
            . 'pool.d'
            . DIRECTORY_SEPARATOR
            . '*.conf'
        );

        if (! is_array($poolFiles) || $poolFiles === []) {
            return null;
        }

        $identities = [];

        foreach ($poolFiles as $poolFile) {
            clearstatcache(true, $poolFile);

            if (is_link($poolFile) || ! is_file($poolFile) || ! is_readable($poolFile)) {
                return null;
            }

            $contents = file_get_contents($poolFile);

            if (! is_string($contents)) {
                return null;
            }

            if (! preg_match(
                '/^[\t ]*user[\t ]*=[\t ]*([a-zA-Z_][a-zA-Z0-9_.-]*)[\t ]*$/m',
                $contents,
                $userMatch
            )) {
                return null;
            }

            $userName = $userMatch[1];
            $user = posix_getpwnam($userName);

            if (! is_array($user) || ! isset($user['uid'], $user['gid']) || $user['uid'] < 1) {
                return null;
            }

            if (preg_match(
                '/^[\t ]*group[\t ]*=[\t ]*([a-zA-Z_][a-zA-Z0-9_.-]*)[\t ]*$/m',
                $contents,
                $groupMatch
            )) {
                $groupName = $groupMatch[1];
                $group = posix_getgrnam($groupName);

                if (! is_array($group) || ! isset($group['gid']) || $group['gid'] < 1) {
                    return null;
                }

                $groupId = (int) $group['gid'];
            } else {
                $groupId = (int) $user['gid'];
                $group = posix_getgrgid($groupId);

                if (! is_array($group) || ! isset($group['name']) || $groupId < 1) {
                    return null;
                }

                $groupName = (string) $group['name'];
            }

            $userId = (int) $user['uid'];

            if ($userId < 1 || $groupId < 1) {
                return null;
            }

            $identityKey = $userId . ':' . $groupId;
            $identities[$identityKey] = [
                'user' => $userName,
                'group' => $groupName,
                'uid' => $userId,
                'gid' => $groupId,
            ];
        }

        if (count($identities) !== 1) {
            return null;
        }

        return array_values($identities)[0];
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
