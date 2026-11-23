<?php

namespace Nadi\LaravelAgent\Tests\Unit;

use Nadi\LaravelAgent\Monitoring\ApplicationLogReader;
use Orchestra\Testbench\TestCase;

class ApplicationLogReaderTest extends TestCase
{
    private string $logPath;

    protected function setUp(): void
    {
        parent::setUp();

        $directory = storage_path('logs');

        if (! is_dir($directory)) {
            mkdir($directory, 0700, true);
        }

        $this->logPath = $directory
            . DIRECTORY_SEPARATOR
            . 'nadi-application-log-test.log';

        config([
            'logging.default' => 'nadi_test',
            'logging.channels.nadi_test' => [
                'driver' => 'single',
                'path' => $this->logPath,
            ],
            'nadi-agent.logs.application.max_entries' => 10,
            'nadi-agent.logs.application.max_bytes' => 65536,
        ]);
    }

    protected function tearDown(): void
    {
        if (isset($this->logPath) && is_file($this->logPath)) {
            unlink($this->logPath);
        }

        parent::tearDown();
    }

    public function test_it_returns_only_error_metadata_without_raw_log_messages(): void
    {
        $timestamp = now()->format('Y-m-d H:i:s');

        file_put_contents(
            $this->logPath,
            '[' . $timestamp . '] testing.INFO: A normal message' . PHP_EOL
            . '[' . $timestamp . '] testing.ERROR: User password=secret-value failed' . PHP_EOL
            . 'RuntimeException: private detail must not be returned' . PHP_EOL
            . '#0 /var/www/app/vendor/example.php(10): run()' . PHP_EOL
        );

        $result = (new ApplicationLogReader())->collect();

        self::assertSame('ok', $result['status']);
        self::assertCount(1, $result['entries']);
        self::assertSame('ERROR', $result['entries'][0]['level']);
        self::assertSame('RuntimeException', $result['entries'][0]['exception_type']);
        self::assertArrayNotHasKey('message', $result['entries'][0]);
        self::assertStringNotContainsString(
            'secret-value',
            json_encode($result, JSON_THROW_ON_ERROR)
        );
        self::assertStringNotContainsString(
            'private detail',
            json_encode($result, JSON_THROW_ON_ERROR)
        );
    }

    public function test_it_fails_closed_when_the_log_file_is_outside_storage_logs(): void
    {
        $outsidePath = sys_get_temp_dir()
            . DIRECTORY_SEPARATOR
            . 'nadi-outside-log-'
            . bin2hex(random_bytes(8))
            . '.log';

        file_put_contents($outsidePath, '[2026-11-23 00:00:00] testing.ERROR: hidden');

        config([
            'logging.channels.nadi_test.path' => $outsidePath,
        ]);

        try {
            $result = (new ApplicationLogReader())->collect();

            self::assertSame('unavailable', $result['status']);
            self::assertSame([], $result['entries']);
        } finally {
            unlink($outsidePath);
        }
    }
}
