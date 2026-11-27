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
     * Additional Laravel applications enrolled by the same server Agent.
     * Each entry must define application_name, environment, base_url,
     * base_path, runtime_user, and runtime_group.
     */
    'additional_applications' => [],

    /*
     * Optional PHP-FPM identity override for the current Laravel application.
     * Required when the host has multiple PHP-FPM pools.
     */
    'runtime_user' => env('NADI_RUNTIME_USER'),

    'runtime_group' => env('NADI_RUNTIME_GROUP'),

    /*
     * Runtime authorization values are loaded only from the root-owned,
     * installer-managed per-application configuration outside the app tree.
     * Missing or invalid managed configuration keeps the Bridge disabled.
     */
    'enabled' => false,

    'route' => [
        'prefix' => '_nadi/v1',
        'middleware' => ['api'],
        'throttle' => '60,1',
    ],

    'allowed_ips' => [],

    'credential_file' => null,

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
