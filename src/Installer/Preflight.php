<?php

namespace Nadi\LaravelAgent\Installer;

use RuntimeException;

class Preflight
{
    public function inspect(string $applicationPath, array $applicationTargets = []): array
    {
        $applicationUrl = (string) config('app.url');
        $parsedUrl = parse_url($applicationUrl);
        $phpFpmIdentities = $this->resolveApplicationRuntimeIdentities($applicationTargets);

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
            'php_fpm_runtime_identities' => $phpFpmIdentities !== null,
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
            'php_fpm_identities' => $phpFpmIdentities,
            'php_fpm_identity' => is_array($phpFpmIdentities) && count($phpFpmIdentities) === 1
                ? $phpFpmIdentities[0]
                : null,
        ];
    }

    public function detectPhpFpmRuntimeIdentity(string $configurationRoot = '/etc/php'): ?array
    {
        $identities = $this->readPhpFpmPoolIdentities($configurationRoot);

        if ($identities === null || $identities === []) {
            return null;
        }

        $unique = [];

        foreach ($identities as $identity) {
            $unique[$identity['uid'] . ':' . $identity['gid']] = $identity;
        }

        if (count($unique) !== 1) {
            return null;
        }

        return array_values($unique)[0];
    }

    public function resolveApplicationRuntimeIdentities(
        array $applicationTargets,
        string $configurationRoot = '/etc/php'
    ): ?array {
        if ($applicationTargets === []) {
            $identity = $this->detectPhpFpmRuntimeIdentity($configurationRoot);

            return $identity === null ? null : [$identity];
        }

        $poolIdentities = $this->readPhpFpmPoolIdentities($configurationRoot);

        if ($poolIdentities === null || $poolIdentities === []) {
            return null;
        }

        $resolved = [];
        $seenUIDs = [];

        foreach ($applicationTargets as $index => $target) {
            if (! is_array($target)) {
                return null;
            }

            $runtimeUser = $target['runtime_user'] ?? null;
            $runtimeGroup = $target['runtime_group'] ?? null;

            if ($runtimeUser === null || $runtimeUser === '') {
                $uniquePoolIdentities = [];

                foreach ($poolIdentities as $poolIdentity) {
                    $uniquePoolIdentities[$poolIdentity['uid'] . ':' . $poolIdentity['gid']] = $poolIdentity;
                }

                if (count($uniquePoolIdentities) !== 1) {
                    return null;
                }

                $inferredIdentity = array_values($uniquePoolIdentities)[0];
                $runtimeUser = $inferredIdentity['user'];

                if ($runtimeGroup === null || $runtimeGroup === '') {
                    $runtimeGroup = $inferredIdentity['group'];
                }
            }

            if (
                ! is_string($runtimeUser)
                || ! preg_match('/^[a-zA-Z_][a-zA-Z0-9_.-]*$/D', $runtimeUser)
            ) {
                return null;
            }

            $user = posix_getpwnam($runtimeUser);

            if (! is_array($user) || ! isset($user['uid'], $user['gid'])) {
                return null;
            }

            $uid = (int) $user['uid'];

            if ($uid < 1) {
                return null;
            }

            if ($runtimeGroup === null || $runtimeGroup === '') {
                $group = posix_getgrgid((int) $user['gid']);

                if (! is_array($group) || ! isset($group['name'])) {
                    return null;
                }

                $runtimeGroup = (string) $group['name'];
            }

            if (
                ! is_string($runtimeGroup)
                || ! preg_match('/^[a-zA-Z_][a-zA-Z0-9_.-]*$/D', $runtimeGroup)
            ) {
                return null;
            }

            $group = posix_getgrnam($runtimeGroup);

            if (! is_array($group) || ! isset($group['gid'])) {
                return null;
            }

            $gid = (int) $group['gid'];

            if ($gid < 1) {
                return null;
            }

            $matchingPools = array_filter(
                $poolIdentities,
                static fn (array $identity): bool =>
                    $identity['uid'] === $uid
                    && $identity['gid'] === $gid
            );

            if ($matchingPools === []) {
                return null;
            }

            if (isset($seenUIDs[$uid])) {
                return null;
            }

            $seenUIDs[$uid] = true;
            $resolved[] = [
                'application_key' => (string) ($target['application_key'] ?? ''),
                'base_path' => (string) ($target['base_path'] ?? ''),
                'user' => $runtimeUser,
                'group' => $runtimeGroup,
                'uid' => $uid,
                'gid' => $gid,
                'pool_count' => count($matchingPools),
                'index' => $index,
            ];
        }

        return $resolved;
    }

    private function readPhpFpmPoolIdentities(string $configurationRoot): ?array
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

            if (! is_array($user) || ! isset($user['uid'], $user['gid']) || (int) $user['uid'] < 1) {
                return null;
            }

            if (preg_match(
                '/^[\t ]*group[\t ]*=[\t ]*([a-zA-Z_][a-zA-Z0-9_.-]*)[\t ]*$/m',
                $contents,
                $groupMatch
            )) {
                $groupName = $groupMatch[1];
                $group = posix_getgrnam($groupName);

                if (! is_array($group) || ! isset($group['gid']) || (int) $group['gid'] < 1) {
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

            $identities[] = [
                'user' => $userName,
                'group' => $groupName,
                'uid' => (int) $user['uid'],
                'gid' => $groupId,
                'pool_file' => $poolFile,
            ];
        }

        return $identities;
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

        if (! is_array($result['php_fpm_identities'] ?? null) || $result['php_fpm_identities'] === []) {
            throw new RuntimeException('NADI application PHP-FPM runtime identities could not be verified.');
        }
    }
}
