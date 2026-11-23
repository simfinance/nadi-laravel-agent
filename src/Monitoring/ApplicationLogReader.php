<?php

namespace Nadi\LaravelAgent\Monitoring;

use RuntimeException;

class ApplicationLogReader
{
    private const ERROR_LEVELS = [
        'ERROR',
        'CRITICAL',
        'ALERT',
        'EMERGENCY',
    ];

    public function collect(): array
    {
        $maximumEntries = max(
            1,
            min(100, (int) config('nadi-agent.logs.application.max_entries', 50))
        );

        $maximumBytes = max(
            65536,
            min(1048576, (int) config('nadi-agent.logs.application.max_bytes', 1048576))
        );

        $path = $this->resolveLogPath();

        if ($path === null) {
            return [
                'status' => 'unavailable',
                'error_code' => 'application_log_file_unavailable',
                'entries' => [],
                'count' => 0,
                'truncated' => false,
                'checked_at' => now()->toISOString(),
            ];
        }

        try {
            [$contents, $truncated] = $this->readTail($path, $maximumBytes);
            $entries = $this->summarize($contents, $maximumEntries);

            return [
                'status' => 'ok',
                'source' => 'laravel_application',
                'entries' => $entries,
                'count' => count($entries),
                'truncated' => $truncated,
                'checked_at' => now()->toISOString(),
            ];
        } catch (RuntimeException) {
            return [
                'status' => 'unavailable',
                'error_code' => 'application_log_read_failed',
                'entries' => [],
                'count' => 0,
                'truncated' => false,
                'checked_at' => now()->toISOString(),
            ];
        }
    }

    private function resolveLogPath(): ?string
    {
        $defaultChannel = config('logging.default');

        if (! is_string($defaultChannel) || $defaultChannel === '') {
            return null;
        }

        $channels = $this->resolveChannels($defaultChannel, []);

        foreach ($channels as $channel) {
            $driver = $channel['driver'] ?? null;
            $configuredPath = $channel['path'] ?? null;

            if (
                ! in_array($driver, ['single', 'daily'], true)
                || ! is_string($configuredPath)
                || $configuredPath === ''
            ) {
                continue;
            }

            $candidates = $driver === 'daily'
                ? $this->dailyLogCandidates($configuredPath)
                : [$configuredPath];

            foreach ($candidates as $candidate) {
                $safePath = $this->validateLogPath($candidate);

                if ($safePath !== null) {
                    return $safePath;
                }
            }
        }

        return null;
    }

    private function resolveChannels(string $name, array $visited): array
    {
        if (isset($visited[$name])) {
            return [];
        }

        $visited[$name] = true;
        $channel = config('logging.channels.' . $name);

        if (! is_array($channel)) {
            return [];
        }

        if (($channel['driver'] ?? null) !== 'stack') {
            return [$channel];
        }

        $resolved = [];
        $children = $channel['channels'] ?? [];

        if (! is_array($children)) {
            return [];
        }

        foreach ($children as $child) {
            if (is_string($child) && $child !== '') {
                array_push(
                    $resolved,
                    ...$this->resolveChannels($child, $visited)
                );
            }
        }

        return $resolved;
    }

    private function dailyLogCandidates(string $basePath): array
    {
        $directory = dirname($basePath);
        $filename = pathinfo($basePath, PATHINFO_FILENAME);
        $pattern = $directory . DIRECTORY_SEPARATOR . $filename . '-*.log';
        $files = glob($pattern);

        if (! is_array($files)) {
            return [];
        }

        usort(
            $files,
            static fn (string $left, string $right): int =>
                (filemtime($right) ?: 0) <=> (filemtime($left) ?: 0)
        );

        return $files;
    }

    private function validateLogPath(string $path): ?string
    {
        if (is_link($path) || ! is_file($path) || ! is_readable($path)) {
            return null;
        }

        $logsRoot = realpath(storage_path('logs'));
        $resolvedPath = realpath($path);

        if ($logsRoot === false || $resolvedPath === false) {
            return null;
        }

        $expectedPrefix = rtrim($logsRoot, DIRECTORY_SEPARATOR)
            . DIRECTORY_SEPARATOR;

        if (! str_starts_with($resolvedPath, $expectedPrefix)) {
            return null;
        }

        $metadata = lstat($resolvedPath);

        if (
            ! is_array($metadata)
            || (($metadata['mode'] & 0170000) !== 0100000)
        ) {
            return null;
        }

        return $resolvedPath;
    }

    private function readTail(string $path, int $maximumBytes): array
    {
        $handle = fopen($path, 'rb');

        if ($handle === false) {
            throw new RuntimeException('Unable to open application log.');
        }

        try {
            $metadata = fstat($handle);

            if (
                ! is_array($metadata)
                || (($metadata['mode'] & 0170000) !== 0100000)
            ) {
                throw new RuntimeException('Application log is not a regular file.');
            }

            $fileSize = (int) $metadata['size'];
            $truncated = $fileSize > $maximumBytes;

            if ($truncated && fseek($handle, -$maximumBytes, SEEK_END) !== 0) {
                throw new RuntimeException('Unable to seek application log.');
            }

            if ($truncated) {
                fgets($handle);
            }

            $contents = stream_get_contents($handle, $maximumBytes + 1);

            if (! is_string($contents) || strlen($contents) > $maximumBytes) {
                throw new RuntimeException('Application log tail exceeds the size limit.');
            }

            return [$contents, $truncated];
        } finally {
            fclose($handle);
        }
    }

    private function summarize(string $contents, int $maximumEntries): array
    {
        $lines = preg_split('/\r?\n/', $contents);
        $records = [];
        $currentRecord = null;

        foreach ($lines as $line) {
            if (preg_match(
                '/^\[(?<timestamp>[^\]]+)\]\s+[^.]+\.(?<level>DEBUG|INFO|NOTICE|WARNING|ERROR|CRITICAL|ALERT|EMERGENCY):/',
                $line,
                $matches
            )) {
                if ($currentRecord !== null) {
                    $records[] = $currentRecord;
                }

                $currentRecord = [
                    'timestamp' => $matches['timestamp'],
                    'level' => strtoupper($matches['level']),
                    'content' => $line,
                ];

                continue;
            }

            if ($currentRecord !== null) {
                $currentRecord['content'] .= "\n" . $line;
            }
        }

        if ($currentRecord !== null) {
            $records[] = $currentRecord;
        }

        $entries = [];

        foreach (array_reverse($records) as $record) {
            if (! in_array($record['level'], self::ERROR_LEVELS, true)) {
                continue;
            }

            $exceptionType = $this->exceptionType($record['content']);

            $entries[] = [
                'timestamp' => $record['timestamp'],
                'level' => $record['level'],
                'exception_type' => $exceptionType,
                'fingerprint' => hash(
                    'sha256',
                    $record['level'] . '|' . ($exceptionType ?? 'unclassified')
                ),
            ];

            if (count($entries) >= $maximumEntries) {
                break;
            }
        }

        return $entries;
    }

    private function exceptionType(string $record): ?string
    {
        if (! preg_match(
            '/^([A-Za-z_\\\\][A-Za-z0-9_\\\\]*(?:Exception|Error|Throwable))(?::|\s|$)/m',
            $record,
            $matches
        )) {
            return null;
        }

        return $matches[1];
    }
}
