<?php

namespace Nadi\LaravelAgent\Tests\Unit;

use Nadi\LaravelAgent\Installer\PrivilegedProvisioner;
use Orchestra\Testbench\TestCase;
use RuntimeException;

class PrivilegedProvisionerTest extends TestCase
{
    public function test_it_fails_closed_when_the_root_owned_helper_is_not_installed(): void
    {
        $provisioner = new PrivilegedProvisioner('/nonexistent/nadi-provision');

        try {
            $provisioner->assertAvailable();
            self::fail('Expected the missing privileged helper to be rejected.');
        } catch (RuntimeException $exception) {
            self::assertStringContainsString(
                'NADI provisioning helper',
                $exception->getMessage()
            );
        }
    }

    public function test_it_accepts_only_a_complete_success_response(): void
    {
        $provisioner = new PrivilegedProvisioner();

        $response = $provisioner->decodeResponse(json_encode([
            'protocol_version' => 1,
            'success' => true,
            'agent_version' => '0.7.5',
            'enrollment_status' => 'registered',
            'application_count' => 1,
        ], JSON_THROW_ON_ERROR));

        self::assertSame('registered', $response['enrollment_status']);
        self::assertSame(1, $response['application_count']);
    }

    public function test_it_rejects_unknown_response_fields(): void
    {
        $provisioner = new PrivilegedProvisioner();

        $this->expectException(RuntimeException::class);
        $this->expectExceptionMessage('response fields are invalid');

        $provisioner->decodeResponse(json_encode([
            'protocol_version' => 1,
            'success' => true,
            'agent_version' => '0.7.5',
            'enrollment_status' => 'registered',
            'application_count' => 1,
            'agent_credential' => 'must-not-be-returned',
        ], JSON_THROW_ON_ERROR));
    }

    public function test_it_rejects_a_response_that_does_not_confirm_registration(): void
    {
        $provisioner = new PrivilegedProvisioner();

        $this->expectException(RuntimeException::class);
        $this->expectExceptionMessage('did not confirm a completed enrollment');

        $provisioner->decodeResponse(json_encode([
            'protocol_version' => 1,
            'success' => true,
            'agent_version' => '0.7.5',
            'enrollment_status' => 'pending',
            'application_count' => 1,
        ], JSON_THROW_ON_ERROR));
    }

    public function test_it_rejects_invalid_json_and_trailing_json_values(): void
    {
        $provisioner = new PrivilegedProvisioner();

        try {
            $provisioner->decodeResponse('{"protocol_version":1} {}');
            self::fail('Expected trailing JSON to be rejected.');
        } catch (RuntimeException $exception) {
            self::assertSame(
                'NADI provisioning helper returned invalid JSON.',
                $exception->getMessage()
            );
        }
    }

    public function test_it_rejects_an_oversized_response(): void
    {
        $provisioner = new PrivilegedProvisioner();

        $this->expectException(RuntimeException::class);
        $this->expectExceptionMessage('empty or too large');

        $provisioner->decodeResponse(str_repeat('x', 4097));
    }

    public function test_it_does_not_accept_a_success_response_with_zero_applications(): void
    {
        $provisioner = new PrivilegedProvisioner();

        $this->expectException(RuntimeException::class);
        $this->expectExceptionMessage('did not confirm a completed enrollment');

        $provisioner->decodeResponse(json_encode([
            'protocol_version' => 1,
            'success' => true,
            'agent_version' => '0.7.5',
            'enrollment_status' => 'registered',
            'application_count' => 0,
        ], JSON_THROW_ON_ERROR));
    }
}
