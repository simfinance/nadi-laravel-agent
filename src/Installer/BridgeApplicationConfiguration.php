<?php

namespace Nadi\LaravelAgent\Installer;

use RuntimeException;
use Throwable;

class BridgeApplicationConfiguration
{
    private const CONFIG_DIRECTORY = '/var/lib/nadi-bridge/config';

    private const MAXIMUM_FILE_SIZE = 16384;

    public function __construct(
        private readonly string $directory = self::CONFIG_DIRECTORY,
        private readonly int $ownerUid = 0,
        private readonly int $directoryUid = 0,
        private readonly int $directoryGid = 0
    ) {
    }

    public function load(string $basePath, ?int $effectiveGid = null): ?array
    {
        if ($effectiveGid === null) {
            if (! function_exists('posix_getegid')) {
                return null;
            }

            $effectiveGid = posix_getegid();
        }

        if ($effectiveGid < 1) {
            return null;
        }

        $canonicalBasePath = realpath($basePath);

        if (
            $canonicalBasePath === false
            || $canonicalBasePath !== $basePath
            || ! str_starts_with($canonicalBasePath, DIRECTORY_SEPARATOR)
        ) {
            return null;
        }

        $configurationPath = $this->pathForBasePath($canonicalBasePath);

        try {
            $contents = $this->readTrustedFile($configurationPath, $effectiveGid);
            $configuration = json_decode($contents, true, 16, JSON_THROW_ON_ERROR);

            if (! is_array($configuration) || array_is_list($configuration)) {
                throw new RuntimeException('Managed Bridge configuration must be a JSON object.');
            }

            $this->validateConfiguration($configuration);

            return [
                'enabled' => $configuration['enabled'],
                'allowed_ips' => $configuration['allowed_ips'],
                'credential_file' => $configuration['credential_file'],
                'activation_state_file' => $configuration['activation_state_file'],
                'activation_application_id' => $configuration['application_id'],
                'activation_application_key' => $configuration['application_key'],
            ];
        } catch (Throwable) {
            return null;
        }
    }

    public function pathForBasePath(string $canonicalBasePath): string
    {
        return rtrim($this->directory, DIRECTORY_SEPARATOR)
            . DIRECTORY_SEPARATOR
            . hash('sha256', $canonicalBasePath)
            . '.json';
    }

    private function readTrustedFile(string $path, int $effectiveGid): string
    {
        $this->assertTrustedDirectory($this->directory);

        clearstatcache(true, $path);

        if (is_link($path)) {
            throw new RuntimeException('Managed Bridge configuration cannot be a symbolic link.');
        }

        $pathStat = @lstat($path);

        if (
            ! is_array($pathStat)
            || (($pathStat['mode'] & 0170000) !== 0100000)
            || $pathStat['uid'] !== $this->ownerUid
            || $pathStat['gid'] !== $effectiveGid
            || (($pathStat['mode'] & 0777) !== 0640)
            || $pathStat['size'] < 2
            || $pathStat['size'] > self::MAXIMUM_FILE_SIZE
            || ($pathStat['nlink'] ?? 1) !== 1
        ) {
            throw new RuntimeException('Managed Bridge configuration metadata is invalid.');
        }

        $handle = @fopen($path, 'rb');

        if ($handle === false) {
            throw new RuntimeException('Managed Bridge configuration cannot be opened.');
        }

        try {
            $handleStat = fstat($handle);

            if (
                ! is_array($handleStat)
                || $handleStat['dev'] !== $pathStat['dev']
                || $handleStat['ino'] !== $pathStat['ino']
                || (($handleStat['mode'] & 0170000) !== 0100000)
                || $handleStat['uid'] !== $this->ownerUid
                || $handleStat['gid'] !== $effectiveGid
                || (($handleStat['mode'] & 0777) !== 0640)
                || $handleStat['size'] < 2
                || $handleStat['size'] > self::MAXIMUM_FILE_SIZE
                || ($handleStat['nlink'] ?? 1) !== 1
            ) {
                throw new RuntimeException('Managed Bridge configuration changed during access.');
            }

            $contents = stream_get_contents($handle, self::MAXIMUM_FILE_SIZE + 1);

            if (
                ! is_string($contents)
                || strlen($contents) < 2
                || strlen($contents) > self::MAXIMUM_FILE_SIZE
            ) {
                throw new RuntimeException('Managed Bridge configuration size is invalid.');
            }

            return $contents;
        } finally {
            fclose($handle);
        }
    }

