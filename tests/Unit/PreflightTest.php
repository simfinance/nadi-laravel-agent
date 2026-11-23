<?php

namespace Nadi\LaravelAgent\Tests\Unit;

use Nadi\LaravelAgent\Installer\Preflight;
use Orchestra\Testbench\TestCase;

class PreflightTest extends TestCase
{
    private string $configurationRoot;

    protected function setUp(): void
    {
        parent::setUp();

        $this->configurationRoot = sys_get_temp_dir()
            . DIRECTORY_SEPARATOR
            . 'nadi-php-fpm-'
            . bin2hex(random_bytes(8));

        mkdir(
            $this->configurationRoot . '/8.3/fpm/pool.d',
            0700,
            true
        );
    }

    protected function tearDown(): void
    {
        if (isset($this->configurationRoot) && is_dir($this->configurationRoot)) {
            $files = new \RecursiveIteratorIterator(
                new \RecursiveDirectoryIterator(
                    $this->configurationRoot,
                    \FilesystemIterator::SKIP_DOTS
                ),
                \RecursiveIteratorIterator::CHILD_FIRST
            );

            foreach ($files as $file) {
                if ($file->isDir() && ! $file->isLink()) {
                    rmdir($file->getPathname());
                } else {
                    unlink($file->getPathname());
                }
            }

            rmdir($this->configurationRoot);
        }

        parent::tearDown();
    }

    public function test_it_detects_a_single_php_fpm_runtime_identity(): void
    {
        if (posix_getpwnam('www-data') === false || posix_getgrnam('www-data') === false) {
            self::markTestSkipped('The www-data account is not available on this test host.');
        }

        file_put_contents(
            $this->configurationRoot . '/8.3/fpm/pool.d/www.conf',
            "[www]\nuser = www-data\ngroup = www-data\n"
        );

        $identity = (new Preflight())->detectPhpFpmRuntimeIdentity($this->configurationRoot);

        self::assertIsArray($identity);
        self::assertSame('www-data', $identity['user']);
        self::assertSame('www-data', $identity['group']);
        self::assertSame((int) posix_getpwnam('www-data')['uid'], $identity['uid']);
        self::assertSame((int) posix_getgrnam('www-data')['gid'], $identity['gid']);
    }

    public function test_it_fails_closed_when_pool_files_use_different_runtime_identities(): void
    {
        if (
            posix_getpwnam('www-data') === false
            || posix_getgrnam('www-data') === false
            || posix_getpwnam('nobody') === false
            || posix_getgrnam('nogroup') === false
        ) {
            self::markTestSkipped('Required test accounts are not available on this test host.');
        }

        file_put_contents(
            $this->configurationRoot . '/8.3/fpm/pool.d/www.conf',
            "[www]\nuser = www-data\ngroup = www-data\n"
        );
        file_put_contents(
            $this->configurationRoot . '/8.3/fpm/pool.d/other.conf',
            "[other]\nuser = nobody\ngroup = nogroup\n"
        );

        self::assertNull(
            (new Preflight())->detectPhpFpmRuntimeIdentity($this->configurationRoot)
        );
    }

    public function test_it_rejects_a_root_php_fpm_pool(): void
    {
        file_put_contents(
            $this->configurationRoot . '/8.3/fpm/pool.d/www.conf',
            "[www]\nuser = root\ngroup = root\n"
        );

        self::assertNull(
            (new Preflight())->detectPhpFpmRuntimeIdentity($this->configurationRoot)
        );
    }

    public function test_it_rejects_a_pool_with_an_unresolved_runtime_user(): void
    {
        file_put_contents(
            $this->configurationRoot . '/8.3/fpm/pool.d/www.conf',
            "[www]\nuser = nadi-user-that-does-not-exist\ngroup = nadi-group-that-does-not-exist\n"
        );

        self::assertNull(
            (new Preflight())->detectPhpFpmRuntimeIdentity($this->configurationRoot)
        );
    }

    public function test_it_rejects_a_symlinked_pool_configuration(): void
    {
        $targetPath = $this->configurationRoot . '/pool-target.conf';
        file_put_contents(
            $targetPath,
            "[www]\nuser = www-data\ngroup = www-data\n"
        );

        symlink(
            $targetPath,
            $this->configurationRoot . '/8.3/fpm/pool.d/www.conf'
        );

        self::assertNull(
            (new Preflight())->detectPhpFpmRuntimeIdentity($this->configurationRoot)
        );
    }
}
