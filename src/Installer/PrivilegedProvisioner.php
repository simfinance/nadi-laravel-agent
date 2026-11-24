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

    public function __construct(
        private readonly string $helperPath = self::HELPER_PATH,
        private readonly string $sudoPath = self::SUDO_PATH
    ) {
    }

    public function assertAvailable(): void
    {
        $this->assertTrustedExecutable($this->helperPath, 'NADI provisioning helper');
        $this->assertTrustedExecutable($this->sudoPath, 'sudo');
    }

    public function installAgent(
        array $signedRelease,
        string $artifactPath,
        array $applicationManifest,
        array $applicationTargets,
        array $runtimeIdentities,
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

        $manifestApplications = $applicationManifest['applications'] ?? null;

        if (
            ! is_array($manifestApplications)
            || ! array_is_list($manifestApplications)
            || count($manifestApplications) < 1
            || count($manifestApplications) > 20
            || count($applicationTargets) !== count($manifestApplications)
            || count($runtimeIdentities) !== count($manifestApplications)
        ) {
            throw new RuntimeException('One Agent enrollment must contain 1 to 20 complete application mappings.');
        }

        $identityByKey = [];
        $seenUIDs = [];

        foreach ($runtimeIdentities as $identity) {
            if (
                ! is_array($identity)
                || ! is_string($identity['application_key'] ?? null)
                || ! is_int($identity['uid'] ?? null)
                || ! is_int($identity['gid'] ?? null)
                || $identity['uid'] < 1
                || $identity['gid'] < 1
            ) {
                throw new RuntimeException('A verified non-root PHP-FPM identity is missing for an application.');
            }

            $key = $identity['application_key'];

            if (isset($identityByKey[$key]) || isset($seenUIDs[$identity['uid']])) {
                throw new RuntimeException('Application runtime identities must have unique application keys and UIDs.');
            }

            $identityByKey[$key] = $identity;
            $seenUIDs[$identity['uid']] = true;
        }

        $manifestByKey = [];

        foreach ($manifestApplications as $application) {
            if (! is_array($application) || ! is_string($application['application_key'] ?? null)) {
                throw new RuntimeException('Enrollment manifest contains an invalid application identity.');
            }

            $key = $application['application_key'];

            if (isset($manifestByKey[$key])) {
                throw new RuntimeException('Enrollment manifest contains duplicate application keys.');
            }

            $manifestByKey[$key] = $application;
        }

        $applicationRequests = [];
        $resolvedPaths = [];
        $seenTargetKeys = [];

        foreach ($applicationTargets as $target) {
            if (
                ! is_array($target)
                || ! is_string($target['application_key'] ?? null)
                || ! is_string($target['base_path'] ?? null)
            ) {
                throw new RuntimeException('Application provisioning target is invalid.');
            }

            $key = $target['application_key'];
            $identity = $identityByKey[$key] ?? null;

            if (
                isset($seenTargetKeys[$key])
                || ! isset($manifestByKey[$key])
                || ! is_array($identity)
                || ($identity['base_path'] ?? null) !== $target['base_path']
            ) {
                throw new RuntimeException('Application target does not uniquely match the enrollment manifest and PHP-FPM identity.');
            }

            $seenTargetKeys[$key] = true;

            $resolvedPath = realpath($target['base_path']);

            if (
                $resolvedPath === false
                || $resolvedPath !== $target['base_path']
                || is_link($target['base_path'])
                || ! is_file($resolvedPath . DIRECTORY_SEPARATOR . 'artisan')
                || is_link($resolvedPath . DIRECTORY_SEPARATOR . 'artisan')
            ) {
                throw new RuntimeException('Laravel application path is unsafe or not canonical.');
            }

            foreach ($resolvedPaths as $existingPath) {
                if (
                    $resolvedPath === $existingPath
                    || str_starts_with($resolvedPath, $existingPath . DIRECTORY_SEPARATOR)
                    || str_starts_with($existingPath, $resolvedPath . DIRECTORY_SEPARATOR)
                ) {
                    throw new RuntimeException('Laravel application paths must not overlap.');
                }
            }

            $resolvedPaths[] = $resolvedPath;
            $applicationRequests[] = [
                'application_key' => $key,
                'base_path' => $resolvedPath,
                'runtime_uid' => $identity['uid'],
                'runtime_gid' => $identity['gid'],
            ];
        }

        if (count($applicationRequests) !== count($manifestByKey)) {
            throw new RuntimeException('Every enrollment application must have exactly one local provisioning target.');
        }

        $resolvedArtifactPath = realpath($artifactPath);
        $stagingRoot = realpath(storage_path('app/private/nadi/releases/staging'));

        if (
            $resolvedArtifactPath === false
            || $stagingRoot === false
            || ! is_file($resolvedArtifactPath)
            || is_link($artifactPath)
            || ! str_starts_with(
                $resolvedArtifactPath,
                $stagingRoot . DIRECTORY_SEPARATOR
            )
        ) {
            throw new RuntimeException('Agent artifact staging path is unsafe.');
        }

        $releasePayload = [
            'manifest_base64' => base64_encode($manifestJson),
            'signature_base64' => $signature,
            'artifact_path' => $resolvedArtifactPath,
        ];
        $enrollmentPayload = [
            'manifest_base64' => base64_encode(
                json_encode(
                    $applicationManifest,
                    JSON_UNESCAPED_SLASHES | JSON_THROW_ON_ERROR
                )
            ),
            'token' => $enrollmentToken,
        ];

        try {
            $installed = $this->invokeOperation([
                'protocol_version' => 1,
                'operation' => 'install_agent',
                'release' => $releasePayload,
                'applications' => $applicationRequests,
            ]);

            if (
                $installed['phase'] !== 'installed'
                || $installed['enrollment_status'] !== 'not_started'
                || ! $installed['retry_safe']
            ) {
                throw new RuntimeException('NADI install_agent operation returned an inconsistent outcome.');
            }

            $enrolled = $this->invokeOperation([
                'protocol_version' => 1,
                'operation' => 'enroll_agent',
                'enrollment' => $enrollmentPayload,
            ]);

            if (
                $enrolled['phase'] !== 'enrolled'
                || $enrolled['enrollment_status'] !== 'registered'
                || $enrolled['retry_safe']
            ) {
                throw new RuntimeException('NADI enroll_agent operation returned an inconsistent outcome. Do not retry enrollment automatically.');
            }

            $started = $this->invokeOperation([
                'protocol_version' => 1,
                'operation' => 'start_agent',
            ]);

            if (
                $started['phase'] !== 'completed'
                || $started['enrollment_status'] !== 'registered'
                || $started['retry_safe']
            ) {
                throw new RuntimeException('NADI start_agent operation returned an inconsistent outcome.');
            }

            return $started;
        } finally {
            if (function_exists('sodium_memzero')) {
                sodium_memzero($enrollmentToken);
                sodium_memzero($enrollmentPayload['token']);
            }

            unset($enrollmentPayload, $releasePayload);
        }
    }

    private function invokeOperation(array $request): array
    {
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
            $this->sudoPath,
            '-n',
            '--',
            $this->helperPath,
            '--request-stdin',
        ]);

        $stdin = $requestBody . "\n";
        $process->setInput($stdin);
        $process->setTimeout(360);
        $process->setEnv([
            'NADI_ENROLLMENT_TOKEN' => false,
            'NADI_AGENT_TOKEN' => false,
        ]);

        try {
            try {
                $process->run();
            } catch (Throwable $exception) {
                throw new RuntimeException(
                    'NADI privileged provisioning operation could not be completed. If enrollment may have started, do not retry with a new token.',
                    0,
                    $exception
                );
            }

            try {
                $response = $this->decodeResponse($process->getOutput());
            } catch (Throwable $exception) {
                throw new RuntimeException(
                    'NADI provisioning helper exited without a valid outcome envelope. Enrollment status may be unknown; do not retry enrollment automatically.',
                    0,
                    $exception
                );
            }

            if (! $process->isSuccessful() || $response['success'] !== true) {
                throw new RuntimeException(sprintf(
                    'NADI provisioning operation "%s" stopped during phase "%s"; enrollment_status="%s"; retry_safe=%s. %s',
                    $request['operation'],
                    $response['phase'],
                    $response['enrollment_status'],
                    $response['retry_safe'] ? 'true' : 'false',
                    $response['message']
                ));
            }

            return $response;
        } finally {
            if (function_exists('sodium_memzero')) {
                sodium_memzero($requestBody);
                sodium_memzero($stdin);

                if (isset($request['enrollment']['token'])) {
                    sodium_memzero($request['enrollment']['token']);
                }
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
            'phase',
            'enrollment_status',
            'retry_safe',
            'agent_version',
            'application_count',
            'message',
        ];

        $actualKeys = array_keys($response);
        sort($expectedKeys);
        sort($actualKeys);

        if ($actualKeys !== $expectedKeys) {
            throw new RuntimeException('NADI provisioning helper response fields are invalid.');
        }

        if (
            $response['protocol_version'] !== 1
            || ! is_bool($response['success'])
            || ! is_string($response['phase'])
            || ! in_array($response['phase'], [
                'request_validation',
                'release_verification',
                'existing_installation',
                'os_provisioning',
                'installed',
                'enrollment',
                'broker_configuration',
                'enrolled',
                'service_activation',
                'completed',
            ], true)
            || ! is_string($response['enrollment_status'])
            || ! in_array($response['enrollment_status'], [
                'not_started',
                'unknown',
                'registered',
            ], true)
            || ! is_bool($response['retry_safe'])
            || ! is_string($response['agent_version'])
            || ! is_int($response['application_count'])
            || $response['application_count'] < 0
            || $response['application_count'] > 20
            || ! is_string($response['message'])
            || $response['message'] === ''
            || strlen($response['message']) > 512
        ) {
            throw new RuntimeException('NADI provisioning helper response values are invalid.');
        }

        if (
            ($response['retry_safe'] && $response['enrollment_status'] !== 'not_started')
            || ($response['success'] && (
                ! in_array($response['phase'], ['installed', 'enrolled', 'completed'], true)
                || $response['agent_version'] === ''
                || $response['application_count'] < 1
                || ($response['phase'] === 'installed' && (
                    $response['enrollment_status'] !== 'not_started'
                    || ! $response['retry_safe']
                ))
                || (in_array($response['phase'], ['enrolled', 'completed'], true) && (
                    $response['enrollment_status'] !== 'registered'
                    || $response['retry_safe']
                ))
            ))
            || (! $response['success'] && in_array($response['phase'], ['installed', 'enrolled', 'completed'], true))
        ) {
            throw new RuntimeException('NADI provisioning helper outcome consistency check failed.');
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

            if (! file_exists($currentPath) && ! is_link($currentPath)) {
                throw new RuntimeException(
                    $label . ' parent directory does not meet the root-owned directory policy.'
                );
            }

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

        if (! file_exists($path) && ! is_link($path)) {
            throw new RuntimeException(
                $label . ' is missing or does not meet the root-owned executable policy.'
            );
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
