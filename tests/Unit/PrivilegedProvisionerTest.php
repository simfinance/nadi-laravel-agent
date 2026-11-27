<?php

namespace Nadi\LaravelAgent\Tests\Unit;

use Nadi\LaravelAgent\Installer\PrivilegedProvisioner;
use Orchestra\Testbench\TestCase;
use RuntimeException;

class PrivilegedProvisionerTest extends TestCase
{
    public function test_root_invocation_does_not_require_sudo(): void
    {
        $provisioner = new PrivilegedProvisioner(
            '/usr/bin/true',
            '/nonexistent/sudo',
            0
        );

        $provisioner->assertAvailable();

        $method = new \ReflectionMethod($provisioner, 'invocationCommand');

        self::assertSame(
            [
                '/usr/bin/true',
                '--request-stdin',
            ],
            $method->invoke($provisioner)
        );
    }

    public function test_non_root_invocation_uses_non_interactive_sudo(): void
    {
        $provisioner = new PrivilegedProvisioner(
            '/usr/bin/true',
            '/usr/bin/sudo',
            1000
        );

        $method = new \ReflectionMethod($provisioner, 'invocationCommand');

        self::assertSame(
            [
                '/usr/bin/sudo',
                '-n',
                '--',
                '/usr/bin/true',
                '--request-stdin',
            ],
            $method->invoke($provisioner)
        );
    }

