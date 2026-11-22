<?php

namespace Nadi\LaravelAgent\Installer;

use Illuminate\Http\Client\ConnectionException;
use Illuminate\Support\Facades\Http;
use Nadi\LaravelAgent\Security\ReleaseManifestVerifier;
use RuntimeException;

class ReleaseArtifactDownloader
{
    private const MAX_ARTIFACT_BYTES = 104857600;

    public function __construct(
        private ReleaseManifestVerifier $verifier
    ) {
    }

    public function download(array $artifact, string $stagingDirectory): string
    {
        $filename = $artifact['filename'] ?? null;
        $url = $artifact['url'] ?? null;
        $sha256 = $artifact['sha256'] ?? null;

        if (
            ! is_string($filename)
            || ! preg_match('/^[A-Za-z0-9._-]+$/D', $filename)
            || $filename === '.'
            || $filename === '..'
        ) {
            throw new RuntimeException('Agent artifact filename is invalid.');
        }

        if (! is_string($url)) {
            throw new RuntimeException('Agent artifact URL is invalid.');
        }

        $parsedUrl = parse_url($url);

        if (
            ! is_array($parsedUrl)
            || ($parsedUrl['scheme'] ?? null) !== 'https'
            || empty($parsedUrl['host'])
            || isset($parsedUrl['user'])
            || isset($parsedUrl['pass'])
        ) {
            throw new RuntimeException('Agent artifact URL must be a valid HTTPS URL.');
        }

        if (! is_string($sha256) || ! preg_match('/^[a-f0-9]{64}$/D', $sha256)) {
            throw new RuntimeException('Agent artifact SHA-256 digest is malformed.');
        }

        if (! is_dir($stagingDirectory) || ! is_writable($stagingDirectory)) {
            throw new RuntimeException('Agent release staging directory is missing or not writable.');
        }

        if (is_link($stagingDirectory)) {
            throw new RuntimeException('Agent release staging directory must not be a symbolic link.');
        }

        $directory = realpath($stagingDirectory);

        if ($directory === false) {
            throw new RuntimeException('Agent release staging directory cannot be resolved.');
        }

        $targetPath = $directory . DIRECTORY_SEPARATOR . $filename;

        if (is_link($targetPath)) {
            throw new RuntimeException('Agent artifact staging target must not be a symbolic link.');
        }

        if (is_file($targetPath)) {
            $this->verifier->verifyArtifact($targetPath, $sha256);

            return $targetPath;
        }

        if (file_exists($targetPath)) {
            throw new RuntimeException('Agent artifact staging target already exists and is not a regular file.');
        }

        try {
            $response = Http::connectTimeout(10)
                ->timeout(120)
                ->withOptions([
                    'allow_redirects' => [
                        'max' => 5,
                        'protocols' => ['https'],
                    ],
                ])
                ->get($url);
        } catch (ConnectionException $exception) {
            throw new RuntimeException('Unable to connect to the Agent artifact download URL.', 0, $exception);
        }

        if (! $response->successful()) {
            throw new RuntimeException(
                'Agent artifact download failed with HTTP ' . $response->status() . '.'
            );
        }

        $content = $response->body();
        $contentLength = strlen($content);

        if ($contentLength === 0 || $contentLength > self::MAX_ARTIFACT_BYTES) {
            throw new RuntimeException('Agent artifact download is empty or exceeds the size limit.');
        }

        $temporaryPath = tempnam($directory, '.nadi-agent-');

        if ($temporaryPath === false) {
            throw new RuntimeException('Unable to create temporary Agent artifact file.');
        }

        try {
            if (file_put_contents($temporaryPath, $content, LOCK_EX) !== $contentLength) {
                throw new RuntimeException('Unable to write the downloaded Agent artifact.');
            }

            chmod($temporaryPath, 0600);
            $this->verifier->verifyArtifact($temporaryPath, $sha256);

            if (! rename($temporaryPath, $targetPath)) {
                throw new RuntimeException('Unable to move the verified Agent artifact into staging.');
            }

            chmod($targetPath, 0600);

            return $targetPath;
        } finally {
            if (is_file($temporaryPath)) {
                unlink($temporaryPath);
            }
        }
    }
}
