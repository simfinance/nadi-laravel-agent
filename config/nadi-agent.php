<?php

return [
    'central_url' => 'https://nadi.pesantrenalirsyad.org',

    'release_manifest_url' => 'https://nadi.pesantrenalirsyad.org/api/v1/nadi/releases/manifest',

    'release_key_id' => 'nadi-agent-release-2026-01',

    'release_public_key' => is_file(dirname(__DIR__) . '/resources/agent/release-signing-public.pem')
        ? (string) file_get_contents(dirname(__DIR__) . '/resources/agent/release-signing-public.pem')
        : '',

    'release_public_key_fingerprint' => '91d063a323ffb7feaad3e4bf39d750ec339b2206b74f762d11e1f291267a49e1',

    'activation_key_id' => 'nadi-central-2026-01',

    'activation_public_key' => is_file(dirname(__DIR__) . '/resources/agent/activation-public.pem')
        ? (string) file_get_contents(dirname(__DIR__) . '/resources/agent/activation-public.pem')
        : '',

    'activation_public_key_fingerprint' => '160a8cfa0ebe7b109230a6c15c8f3aa7fc0bfa4f8cdb1239de9bb8dee348ba25',

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

    /*
     * Root-owned activation assertion published by the local handoff broker.
     * Null deliberately denies activation until provisioning configures it.
     */
    'activation_state_file' => null,

    'activation_application_id' => null,

    'activation_application_key' => null,

    'logs' => [
        'application' => [
            'max_entries' => 50,
            'max_bytes' => 1048576,
        ],
    ],

    'database' => [
        'cache_seconds' => 10,
    ],

    'info' => [
        'application_name' => env('APP_NAME', 'Laravel'),
        'environment' => env('APP_ENV', 'production'),
    ],
];
