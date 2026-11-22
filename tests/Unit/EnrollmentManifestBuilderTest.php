<?php

namespace Nadi\LaravelAgent\Tests\Unit;

use Illuminate\Contracts\Foundation\Application;
use Nadi\LaravelAgent\Installer\EnrollmentManifestBuilder;
use Orchestra\Testbench\TestCase;
use RuntimeException;

class EnrollmentManifestBuilderTest extends TestCase
{
    private string $applicationPath;

    protected function setUp(): void
    {
        parent::setUp();

        $this->applicationPath = sys_get_temp_dir()
            . DIRECTORY_SEPARATOR
            . 'nadi-enrollment-'
            . bin2hex(random_bytes(8));

        mkdir($this->applicationPath, 0700, true);
    }

    protected function tearDown(): void
    {
        if (isset($this->applicationPath) && is_dir($this->applicationPath))
        {
            rmdir($this->applicationPath);
        }

        parent::tearDown();
    }

    public function test_it_builds_a_stable_single_application_manifest(): void
    {
        config([
            'app.url' => 'https://example.org',
            'app.name' => 'Example App',
            'app.env' => 'production',
        ]);

        $application = $this->createMock(Application::class);
        $application->method('basePath')->willReturn($this->applicationPath);

        $builder = new EnrollmentManifestBuilder($application);
        $manifest = $builder->build();

        self::assertCount(1, $manifest['applications']);

        self::assertSame(
            'https://example.org',
            $manifest['applications'][0]['base_url']
        );

        self::assertStringStartsWith(
            'example-app-production-',
            $manifest['applications'][0]['application_key']
        );
    }

    public function test_it_rejects_non_https_application_url(): void
    {
        config([
            'app.url' => 'http://example.org',
            'app.name' => 'Example App',
            'app.env' => 'production',
        ]);

        $application = $this->createMock(Application::class);
        $application->method('basePath')->willReturn($this->applicationPath);

        $this->expectException(RuntimeException::class);

        (new EnrollmentManifestBuilder($application))->build();
    }
}
