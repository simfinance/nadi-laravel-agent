<?php

namespace Nadi\LaravelAgent\Tests\Unit;

use Nadi\LaravelAgent\Security\ReleaseManifestVerifier;
use Orchestra\Testbench\TestCase;
use RuntimeException;

class ReleaseManifestVerifierTest extends TestCase
{
    private const KEY_ID = 'nadi-agent-release-2026-01';

    private const ED25519_SPKI_PREFIX = "\x30\x2a\x30\x05\x06\x03\x2b\x65\x70\x03\x21\x00";

    public function test_it_verifies_a_signed_release_manifest_with_a_pem_public_key(): void
    {
        [$manifestJson, $signature, $publicKeyPem, $fingerprint] = $this->signedManifest();

        $manifest = (new ReleaseManifestVerifier())->verify(
            $manifestJson,
            $signature,
            $publicKeyPem,
            self::KEY_ID,
            $fingerprint
        );

        self::assertSame('nadi-agent', $manifest['product']);
        self::assertSame(self::KEY_ID, $manifest['key_id']);
        self::assertSame('0.7.5', $manifest['version']);
    }

    public function test_it_rejects_a_manifest_that_was_modified_after_signing(): void
    {
        [$manifestJson, $signature, $publicKeyPem, $fingerprint] = $this->signedManifest();

        $manifestJson = str_replace('0.7.5', '0.7.6', $manifestJson);

        $this->expectException(RuntimeException::class);
        $this->expectExceptionMessage('Release manifest signature verification failed.');

        (new ReleaseManifestVerifier())->verify(
            $manifestJson,
            $signature,
            $publicKeyPem,
            self::KEY_ID,
            $fingerprint
        );
    }

    public function test_it_rejects_a_manifest_signed_with_an_unexpected_key_id(): void
    {
        [$manifestJson, $signature, $publicKeyPem, $fingerprint] = $this->signedManifest();

        $this->expectException(RuntimeException::class);
        $this->expectExceptionMessage('Release manifest key ID does not match the trusted key.');

        (new ReleaseManifestVerifier())->verify(
            $manifestJson,
            $signature,
            $publicKeyPem,
            'nadi-agent-release-unexpected',
            $fingerprint
        );
    }

    public function test_it_rejects_a_public_key_with_an_unexpected_fingerprint(): void
    {
        [$manifestJson, $signature, $publicKeyPem] = $this->signedManifest();

        $this->expectException(RuntimeException::class);
        $this->expectExceptionMessage('Release public key fingerprint verification failed.');

        (new ReleaseManifestVerifier())->verify(
            $manifestJson,
            $signature,
            $publicKeyPem,
            self::KEY_ID,
            str_repeat('0', 64)
        );
    }

    private function signedManifest(): array
    {
        $keyPair = sodium_crypto_sign_keypair();
        $publicKey = sodium_crypto_sign_publickey($keyPair);
        $secretKey = sodium_crypto_sign_secretkey($keyPair);
        $der = self::ED25519_SPKI_PREFIX . $publicKey;
        $publicKeyPem = "-----BEGIN PUBLIC KEY-----\n"
            . chunk_split(base64_encode($der), 64, "\n")
            . "-----END PUBLIC KEY-----\n";

        $manifest = [
            'schema_version' => 1,
            'product' => 'nadi-agent',
            'key_id' => self::KEY_ID,
            'version' => '0.7.5',
            'protocol_version' => '1',
            'released_at' => '2026-11-22T00:00:00Z',
            'artifacts' => [
                [
                    'os' => 'linux',
                    'architecture' => 'amd64',
                    'filename' => 'nadi-agent',
                    'url' => 'https://releases.example.org/nadi-agent',
                    'sha256' => str_repeat('a', 64),
                ],
            ],
        ];

        $manifestJson = json_encode(
            $manifest,
            JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR
        );

        $signature = sodium_crypto_sign_detached($manifestJson, $secretKey);

        return [
            $manifestJson,
            base64_encode($signature),
            $publicKeyPem,
            hash('sha256', $der),
        ];
    }
}