    public function test_non_root_installation_requires_a_trusted_sudo_executable(): void
    {
        $provisioner = new PrivilegedProvisioner(
            '/usr/bin/true',
            '/nonexistent/sudo',
            1000
        );

        $this->expectException(RuntimeException::class);
        $this->expectExceptionMessage('sudo');

        $provisioner->assertAvailable();
    }

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
            'phase' => 'completed',
            'enrollment_status' => 'registered',
            'retry_safe' => false,
            'agent_version' => '0.7.5',
            'application_count' => 1,
            'message' => 'NADI Agent enrollment and service startup completed.',
        ], JSON_THROW_ON_ERROR));

        self::assertSame('registered', $response['enrollment_status']);
        self::assertSame(1, $response['application_count']);
    }


    public function test_it_accepts_a_non_retryable_partial_enrollment_outcome(): void
    {
        $provisioner = new PrivilegedProvisioner();

        $response = $provisioner->decodeResponse(json_encode([
            'protocol_version' => 1,
            'success' => false,
            'phase' => 'enrollment',
            'enrollment_status' => 'unknown',
            'retry_safe' => false,
            'agent_version' => 'unknown',
            'application_count' => 2,
            'message' => 'Central enrollment outcome is uncertain.',
        ], JSON_THROW_ON_ERROR));

        self::assertFalse($response['success']);
        self::assertSame('unknown', $response['enrollment_status']);
        self::assertFalse($response['retry_safe']);
    }

    public function test_it_accepts_a_recovery_required_failure_outcome(): void
    {
        $provisioner = new PrivilegedProvisioner();

        $response = $provisioner->decodeResponse(json_encode([
            'protocol_version' => 1,
            'success' => false,
            'phase' => 'recovery_required',
            'enrollment_status' => 'unknown',
            'retry_safe' => false,
            'agent_version' => '0.7.5',
            'application_count' => 2,
            'message' => 'Existing Agent identity requires manual reconciliation.',
        ], JSON_THROW_ON_ERROR));

        self::assertFalse($response['success']);
        self::assertSame('recovery_required', $response['phase']);
        self::assertSame('unknown', $response['enrollment_status']);
        self::assertFalse($response['retry_safe']);
    }

    public function test_it_accepts_a_rolled_back_failure_outcome(): void
    {
        $provisioner = new PrivilegedProvisioner();

        $response = $provisioner->decodeResponse(json_encode([
            'protocol_version' => 1,
            'success' => false,
            'phase' => 'rolled_back',
            'enrollment_status' => 'not_started',
            'retry_safe' => true,
            'agent_version' => '0.7.5',
            'application_count' => 2,
            'message' => 'Transaction-created files and directories were rolled back.',
        ], JSON_THROW_ON_ERROR));

        self::assertFalse($response['success']);
        self::assertSame('rolled_back', $response['phase']);
        self::assertSame('not_started', $response['enrollment_status']);
        self::assertTrue($response['retry_safe']);
    }

    public function test_it_rejects_a_retry_safe_outcome_after_enrollment_started(): void
    {
        $provisioner = new PrivilegedProvisioner();

        $this->expectException(RuntimeException::class);
        $this->expectExceptionMessage('outcome consistency check failed');

        $provisioner->decodeResponse(json_encode([
            'protocol_version' => 1,
            'success' => false,
            'phase' => 'enrollment',
            'enrollment_status' => 'unknown',
            'retry_safe' => true,
            'agent_version' => 'unknown',
            'application_count' => 1,
            'message' => 'Outcome uncertain.',
        ], JSON_THROW_ON_ERROR));
    }

    public function test_it_rejects_unknown_response_fields(): void
    {
        $provisioner = new PrivilegedProvisioner();

        $this->expectException(RuntimeException::class);
        $this->expectExceptionMessage('response fields are invalid');

        $provisioner->decodeResponse(json_encode([
            'protocol_version' => 1,
            'success' => true,
            'phase' => 'completed',
            'enrollment_status' => 'registered',
            'retry_safe' => false,
            'agent_version' => '0.7.5',
            'application_count' => 1,
            'message' => 'NADI Agent enrollment and service startup completed.',
            'agent_credential' => 'must-not-be-returned',
        ], JSON_THROW_ON_ERROR));
    }

    public function test_it_rejects_a_response_that_does_not_confirm_registration(): void
    {
        $provisioner = new PrivilegedProvisioner();

        $this->expectException(RuntimeException::class);
        $this->expectExceptionMessage('outcome consistency check failed');

        $provisioner->decodeResponse(json_encode([
            'protocol_version' => 1,
            'success' => true,
            'phase' => 'completed',
            'enrollment_status' => 'unknown',
            'retry_safe' => false,
            'agent_version' => '0.7.5',
            'application_count' => 1,
            'message' => 'Invalid completion status.',
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
        $this->expectExceptionMessage('outcome consistency check failed');

        $provisioner->decodeResponse(json_encode([
            'protocol_version' => 1,
            'success' => true,
            'phase' => 'completed',
            'enrollment_status' => 'registered',
            'retry_safe' => false,
            'agent_version' => '0.7.5',
            'application_count' => 0,
            'message' => 'Invalid application count.',
        ], JSON_THROW_ON_ERROR));
    }

    public function test_it_accepts_a_successful_install_only_response(): void
    {
        $provisioner = new PrivilegedProvisioner();

        $response = $provisioner->decodeResponse(json_encode([
            'protocol_version' => 1,
            'success' => true,
            'phase' => 'installed',
            'enrollment_status' => 'not_started',
            'retry_safe' => true,
            'agent_version' => '0.7.5',
            'application_count' => 2,
            'message' => 'NADI Agent assets installed; enrollment has not started.',
        ], JSON_THROW_ON_ERROR));

        self::assertSame('installed', $response['phase']);
        self::assertSame('not_started', $response['enrollment_status']);
        self::assertTrue($response['retry_safe']);
    }

    public function test_it_accepts_a_successful_enrollment_without_service_startup(): void
    {
        $provisioner = new PrivilegedProvisioner();

        $response = $provisioner->decodeResponse(json_encode([
            'protocol_version' => 1,
            'success' => true,
            'phase' => 'enrolled',
            'enrollment_status' => 'registered',
            'retry_safe' => false,
            'agent_version' => '0.7.5',
            'application_count' => 2,
            'message' => 'NADI Agent enrollment and broker configuration completed.',
        ], JSON_THROW_ON_ERROR));

        self::assertSame('enrolled', $response['phase']);
        self::assertSame('registered', $response['enrollment_status']);
        self::assertFalse($response['retry_safe']);
    }

    public function test_it_rejects_an_install_response_that_claims_enrollment_started(): void
    {
        $provisioner = new PrivilegedProvisioner();

        $this->expectException(RuntimeException::class);
        $this->expectExceptionMessage('outcome consistency check failed');

        $provisioner->decodeResponse(json_encode([
            'protocol_version' => 1,
            'success' => true,
            'phase' => 'installed',
            'enrollment_status' => 'registered',
            'retry_safe' => false,
            'agent_version' => '0.7.5',
            'application_count' => 1,
            'message' => 'Invalid install phase.',
        ], JSON_THROW_ON_ERROR));
    }
}
