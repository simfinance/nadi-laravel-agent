<?php

namespace Nadi\LaravelAgent\Installer;

use Illuminate\Contracts\Foundation\Application;
use RuntimeException;

class EnrollmentManifestBuilder
{
    public function __construct(
        private readonly Application $application
    ) {
    }

    public function build(): array
    {
        $baseUrl = rtrim((string) config('app.url'), '/');

        if ($baseUrl === '') {
            throw new RuntimeException('APP_URL is required for NADI enrollment.');
        }

        $parsedUrl = parse_url($baseUrl);

        if (
            ! is_array($parsedUrl)
            || ($parsedUrl['scheme'] ?? null) !== 'https'
            || empty($parsedUrl['host'])
        ) {
            throw new RuntimeException(
                'APP_URL must be an absolute HTTPS URL before NADI enrollment.'
            );
        }

        $applicationName = trim((string) config('app.name'));

        if ($applicationName === '') {
            throw new RuntimeException('APP_NAME is required for NADI enrollment.');
        }

        $environment = trim((string) config('app.env'));

        if ($environment === '') {
            throw new RuntimeException('APP_ENV is required for NADI enrollment.');
        }

        $applicationPath = realpath($this->application->basePath());

        if ($applicationPath === false) {
            throw new RuntimeException('Unable to resolve the Laravel application path.');
        }

        $applicationKey = $this->buildApplicationKey(
            $applicationName,
            $baseUrl,
            $environment,
            $applicationPath
        );

        return [
            'applications' => [
                [
                    'application_key' => $applicationKey,
                    'application_name' => $applicationName,
                    'environment' => $environment,
                    'framework_version' => app()->version(),
                    'php_version' => PHP_VERSION,
                    'base_url' => $baseUrl,
                ],
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
