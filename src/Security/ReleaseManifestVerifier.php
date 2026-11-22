<?php

namespace Nadi\LaravelAgent\Security;

use RuntimeException;

class ReleaseManifestVerifier
{
    public function verify(
        string $manifestJson,
        string $signatureBase64,
        string $publicKeyBase64
    ): array {
        if (! function_exists('sodium_crypto_sign_verify_detached')) {
            throw new RuntimeException('PHP sodium extension is required.');
        }

        $publicKey = base64_decode($publicKeyBase64, true);
        $signature = base64_decode($signatureBase64, true);

        if ($publicKey === false || strlen($publicKey) !== SODIUM_CRYPTO_SIGN_PUBLICKEYBYTES) {
            throw new RuntimeException('Release public key is invalid.');
        }

        if ($signature === false || strlen($signature) !== SODIUM_CRYPTO_SIGN_BYTES) {
            throw new RuntimeException('Release manifest signature is invalid.');
        }

        if (! sodium_crypto_sign_verify_detached($signature, $manifestJson, $publicKey)) {
            throw new RuntimeException('Release manifest signature verification failed.');
        }

        try {
            $manifest = json_decode($manifestJson, true, 512, JSON_THROW_ON_ERROR);
        } catch (\JsonException $exception) {
            throw new RuntimeException('Release manifest JSON is invalid.', 0, $exception);
        }

        $this->validateManifest($manifest);

        return $manifest;
    }

    public function verifyArtifact(string $filePath, string $expectedSha256): void
    {
        if (! is_file($filePath) || ! is_readable($filePath)) {
            throw new RuntimeException('Downloaded Agent artifact is missing or unreadable.');
        }

        if (! preg_match('/^[a-f0-9]{64}$/i', $expectedSha256)) {
            throw new RuntimeException('Expected SHA-256 digest is malformed.');
        }

        $actualSha256 = hash_file('sha256', $filePath);

        if (! hash_equals(strtolower($expectedSha256), strtolower($actualSha256))) {
            throw new RuntimeException('Agent artifact SHA-256 verification failed.');
        }
    }

    private function validateManifest(array $manifest): void
    {
        $required = [
            'schema_version',
            'product',
            'version',
            'protocol_version',
            'released_at',
            'artifacts',
        ];

        foreach ($required as $field) {
            if (! array_key_exists($field, $manifest)) {
                throw new RuntimeException("Release manifest is missing {$field}.");
            }
        }

        if ($manifest['schema_version'] !== 1 || $manifest['product'] !== 'nadi-agent') {
            throw new RuntimeException('Unsupported release manifest schema or product.');
        }

        if (! is_array($manifest['artifacts']) || $manifest['artifacts'] === []) {
            throw new RuntimeException('Release manifest contains no artifacts.');
        }

        foreach ($manifest['artifacts'] as $artifact) {
            foreach (['os', 'architecture', 'filename', 'url', 'sha256'] as $field) {
                if (! isset($artifact[$field]) || ! is_string($artifact[$field]) || $artifact[$field] === '') {
                    throw new RuntimeException("Release artifact is missing {$field}.");
                }
            }

            if (! str_starts_with($artifact['url'], 'https://')) {
                throw new RuntimeException('Release artifact URL must use HTTPS.');
            }

            if (! preg_match('/^[a-f0-9]{64}$/i', $artifact['sha256'])) {
                throw new RuntimeException('Release artifact SHA-256 digest is malformed.');
            }
        }
    }
}