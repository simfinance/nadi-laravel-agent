<?php

namespace Nadi\LaravelAgent\Installer;

use Illuminate\Http\Client\ConnectionException;
use Illuminate\Support\Facades\Http;
use Nadi\LaravelAgent\Security\ReleaseManifestVerifier;
use RuntimeException;

class ReleaseManifestClient
{
    private const MAX_MANIFEST_BYTES = 1048576;

    public function __construct(
        private ReleaseManifestVerifier $verifier
    ) {
    }

    public function fetch(): array
    {
        $manifestUrl = (string) config('nadi-agent.release_manifest_url');
        $parsedUrl = parse_url($manifestUrl);

        if (
            ! is_array($parsedUrl)
            || ($parsedUrl['scheme'] ?? null) !== 'https'
            || empty($parsedUrl['host'])
            || isset($parsedUrl['user'])
            || isset($parsedUrl['pass'])
        ) {
            throw new RuntimeException('NADI release manifest URL must be a valid HTTPS URL.');
        }

        try {
            $response = Http::acceptJson()
                ->connectTimeout(5)
                ->timeout(15)
                ->withOptions([
                    'allow_redirects' => false,
                ])
                ->get($manifestUrl);
        } catch (ConnectionException $exception) {
            throw new RuntimeException('Unable to connect to the NADI release manifest endpoint.', 0, $exception);
        }

        if (! $response->successful()) {
            throw new RuntimeException(
                'NADI release manifest request failed with HTTP ' . $response->status() . '.'
            );
        }

        $manifestJson = $response->body();

        if ($manifestJson === '' || strlen($manifestJson) > self::MAX_MANIFEST_BYTES) {
            throw new RuntimeException('NADI release manifest response is empty or too large.');
        }

        $signature = $response->header('X-NADI-Manifest-Signature');

        if (! is_string($signature) || trim($signature) === '') {
            throw new RuntimeException('NADI release manifest signature header is missing.');
        }

        return $this->verifier->verify(
            $manifestJson,
            trim($signature),
            (string) config('nadi-agent.release_public_key'),
            (string) config('nadi-agent.release_key_id'),
            (string) config('nadi-agent.release_public_key_fingerprint')
        );
    }

    public function selectArtifact(array $manifest, string $architecture): array
    {
        $normalizedArchitecture = match ($architecture) {
            'x86_64', 'amd64' => 'amd64',
            'aarch64', 'arm64' => 'arm64',
            default => throw new RuntimeException('Unsupported Agent architecture: ' . $architecture),
        };

        $matches = array_values(array_filter(
            $manifest['artifacts'] ?? [],
            static fn (array $artifact): bool =>
                ($artifact['os'] ?? null) === 'linux'
                && ($artifact['architecture'] ?? null) === $normalizedArchitecture
        ));

        if (count($matches) !== 1) {
            throw new RuntimeException(
                'Release manifest must contain exactly one Linux ' . $normalizedArchitecture . ' artifact.'
            );
        }

        return $matches[0];
    }
}
