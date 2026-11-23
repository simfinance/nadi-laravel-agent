<?php

namespace Nadi\\LaravelAgent\\Installer;

use Illuminate\\Contracts\\Foundation\\Application;
use RuntimeException;

class EnrollmentManifestBuilder
{
    public function __construct(
        private readonly Application $application
    ) {
    }

    public function build(): array
    {
        $inventory = $this->inventory();

        return [
            'applications' => array_map(
                static fn (array $application): array => $application['manifest'],
                $inventory
            ),
        ];
    }

    public function buildTargets(): array
    {
        return array_map(
            static fn (array $application): array => $application['target'],
            $this->inventory()
        );
    }

    private function inventory(): array
    {
        $inventory = [];
        $currentPath = realpath($this->application->basePath());

        if ($currentPath === false || ! is_file($currentPath . DIRECTORY_SEPARATOR . 'artisan')) {
            throw new RuntimeException('Unable to resolve the current Laravel application path.');
        }

        $inventory[] = $this->buildApplication(
            trim((string) config('app.name')),
            trim((string) config('app.env')),
            rtrim((string) config('app.url'), '/'),
            $currentPath,
            config('nadi-agent.runtime_user'),
            config('nadi-agent.runtime_group')
        );

        $additionalApplications = config('nadi-agent.additional_applications', []);

        if (! is_array($additionalApplications) || ! array_is_list($additionalApplications)) {
            throw new RuntimeException('NADI additional_applications must be a JSON-style list.');
        }

        foreach ($additionalApplications as $index => $application) {
            if (! is_array($application)) {
                throw new RuntimeException(
                    'NADI additional application at index ' . $index . ' must be an object.'
                );
            }

            $basePath = $application['base_path'] ?? null;
            $resolvedPath = is_string($basePath) ? realpath($basePath) : false;

            if (
                $resolvedPath === false
                || ! is_file($resolvedPath . DIRECTORY_SEPARATOR . 'artisan')
            ) {
                throw new RuntimeException(
                    'NADI additional application at index ' . $index . ' has an invalid Laravel base_path.'
                );
            }

            $inventory[] = $this->buildApplication(
                trim((string) ($application['application_name'] ?? '')),
                trim((string) ($application['environment'] ?? '')),
                rtrim((string) ($application['base_url'] ?? ''), '/'),
                $resolvedPath,
                $application['runtime_user'] ?? null,
                $application['runtime_group'] ?? null
            );
        }

        if (count($inventory) < 1 || count($inventory) > 20) {
            throw new RuntimeException(
                'One NADI Agent enrollment must contain between 1 and 20 applications.'
            );
        }

        $seenKeys = [];
        $seenPaths = [];

        foreach ($inventory as $index => $application) {
            $key = $application['manifest']['application_key'];
            $path = $application['target']['base_path'];

            if (isset($seenKeys[$key])) {
                throw new RuntimeException('NADI application inventory contains a duplicate application_key.');
            }

            if (isset($seenPaths[$path])) {
                throw new RuntimeException('NADI application inventory contains a duplicate base_path.');
            }

            $seenKeys[$key] = true;
            $seenPaths[$path] = true;
        }

        return $inventory;
    }

    private function buildApplication(
        string $applicationName,
        string $environment,
        string $baseUrl,
        string $applicationPath,
        mixed $runtimeUser,
        mixed $runtimeGroup
    ): array {
        if ($baseUrl === '') {
            throw new RuntimeException('Every NADI application requires an absolute HTTPS base_url.');
        }

        $parsedUrl = parse_url($baseUrl);

        if (
            ! is_array($parsedUrl)
            || ($parsedUrl['scheme'] ?? null) !== 'https'
            || empty($parsedUrl['host'])
            || isset($parsedUrl['user'])
            || isset($parsedUrl['pass'])
        ) {
            throw new RuntimeException('Every NADI application base_url must be an absolute HTTPS URL.');
        }

        if ($applicationName === '') {
            throw new RuntimeException('Every NADI application requires application_name.');
        }

        if ($environment === '') {
            throw new RuntimeException('Every NADI application requires environment.');
        }

        if (
            $runtimeUser !== null
            && (! is_string($runtimeUser) || ! preg_match('/^[a-zA-Z_][a-zA-Z0-9_.-]*$/D', $runtimeUser))
        ) {
            throw new RuntimeException('NADI application runtime_user is invalid.');
        }

        if (
            $runtimeGroup !== null
            && (! is_string($runtimeGroup) || ! preg_match('/^[a-zA-Z_][a-zA-Z0-9_.-]*$/D', $runtimeGroup))
        ) {
            throw new RuntimeException('NADI application runtime_group is invalid.');
        }

        $applicationKey = $this->buildApplicationKey(
            $applicationName,
            $baseUrl,
            $environment,
            $applicationPath
        );

        return [
            'manifest' => [
                'application_key' => $applicationKey,
                'application_name' => $applicationName,
                'environment' => $environment,
                'framework_version' => app()->version(),
                'php_version' => PHP_VERSION,
                'base_url' => $baseUrl,
            ],
            'target' => [
                'application_key' => $applicationKey,
                'base_path' => $applicationPath,
                'runtime_user' => $runtimeUser,
                'runtime_group' => $runtimeGroup,
            ],
        ];
    }

    private function buildApplicationKey(
        string $applicationName,
        string $baseUrl,
        string $environment,
        string $applicationPath
    ): string {
        $slug = strtolower(trim((string) preg_replace(
            '/[^a-zA-Z0-9]+/',
            '-',
            $applicationName
        ), '-'));

        $slug = substr($slug !== '' ? $slug : 'laravel-app', 0, 48);
        $fingerprint = substr(hash(
            'sha256',
            $baseUrl . '|' . $environment . '|' . $applicationPath
        ), 0, 16);

        return $slug . '-' . $environment . '-' . $fingerprint;
    }
}
