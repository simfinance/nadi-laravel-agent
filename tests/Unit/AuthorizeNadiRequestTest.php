<?php

namespace Nadi\\LaravelAgent\\Tests\\Unit;

use Illuminate\\Http\\Request;
use Nadi\\LaravelAgent\\Http\\Middleware\\AuthorizeNadiRequest;
use Orchestra\\Testbench\\TestCase;
use Symfony\\Component\\HttpFoundation\\Response;

class AuthorizeNadiRequestTest extends TestCase
{
    private string $directory;

    protected function setUp(): void
    {
        parent::setUp();

        $this->directory = sys_get_temp_dir()
            . DIRECTORY_SEPARATOR
            . 'nadi-credential-test-'
            . bin2hex(random_bytes(8));

        mkdir($this->directory, 0700, true);
    }

    protected function tearDown(): void
    {
        if (isset($this->directory) && is_dir($this->directory)) {
            foreach (array_diff(scandir($this->directory), ['.', '..']) as $entry) {
                $path = $this->directory . DIRECTORY_SEPARATOR . $entry;

                if (is_link($path) || is_file($path)) {
                    unlink($path);
                }
            }

            rmdir($this->directory);
        }

        parent::tearDown();
    }

    public function test_it_accepts_a_private_regular_credential_file(): void
    {
        $credentialPath = $this->writeCredential('central-issued-credential');

        $response = $this->authorize($credentialPath, 'central-issued-credential');

        self::assertSame(200, $response->getStatusCode());
    }

    public function test_it_rejects_a_symlinked_credential_file(): void
    {
        $targetPath = $this->directory . DIRECTORY_SEPARATOR . 'credential-target';
        $credentialPath = $this->directory . DIRECTORY_SEPARATOR . 'credential';

        file_put_contents($targetPath, 'central-issued-credential');
        chmod($targetPath, 0600);
        symlink($targetPath, $credentialPath);

        $response = $this->authorize($credentialPath, 'central-issued-credential');

        self::assertSame(401, $response->getStatusCode());
    }

    public function test_it_rejects_a_credential_file_with_group_or_other_access(): void
    {
        $credentialPath = $this->writeCredential('central-issued-credential');
        chmod($credentialPath, 0640);

        $response = $this->authorize($credentialPath, 'central-issued-credential');

        self::assertSame(401, $response->getStatusCode());
    }

    public function test_it_rejects_a_credential_file_with_multiple_hard_links(): void
    {
        $credentialPath = $this->writeCredential('central-issued-credential');
        $secondPath = $this->directory . DIRECTORY_SEPARATOR . 'credential-alias';

        link($credentialPath, $secondPath);

        $response = $this->authorize($credentialPath, 'central-issued-credential');

        self::assertSame(401, $response->getStatusCode());
    }

    public function test_it_rejects_an_oversized_credential_file(): void
    {
        $credentialPath = $this->writeCredential(str_repeat('x', 4097));

        $response = $this->authorize($credentialPath, str_repeat('x', 4097));

        self::assertSame(401, $response->getStatusCode());
    }

    private function writeCredential(string $credential): string
    {
        $path = $this->directory . DIRECTORY_SEPARATOR . 'credential';

        file_put_contents($path, $credential);
        chmod($path, 0600);

        return $path;
    }

    private function authorize(string $credentialPath, string $providedCredential): Response
    {
        config([
            'nadi-agent.enabled' => true,
            'nadi-agent.allowed_ips' => ['127.0.0.1'],
            'nadi-agent.credential_file' => $credentialPath,
        ]);

        $request = Request::create(
            '/_nadi/v1/health',
            'GET',
            [],
            [],
            [],
            ['REMOTE_ADDR' => '127.0.0.1']
        );

        $request->headers->set(
            'Authorization',
            'Bearer ' . $providedCredential
        );

        return (new AuthorizeNadiRequest())->handle(
            $request,
            static fn (): Response => new Response('authorized')
        );
    }
}
