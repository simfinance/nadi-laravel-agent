<?php

namespace Nadi\LaravelAgent\Tests\Unit;

use Nadi\LaravelAgent\Http\Middleware\AuthorizeNadiRequest;
use Orchestra\Testbench\TestCase;
use ReflectionMethod;

class AuthorizeNadiRequestCredentialFileTest extends TestCase
{
    private array $temporaryPaths = [];

    protected function tearDown(): void
    {
        foreach ($this->temporaryPaths as $path) {
            if (is_link($path) || is_file($path)) {
                unlink($path);
            }
        }

        parent::tearDown();
    }

    public function test_it_reads_a_private_regular_credential_file(): void
    {
        $path = $this->createCredentialFile('nadi-credential-test-value');

        config([
            'nadi-agent.credential_file' => $path,
        ]);

        self::assertSame(
            'nadi-credential-test-value',
            $this->readCredential()
        );
    }

    public function test_it_rejects_a_credential_file_with_group_or_other_permissions(): void
    {
        $path = $this->createCredentialFile('nadi-credential-test-value');
        chmod($path, 0640);

        config([
            'nadi-agent.credential_file' => $path,
        ]);

        self::assertNull($this->readCredential());
    }

    public function test_it_rejects_a_symbolic_link(): void
    {
        $target = $this->createCredentialFile('nadi-credential-test-value');
        $link = $target . '.link';
        symlink($target, $link);
        $this->temporaryPaths[] = $link;

        config([
            'nadi-agent.credential_file' => $link,
        ]);

        self::assertNull($this->readCredential());
    }

    public function test_it_rejects_an_oversized_credential_file(): void
    {
        $path = $this->createCredentialFile(str_repeat('x', 4097));

        config([
            'nadi-agent.credential_file' => $path,
        ]);

        self::assertNull($this->readCredential());
    }

    public function test_it_rejects_a_relative_credential_path(): void
    {
        config([
            'nadi-agent.credential_file' => 'storage/app/private/nadi/credential',
        ]);

        self::assertNull($this->readCredential());
    }

    private function createCredentialFile(string $credential): string
    {
        $path = tempnam(sys_get_temp_dir(), 'nadi-credential-');

        file_put_contents($path, $credential);
        chmod($path, 0600);

        $this->temporaryPaths[] = $path;

        return $path;
    }

    private function readCredential(): ?string
    {
        $middleware = new AuthorizeNadiRequest();
        $method = new ReflectionMethod($middleware, 'readApplicationCredential');

        return $method->invoke($middleware);
    }
}
