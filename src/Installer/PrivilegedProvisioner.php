<?php

namespace Nadi\LaravelAgent\Installer;

use JsonException;
use RuntimeException;
use Symfony\Component\Process\Process;
use Throwable;

class PrivilegedProvisioner
{
    private const SUDO_PATH = '/usr/bin/sudo';

    private const HELPER_PATH = '/usr/libexec/nadi/nadi-provision';

    private const MAXIMUM_REQUEST_BYTES = 65536;

    private const MAXIMUM_RESPONSE_BYTES = 4096;

    public function assertAvailable(): void
    {
        $this->assertTrustedExecutable(self::SUDO_PATH, 'sudo');
        $this->assertTrustedExecutable(self::HELPER_PATH, 'NADI provisioning helper');
    }

    public function installAgent(
        array $signedRelease,
        string $artifactPath,
        array $applicationManifest,
        string $applicationBasePath,
        string $enrollmentToken
    ): array {
        $this->assertAvailable();

        $manifestJson = $signedRelease['manifest_json'] ?? null;
        $signature = $signedRelease['signature'] ?? null;

        if (
            ! is_string($manifestJson)
            || $manifestJson === ''
            || ! is_string($signature)
            || $signature === ''
        ) {
            throw new RuntimeException('Verified signed release material is incomplete.');
        }

        if (! preg_match('/^nadi_enroll_[a-f0-9]{64}$/D', $enrollmentToken)) {
            throw new RuntimeException('Enrollment token format is invalid.');
        }

        $resolvedApplicationPath = realpath($applicationBasePath);
        $resolvedArtifactPath = realpath($artifactPath);
        $stagingRoot = realpath(storage_path('app/private/nadi/releases/staging'));

        if (
            $resolvedApplicationPath === false
            || $resolvedArtifactPath === false
            || $stagingRoot === false
            || ! is_file($resolvedArtifactPath)
            || is_link($artifactPath)
            || ! str_starts_with(
                $resolvedArtifactPath,
                $stagingRoot . DIRECTORY_SEPARATOR
            )
        ) {
            throw new RuntimeException('Agent artifact or Laravel application path is unsafe.');
        }

        $request = [
            'protocol_version' => 1,
            'operation' => 'install_agent',
            'release' => [
                'manifest_base64' => base64_encode($manifestJson),
                'signature_base64' => $signature,
                'artifact_path' => $resolvedArtifactPath,
            ],
            'application' => [
                'base_path' => $resolvedApplicationPath,
                'manifest_base64' => base64_encode(
                    json_encode(
                        $applicationManifest,
                        JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR
                    )
                ),
            ],
            'enrollment_token' => $enrollmentToken,
        ];

        try {
            $requestBody = json_encode(
                $request,
                JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR
            );
        } catch (JsonException $exception) {
            throw new RuntimeException('Unable to encode the provisioning request.', 0, $exception);
        }

        if (strlen($requestBody) > self::MAXIMUM_REQUEST_BYTES) {
            throw new RuntimeException('Provisioning request exceeds the allowed size.');
        }

        $process = new Process([
            self::SUDO_PATH,
            '-n',
            '--',
            self::HELPER_PATH,
            '--request-stdin',
        ]);

        $process->setInput($requestBody . "\n");
        $process->setTimeout(180);
        $process->setEnv([
            'NADI_ENROLLMENT_TOKEN' => false,
            'NADI_AGENT_TOKEN' => false,
        ]);

        try {
            try {
                $process->run();
            } catch (Throwable $exception) {
                throw new RuntimeException(
                    'NADI privileged provisioning could not be completed.',
                    0,
                    $exception
                );
            }

            if (! $process->isSuccessful()) {
                throw new RuntimeException(
                    'NADI privileged provisioning failed. No successful installation or enrollment was confirmed.'
                );
            }

            return $this->decodeResponse($process->getOutput());
        } finally {
            if (function_exists('sodium_memzero')) {
                sodium_memzero($requestBody);
                sodium_memzero($request['enrollment_token']);
            }

            unset($process, $requestBody, $request);
        }
    }

    public function decodeResponse(string $responseBody): array
    {
        if (
            $responseBody === ''
            || strlen($responseBody) > self::MAXIMUM_RESPONSE_BYTES
        ) {
            throw new RuntimeException('NADI provisioning helper response is empty or too large.');
        }

        try {
            $response = json_decode(
                $responseBody,
                true,
                16,
                JSON_THROW_ON_ERROR
            );
        } catch (JsonException $exception) {
            throw new RuntimeException('NADI provisioning helper returned invalid JSON.', 0, $exception);
        }

        if (! is_array($response) || array_is_list($response)) {
            throw new RuntimeException('NADI provisioning helper response must be a JSON object.');
        }

        $expectedKeys = [
            'protocol_version',
            'success',
            'agent_version',
            'enrollment_status',
            'application_count',
        ];

        $actualKeys = array_keys($response);
        sort($expectedKeys);
        sort($actualKeys);

        if ($actualKeys !== $expectedKeys) {
            throw new RuntimeException('NADI provisioning helper response fields are invalid.');
        }

        if (
            $response['protocol_version'] !== 1
            || $response['success'] !== true
            || ! is_string($response['agent_version'])
            || $response['agent_version'] === ''
            || $response['enrollment_status'] !== 'registered'
            || ! is_int($response['application_count'])
            || $response['application_count'] < 1
            || $response['application_count'] > 20
        ) {
            throw new RuntimeException('NADI provisioning helper did not confirm a completed enrollment.');
        }

        return $response;
    }

    private function assertTrustedExecutable(string $path, string $label): void
    {
        clearstatcache(true, $path);

        $segments = array_values(array_filter(
            explode(DIRECTORY_SEPARATOR, dirname($path)),
            static fn (string $segment): bool => $segment !== ''
        ));
        $currentPath = DIRECTORY_SEPARATOR;

        foreach ($segments as $segment) {
            $currentPath = rtrim($currentPath, DIRECTORY_SEPARATOR)
                . DIRECTORY_SEPARATOR
                . $segment;
            clearstatcache(true, $currentPath);
            $directoryMetadata = lstat($currentPath);

            if (
                ! is_array($directoryMetadata)
                || (($directoryMetadata['mode'] & 0170000) !== 0040000)
                || is_link($currentPath)
                || $directoryMetadata['uid'] !== 0
                || (($directoryMetadata['mode'] & 0022) !== 0)
            ) {
                throw new RuntimeException(
                    $label . ' parent directory does not meet the root-owned directory policy.'
                );
            }
        }

        $metadata = lstat($path);

        if (
            ! is_array($metadata)
            || (($metadata['mode'] & 0170000) !== 0100000)
            || is_link($path)
            || $metadata['uid'] !== 0
            || (($metadata['mode'] & 0022) !== 0)
            || (($metadata['mode'] & 0100) !== 0100)
        ) {
            throw new RuntimeException(
                $label . ' is missing or does not meet the root-owned executable policy.'
            );
        }
    }
}
