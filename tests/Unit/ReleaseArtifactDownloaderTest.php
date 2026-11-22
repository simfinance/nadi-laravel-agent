<?php

namespace Nadi\LaravelAgent\Tests\Unit;

use Illuminate\Support\Facades\Http;
use Nadi\LaravelAgent\Installer\ReleaseArtifactDownloader;
use Nadi\LaravelAgent\Security\ReleaseManifestVerifier;
use Orchestra\Testbench\TestCase;
use RuntimeException;

class ReleaseArtifactDownloaderTest extends TestCase
{
    public function test_it_downloads_and_verifies_an_artifact_before_staging_it(): void
    {
        $content = "NADI Agent test binary\0";
        $directory = $this->createStagingDirectory();

        Http::fake([
            'https://releases.example.org/*' => Http::response($content, 200),
        ]);

        $downloader = new ReleaseArtifactDownloader(new ReleaseManifestVerifier());
        $path = $downloader->download([
            'filename' => 'nadi-agent-linux-amd64',
            'url' => 'https://releases.example.org/nadi-agent-linux-amd64',
            'sha256' => hash('sha256', $content),
        ], $directory);

        self::assertSame($content, file_get_contents($path));
        self::assertSame(0600, fileperms($path) & 0777);

        unlink($path);
        rmdir($directory);
    }

    public function test_it_does_not_stage_an_artifact_with_an_invalid_checksum(): void
    {
        $directory = $this->createStagingDirectory();

        Http::fake([
            'https://releases.example.org/*' => Http::response('untrusted binary', 200),
        ]);

        $downloader = new ReleaseArtifactDownloader(new ReleaseManifestVerifier());

        try {
            $downloader->download([
                'filename' => 'nadi-agent-linux-amd64',
                'url' => 'https://releases.example.org/nadi-agent-linux-amd64',
                'sha256' => str_repeat('0', 64),
            ], $directory);

            self::fail('Expected checksum verification to fail.');
        } catch (RuntimeException $exception) {
            self::assertSame('Agent artifact SHA-256 verification failed.', $exception->getMessage());
        }

        self::assertSame([], array_values(array_diff(scandir($directory), ['.', '..'])));
        rmdir($directory);
    }

    private function createStagingDirectory(): string
    {
        $directory = sys_get_temp_dir() . '/nadi-release-test-' . bin2hex(random_bytes(8));

        mkdir($directory, 0700, true);

        return $directory;
    }
}
