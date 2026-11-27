<?php

namespace Nadi\LaravelAgent\Tests\Unit;

use Nadi\LaravelAgent\Installer\BridgeApplicationConfiguration;
use Orchestra\Testbench\TestCase;

class BridgeApplicationConfigurationTest extends TestCase
{
    private string $basePath;

    private string $configurationDirectory;

    private int $effectiveUid;

    private int $effectiveGid;

    protected function setUp(): void
    {
        parent::setUp();

        $this->effectiveUid = posix_geteuid();
        $this->effectiveGid = posix_getegid();
        $root = sys_get_temp_dir() . '/nadi-bridge-config-' . bin2hex(random_bytes(8));
        $this->basePath = $root . '/application';
        $this->configurationDirectory = $root . '/config';

        mkdir($this->basePath, 0700, true);
        mkdir($this->configurationDirectory, 0711, true);
    }

    protected function tearDown(): void
    {
        $this->removeDirectory(dirname($this->basePath));

        parent::tearDown();
    }

    public function test_it_loads_the_root_managed_configuration_for_its_canonical_application_path(): void
    {
        $this->writeConfiguration($this->validConfiguration());

        $configuration = $this->reader()->load(
            $this->basePath,
            $this->effectiveGid
        );

        self::assertIsArray($configuration);
        self::assertSame(true, $configuration['enabled']);
        self::assertSame(['203.0.113.10'], $configuration['allowed_ips']);
        self::assertSame(
            '/var/lib/nadi-bridge/123e4567-e89b-42d3-a456-426614174000/credential',
            $configuration['credential_file']
        );
        self::assertSame(
            '123e4567-e89b-42d3-a456-426614174000',
            $configuration['activation_application_id']
        );
    }

    public function test_it_denies_a_configuration_file_owned_by_another_runtime_group(): void
    {
        $this->writeConfiguration($this->validConfiguration());

        self::assertNull(
            $this->reader()->load(
                $this->basePath,
                $this->effectiveGid + 1
            )
        );
    }

    public function test_it_denies_a_configuration_file_with_unsafe_permissions(): void
    {
        $path = $this->writeConfiguration($this->validConfiguration());
        chmod($path, 0660);

        self::assertNull(
            $this->reader()->load($this->basePath, $this->effectiveGid)
        );
    }

    public function test_it_denies_a_configuration_with_an_unexpected_field(): void
    {
        $configuration = $this->validConfiguration();
        $configuration['unexpected'] = true;
        $this->writeConfiguration($configuration);

        self::assertNull(
            $this->reader()->load($this->basePath, $this->effectiveGid)
        );
    }

    public function test_it_denies_a_configuration_with_an_empty_allowlist(): void
    {
        $configuration = $this->validConfiguration();
        $configuration['allowed_ips'] = [];
        $this->writeConfiguration($configuration);

        self::assertNull(
            $this->reader()->load($this->basePath, $this->effectiveGid)
        );
    }

    public function test_it_denies_a_configuration_with_a_caller_selected_credential_path(): void
    {
        $configuration = $this->validConfiguration();
        $configuration['credential_file'] = '/tmp/other-application-credential';
        $this->writeConfiguration($configuration);

        self::assertNull(
            $this->reader()->load($this->basePath, $this->effectiveGid)
        );
    }

    public function test_it_denies_a_symlinked_configuration_file(): void
    {
        $path = $this->reader()->pathForBasePath(realpath($this->basePath));
        $target = $this->configurationDirectory . '/target.json';

        file_put_contents($target, json_encode($this->validConfiguration(), JSON_THROW_ON_ERROR));
        chmod($target, 0640);
        symlink($target, $path);

        self::assertNull(
            $this->reader()->load($this->basePath, $this->effectiveGid)
        );
    }

    public function test_it_denies_when_the_managed_configuration_is_missing(): void
    {
        self::assertNull(
            $this->reader()->load($this->basePath, $this->effectiveGid)
        );
    }

    private function reader(): BridgeApplicationConfiguration
    {
        return new BridgeApplicationConfiguration(
            $this->configurationDirectory,
            $this->effectiveUid,
            $this->effectiveUid,
            $this->effectiveGid
        );
    }

    private function writeConfiguration(array $configuration): string
    {
        $path = $this->reader()->pathForBasePath(realpath($this->basePath));
        file_put_contents($path, json_encode($configuration, JSON_THROW_ON_ERROR));
        chown($path, $this->effectiveUid);
        chgrp($path, $this->effectiveGid);
        chmod($path, 0640);

        return $path;
    }

    private function validConfiguration(): array
    {
        return [
            'schema_version' => 1,
            'enabled' => true,
            'application_id' => '123e4567-e89b-42d3-a456-426614174000',
            'application_key' => 'application-production-1',
            'allowed_ips' => ['203.0.113.10'],
            'credential_file' => '/var/lib/nadi-bridge/123e4567-e89b-42d3-a456-426614174000/credential',
            'activation_state_file' => '/run/nadi/bridge-state/123e4567-e89b-42d3-a456-426614174000.json',
        ];
    }

    private function removeDirectory(string $directory): void
    {
        if (! is_dir($directory) || is_link($directory)) {
            return;
        }

        foreach (scandir($directory) ?: [] as $entry) {
            if ($entry === '.' || $entry === '..') {
                continue;
            }

            $path = $directory . DIRECTORY_SEPARATOR . $entry;

            if (is_link($path) || is_file($path)) {
                unlink($path);
                continue;
            }

            $this->removeDirectory($path);
        }

        rmdir($directory);
    }
}
