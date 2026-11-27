<?php

namespace Nadi\LaravelAgent\Monitoring;

use DateTimeImmutable;
use Throwable;

class ActivationStateReader
{
    private const MAXIMUM_FILE_SIZE = 16384;

    private const MAXIMUM_ASSERTION_LIFETIME = 300;

    private const MAXIMUM_CLOCK_SKEW = 30;

    private const ALLOWED_GRANT_CAPABILITIES = [
        'server_metrics',
        'application_metrics',
        'database_monitoring',
        'queue_monitoring',
        'scheduler_monitoring',
        'system_logs',
        'application_logs',
    ];

    private const GRANT_TO_BRIDGE_CAPABILITIES = [
        'application_metrics' => ['health', 'metrics', 'info'],
        'database_monitoring' => ['database'],
        'queue_monitoring' => ['queue'],
        'scheduler_monitoring' => ['scheduler'],
        'application_logs' => ['application_logs'],
    ];

    public function allows(string $capability): bool
    {
        if (! in_array($capability, ['health', 'metrics', 'database', 'queue', 'scheduler', 'info', 'application_logs'], true)) {
            return false;
        }

        $path = config('nadi-agent.activation_state_file');
        $expectedApplicationId = config('nadi-agent.activation_application_id');
        $expectedApplicationKey = config('nadi-agent.activation_application_key');

        if (
            ! is_string($path)
            || $path === ''
            || ! is_string($expectedApplicationId)
            || $expectedApplicationId === ''
            || ! is_string($expectedApplicationKey)
            || $expectedApplicationKey === ''
        ) {
            return false;
        }

        try {
            $state = $this->readState($path);

            return $this->isValidState(
                $state,
                $expectedApplicationId,
                $expectedApplicationKey,
                $capability
            );
        } catch (Throwable) {
            return false;
        }
    }

    private function readState(string $path): array
    {
        clearstatcache(true, $path);

        $this->assertSafePath($path);

        if (is_link($path) || ! is_file($path) || ! is_readable($path)) {
            throw new \RuntimeException('Activation state file is unavailable.');
        }

        $pathStat = lstat($path);

        if (
            ! is_array($pathStat)
            || (($pathStat['mode'] & 0170000) !== 0100000)
            || $pathStat['uid'] !== 0
            || (($pathStat['mode'] & 0022) !== 0)
            || $pathStat['size'] < 2
            || $pathStat['size'] > self::MAXIMUM_FILE_SIZE
        ) {
            throw new \RuntimeException('Activation state file metadata is invalid.');
        }

        $handle = fopen($path, 'rb');

        if ($handle === false) {
            throw new \RuntimeException('Activation state file cannot be opened.');
        }

        try {
            $handleStat = fstat($handle);

            if (
                ! is_array($handleStat)
                || $handleStat['dev'] !== $pathStat['dev']
                || $handleStat['ino'] !== $pathStat['ino']
                || (($handleStat['mode'] & 0170000) !== 0100000)
                || $handleStat['uid'] !== 0
                || (($handleStat['mode'] & 0022) !== 0)
                || $handleStat['size'] > self::MAXIMUM_FILE_SIZE
            ) {
                throw new \RuntimeException('Activation state file changed during access.');
            }

            $contents = stream_get_contents($handle, self::MAXIMUM_FILE_SIZE + 1);

            if (
                ! is_string($contents)
                || $contents === ''
                || strlen($contents) > self::MAXIMUM_FILE_SIZE
            ) {
                throw new \RuntimeException('Activation state file size is invalid.');
            }
        } finally {
            fclose($handle);
        }

        $state = json_decode($contents, true, 16, JSON_THROW_ON_ERROR);

        if (! is_array($state) || array_is_list($state)) {
            throw new \RuntimeException('Activation state document is invalid.');
        }

        return $state;
    }

