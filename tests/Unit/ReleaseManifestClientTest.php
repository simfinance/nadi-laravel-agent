<?php

namespace Nadi\LaravelAgent\Tests\Unit;

use Illuminate\Support\Facades\Http;
use Nadi\LaravelAgent\Installer\ReleaseManifestClient;
use Orchestra\Testbench\TestCase;
use RuntimeException;

class ReleaseManifestClientTest extends TestCase
{
    private const KEY_ID = 'nadi-agent-release-2026-01';

    private const ED25519_SPKI_PREFIX = "\x30\x2a\x30\x05\x06\x03\x2b\x65\x70\x03\x21\x00";

    public function test_it_fetches_and_verifies_the_central_manifest_with_integer_protocol_version(): void
    {
        [$manifestJson, $signature, $publicKeyPem, $fingerprint] = $this->signedManifest();

        config([
            'nadi-agent.release_manifest_url' => 'https://nadi.example.org/api/v1/nadi/releases/manifest',
            'nadi-agent.release_key_id' => self::KEY_ID,
            'nadi-agent.release_public_key' => $publicKeyPem,
            'nadi-agent.release_public_key_fingerprint' => $fingerprint,
        ]);

        Http::fake([
            'https://nadi.example.org/api/v1/nadi/releases/manifest' => Http::response(
                $manifestJson,
                200,
                [
                    'Content-Type' => 'application/json',
                    'X-NADI-Manifest-Signature' => $signature,
                ]
            ),
        ]);

        $manifest = app(ReleaseManifestClient::class)->fetch();

        self::assertSame('0.7.5-dev', $manifest['version']);
        self::assertSame(1, $manifest['protocol_version']);
    }

    public function test_it_maps_php_architecture_names_to_manifest_architectures(): void
    {
        $client = app(ReleaseManifestClient::class);
        $manifest = [
            'artifacts' => [
                [
                    'os' => 'linux',
                    'architecture' => 'amd64',
                    'filename' => 'nadi-agent-linux-amd64',
                ],
                [
                    'os' => 'linux',
                    'architecture' => 'arm64',
                    'filename' => 'nadi-agent-linux-arm64',
                ],
            ],
        ];

        self::assertSame(
            'nadi-agent-linux-amd64',
            $client->selectArtifact($manifest, 'x86_64')['filename']
        );
        self::assertSame(
            'nadi-agent-linux-arm64',
            $client->selectArtifact($manifest, 'aarch64')['filename']
        );
    }

    public function test_it_rejects_a_manifest_response_without_a_signature_header(): void
    {
        config([
            'nadi-agent.release_manifest_url' => 'https://nadi.example.org/api/v1/nadi/releases/manifest',
        ]);

        Http::fake([
            'https://nadi.example.org/api/v1/nadi/releases/manifest' => Http::response('{}', 200),
        ]);

        $this->expectException(RuntimeException::class);
        $this->expectExceptionMessage('NADI release manifest signature header is missing.');

        app(ReleaseManifestClient::class)->fetch();
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
            'version' => '0.7.5-dev',
            'protocol_version' => 1,
            'released_at' => '2026-11-22T06:34:20Z',
            'artifacts' => [
                [
                    'os' => 'linux',
                    'architecture' => 'amd64',
                    'filename' => 'nadi-agent-linux-amd64',
                    'url' => 'https://releases.example.org/nadi-agent-linux-amd64',
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