    private function assertTrustedDirectory(string $directory): void
    {
        if (! str_starts_with($directory, DIRECTORY_SEPARATOR)) {
            throw new RuntimeException('Managed Bridge configuration directory must be absolute.');
        }

        $segments = array_values(array_filter(
            explode(DIRECTORY_SEPARATOR, $directory),
            static fn (string $segment): bool => $segment !== ''
        ));

        $currentPath = DIRECTORY_SEPARATOR;

        foreach ($segments as $index => $segment) {
            $currentPath = rtrim($currentPath, DIRECTORY_SEPARATOR)
                . DIRECTORY_SEPARATOR
                . $segment;

            clearstatcache(true, $currentPath);

            $metadata = @lstat($currentPath);

            if (
                ! is_array($metadata)
                || is_link($currentPath)
                || (($metadata['mode'] & 0170000) !== 0040000)
                || (
                    ($metadata['uid'] !== $this->directoryUid && $metadata['uid'] !== 0)
                    || ($metadata['gid'] !== $this->directoryGid && $metadata['gid'] !== 0)
                )
            ) {
                throw new RuntimeException('Managed Bridge configuration directory ancestry is unsafe.');
            }

            $isFinalComponent = $index === count($segments) - 1;
            $permissions = $metadata['mode'] & 0777;
            $sticky = ($metadata['mode'] & 01000) === 01000;

            if ($isFinalComponent) {
                if (
                    $metadata['uid'] !== $this->directoryUid
                    || $metadata['gid'] !== $this->directoryGid
                    || $permissions !== 0711
                ) {
                    throw new RuntimeException('Managed Bridge configuration directory permissions are invalid.');
                }
            } elseif (($permissions & 0022) !== 0 && ! $sticky) {
                throw new RuntimeException('Managed Bridge configuration directory ancestry is writable.');
            }
        }
    }

    private function validateConfiguration(array $configuration): void
    {
        $expectedKeys = [
            'schema_version',
            'enabled',
            'application_id',
            'application_key',
            'allowed_ips',
            'credential_file',
            'activation_state_file',
        ];

        $actualKeys = array_keys($configuration);
        sort($expectedKeys);
        sort($actualKeys);

        if ($actualKeys !== $expectedKeys) {
            throw new RuntimeException('Managed Bridge configuration fields are invalid.');
        }

        $applicationId = $configuration['application_id'] ?? null;

        if (
            ($configuration['schema_version'] ?? null) !== 1
            || ($configuration['enabled'] ?? null) !== true
            || ! is_string($applicationId)
            || ! preg_match(
                '/^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/D',
                $applicationId
            )
            || ! is_string($configuration['application_key'] ?? null)
            || ! preg_match('/^[A-Za-z0-9._:-]{1,100}$/D', $configuration['application_key'])
            || ! is_array($configuration['allowed_ips'] ?? null)
            || ! array_is_list($configuration['allowed_ips'])
            || count($configuration['allowed_ips']) < 1
            || count($configuration['allowed_ips']) > 16
            || $configuration['credential_file'] !== '/var/lib/nadi-bridge/' . $applicationId . '/credential'
            || $configuration['activation_state_file'] !== '/run/nadi/bridge-state/' . $applicationId . '.json'
        ) {
            throw new RuntimeException('Managed Bridge configuration values are invalid.');
        }

        $seenAddresses = [];

        foreach ($configuration['allowed_ips'] as $address) {
            if (
                ! is_string($address)
                || filter_var($address, FILTER_VALIDATE_IP) === false
                || isset($seenAddresses[$address])
            ) {
                throw new RuntimeException('Managed Bridge source-IP allowlist is invalid.');
            }

            $seenAddresses[$address] = true;
        }
    }
}