    private function assertSafePath(string $path): void
    {
        if (! str_starts_with($path, DIRECTORY_SEPARATOR)) {
            throw new \RuntimeException('Activation state path must be absolute.');
        }

        $rawSegments = explode(DIRECTORY_SEPARATOR, $path);

        if ($rawSegments[0] !== '' || count($rawSegments) < 2) {
            throw new \RuntimeException('Activation state path is not canonical.');
        }

        $segments = array_slice($rawSegments, 1);

        foreach ($segments as $segment) {
            if ($segment === '' || $segment === '.' || $segment === '..') {
                throw new \RuntimeException('Activation state path is not canonical.');
            }
        }

        $currentPath = DIRECTORY_SEPARATOR;

        foreach ($segments as $index => $segment) {
            $currentPath = rtrim($currentPath, DIRECTORY_SEPARATOR)
                . DIRECTORY_SEPARATOR
                . $segment;

            clearstatcache(true, $currentPath);

            $metadata = lstat($currentPath);

            if (! is_array($metadata) || is_link($currentPath)) {
                throw new \RuntimeException('Activation state path contains an unsafe component.');
            }

            $isFinalComponent = $index === count($segments) - 1;
            $expectedType = $isFinalComponent ? 0100000 : 0040000;

            if (
                (($metadata['mode'] & 0170000) !== $expectedType)
                || $metadata['uid'] !== 0
                || (($metadata['mode'] & 0022) !== 0)
            ) {
                throw new \RuntimeException('Activation state path ownership or permissions are invalid.');
            }
        }
    }

    private function isValidState(
        array $state,
        string $expectedApplicationId,
        string $expectedApplicationKey,
        string $capability
    ): bool {
        $expectedKeys = [
            'protocol_version',
            'application_id',
            'application_key',
            'activation_state',
            'monitoring_enabled',
            'grant_acknowledged',
            'capabilities',
            'observed_at',
            'valid_until',
        ];

        $actualKeys = array_keys($state);
        sort($expectedKeys);
        sort($actualKeys);

        if ($actualKeys !== $expectedKeys) {
            return false;
        }

        if (
            ($state['protocol_version'] ?? null) !== 1
            || ($state['application_id'] ?? null) !== $expectedApplicationId
            || ($state['application_key'] ?? null) !== $expectedApplicationKey
            || ($state['activation_state'] ?? null) !== 'active'
            || ($state['monitoring_enabled'] ?? null) !== true
            || ($state['grant_acknowledged'] ?? null) !== true
            || ! is_array($state['capabilities'] ?? null)
            || ! array_is_list($state['capabilities'])
            || ! $this->grantAllowsBridgeCapability($state['capabilities'], $capability)
        ) {
            return false;
        }

        foreach ($state['capabilities'] as $stateCapability) {
            if (
                ! is_string($stateCapability)
                || ! in_array($stateCapability, self::ALLOWED_GRANT_CAPABILITIES, true)
            ) {
                return false;
            }
        }

        if (count($state['capabilities']) !== count(array_unique($state['capabilities']))) {
            return false;
        }

        $observedAt = $this->parseTimestamp($state['observed_at'] ?? null);
        $validUntil = $this->parseTimestamp($state['valid_until'] ?? null);

        if ($observedAt === null || $validUntil === null) {
            return false;
        }

        $now = time();
        $observedTimestamp = $observedAt->getTimestamp();
        $validUntilTimestamp = $validUntil->getTimestamp();

        return $observedTimestamp <= ($now + self::MAXIMUM_CLOCK_SKEW)
            && $validUntilTimestamp > $now
            && $validUntilTimestamp > $observedTimestamp
            && ($validUntilTimestamp - $observedTimestamp) <= self::MAXIMUM_ASSERTION_LIFETIME;
    }


    private function grantAllowsBridgeCapability(array $grantCapabilities, string $bridgeCapability): bool
    {
        foreach ($grantCapabilities as $grantCapability) {
            if (! is_string($grantCapability)) {
                return false;
            }

            $bridgeCapabilities = self::GRANT_TO_BRIDGE_CAPABILITIES[$grantCapability] ?? [];

            if (in_array($bridgeCapability, $bridgeCapabilities, true)) {
                return true;
            }
        }

        return false;
    }

    private function parseTimestamp(mixed $value): ?DateTimeImmutable
    {
        if (
            ! is_string($value)
            || ! preg_match(
                '/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/',
                $value
            )
        ) {
            return null;
        }

        try {
            return new DateTimeImmutable($value);
        } catch (Throwable) {
            return null;
        }
    }
}
