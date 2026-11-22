<?php

namespace Nadi\LaravelAgent\Security;

use JsonException;
use RuntimeException;

class ReleaseManifestVerifier
{
    private const ED25519_SPKI_PREFIX = "\x30\x2a\x30\x05\x06\x03\x2b\x65\x70\x03\x21\x00";

    public function verify(
        string $manifestJson,
        string $signatureBase64,
        string $publicKeyPem,
        string $expectedKeyId,
        string $expectedFingerprint
    ): array {
        if (! function_exists('sodium_crypto_sign_verify_detached')) {
            throw new RuntimeException('PHP sodium extension is required.');
        }

        if ($expectedKeyId === '') {
            throw new RuntimeException('Expected release signing key ID is required.');
        }

        $publicKey = $this->decodePublicKeyPem(
            $publicKeyPem,
            $expectedFingerprint
        );

        $signature = base64_decode($signatureBase64, true);

        if ($signature === false || strlen($signature) !== SODIUM_CRYPTO_SIGN_BYTES) {
            throw new RuntimeException('Release manifest signature is invalid.');
        }

        if (! sodium_crypto_sign_verify_detached($signature, $manifestJson, $publicKey)) {
            throw new RuntimeException('Release manifest signature verification failed.');
        }

        try {
            $manifest = json_decode($manifestJson, true, 512, JSON_THROW_ON_ERROR);
        } catch (JsonException $exception) {
            throw new RuntimeException('Release manifest JSON is invalid.', 0, $exception);
        }

        if (! is_array($manifest) || array_is_list($manifest)) {
            throw new RuntimeException('Release manifest must be a JSON object.');
        }

        $this->validateManifest($manifest, $expectedKeyId);

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

        if (! is_string($actualSha256) || ! hash_equals(
            strtolower($expectedSha256),
            strtolower($actualSha256)
        )) {
            throw new RuntimeException('Agent artifact SHA-256 verification failed.');
        }
    }

    private function decodePublicKeyPem(
        string $publicKeyPem,
        string $expectedFingerprint
    ): string {
        if (! preg_match(
            '/\A-----BEGIN PUBLIC KEY-----\s*([A-Za-z0-9+\/=\r\n]+)\s*-----END PUBLIC KEY-----\z/',
            trim($publicKeyPem),
            $matches
        )) {
            throw new RuntimeException('Release public key PEM is invalid.');
        }

        $der = base64_decode(
            preg_replace('/\s+/', '', $matches[1]),
            true
        );

        if ($der === false || ! str_starts_with($der, self::ED25519_SPKI_PREFIX)) {
            throw new RuntimeException('Release public key is not a valid Ed25519 SubjectPublicKeyInfo key.');
        }

        $publicKey = substr($der, strlen(self::ED25519_SPKI_PREFIX));

        if (strlen($publicKey) !== SODIUM_CRYPTO_SIGN_PUBLICKEYBYTES) {
            throw new RuntimeException('Release public key has an invalid Ed25519 key length.');
        }

        if (! preg_match('/^[a-f0-9]{64}$/i', $expectedFingerprint)) {
            throw new RuntimeException('Expected release public key fingerprint is invalid.');
        }

        $actualFingerprint = hash('sha256', $der);

        if (! hash_equals(strtolower($expectedFingerprint), $actualFingerprint)) {
            throw new RuntimeException('Release public key fingerprint verification failed.');
        }

        return $publicKey;
    }

    private function validateManifest(array $manifest, string $expectedKeyId): void
    {
        $required = [
            'schema_version',
            'product',
            'key_id',
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

        if (
            $manifest['schema_version'] !== 1
            || $manifest['product'] !== 'nadi-agent'
        ) {
            throw new RuntimeException('Unsupported release manifest schema or product.');
        }

        if (! is_string($manifest['key_id']) || ! hash_equals(
            $expectedKeyId,
            $manifest['key_id']
        )) {
            throw new RuntimeException('Release manifest key ID does not match the trusted key.');
        }

        foreach (['version', 'protocol_version', 'released_at'] as $field) {
            if (! is_string($manifest[$field]) || trim($manifest[$field]) === '') {
                throw new RuntimeException("Release manifest field {$field} is invalid.");
            }
        }

        if (! is_array($manifest['artifacts']) || $manifest['artifacts'] === []) {
            throw new RuntimeException('Release manifest contains no artifacts.');
        }

        foreach ($manifest['artifacts'] as $artifact) {
            if (! is_array($artifact)) {
                throw new RuntimeException('Release artifact entry is invalid.');
            }

            foreach (['os', 'architecture', 'filename', 'url', 'sha256'] as $field) {
                if (
                    ! isset($artifact[$field])
                    || ! is_string($artifact[$field])
                    || trim($artifact[$field]) === ''
                ) {
                    throw new RuntimeException("Release artifact is missing {$field}.");
                }
            }

            if (
                ! preg_match('/^[A-Za-z0-9._-]+$/D', $artifact['filename'])
                || $artifact['filename'] === '.'
                || $artifact['filename'] === '..'
            ) {
                throw new RuntimeException('Release artifact filename is invalid.');
            }

            $artifactUrl = parse_url($artifact['url']);

            if (
                ! is_array($artifactUrl)
                || ($artifactUrl['scheme'] ?? null) !== 'https'
                || empty($artifactUrl['host'])
                || isset($artifactUrl['user'])
                || isset($artifactUrl['pass'])
            ) {
                throw new RuntimeException('Release artifact URL must be a valid HTTPS URL.');
            }

            if (! preg_match('/^[a-f0-9]{64}$/i', $artifact['sha256'])) {
                throw new RuntimeException('Release artifact SHA-256 digest is malformed.');
            }
        }
    }
}
