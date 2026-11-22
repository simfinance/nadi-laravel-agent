<?php

return [
    'central_url' => 'https://nadi.pesantrenalirsyad.org',

    'release_manifest_url' => 'https://nadi.pesantrenalirsyad.org/api/v1/nadi/releases/manifest',

    'release_public_key' => '',

    'agent_version' => '0.7.0',

    /*
     * Installer-managed settings. The default state is deliberately disabled.
     * These values are written by the installer after secure enrollment.
     */
    'enabled' => false,

    'route' => [
        'prefix' => '_nadi/v1',
        'middleware' => ['api'],
        'throttle' => '60,1',
    ],

    /*
     * The source-IP allowlist must be populated from the Central source IP
     * verified by infrastructure. An empty list denies all requests.
     */
    'allowed_ips' => [],

    /*
     * Application credential is stored outside the webroot. The installer
     * provisions this file and its ownership/permissions.
     */
    'credential_file' => storage_path('app/private/nadi/credential'),

    'database' => [
        'cache_seconds' => 10,
    ],

    'info' => [
        'application_name' => env('APP_NAME', 'Laravel'),
        'environment' => env('APP_ENV', 'production'),
    ],
];