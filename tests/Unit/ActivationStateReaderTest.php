<?php

namespace Nadi\LaravelAgent\Tests\Unit;

use Nadi\LaravelAgent\Monitoring\ActivationStateReader;
use Orchestra\Testbench\TestCase;
use ReflectionMethod;

class ActivationStateReaderTest extends TestCase
{
    public function test_it_denies_when_activation_assertion_is_not_configured(): void
    {
        config([
            'nadi-agent.activation_state_file' => null,
            'nadi-agent.activation_application_id' => null,
            'nadi-agent.activation_application_key' => null,
        ]);

        self::assertFalse(
            (new ActivationStateReader())->allows('health')
        );
    }

    public function test_it_rejects_noncanonical_activation_state_paths(): void
    {
        $reader = new ActivationStateReader();
        $method = new ReflectionMethod($reader, 'assertSafePath');

        $this->expectException(\RuntimeException::class);

        $method->invoke(
            $reader,
            '/run/nadi/bridge-state/../bridge-state/application.json'
        );
    }

    public function test_it_denies_unknown_capabilities(): void
    {
        self::assertFalse(
            (new ActivationStateReader())->allows('arbitrary')
        );
    }

    public function test_it_accepts_a_fresh_active_assertion_for_the_matching_capability(): void
    {
        self::assertTrue(
            $this->validateState($this->activeState(), 'health')
        );
    }

    public function test_it_denies_unexpected_state_fields(): void
    {
        $state = $this->activeState();
        $state['extra'] = 'not-allowed';

        self::assertFalse(
            $this->validateState($state, 'health')
        );
    }

    public function test_it_denies_duplicate_capabilities(): void
    {
        $state = $this->activeState();
        $state['capabilities'] = ['health', 'health'];

        self::assertFalse(
            $this->validateState($state, 'health')
        );
    }

    public function test_it_denies_a_state_for_another_application(): void
    {
        $state = $this->activeState();
        $state['application_id'] = 'another-app';

        self::assertFalse(
            $this->validateState($state, 'health')
        );
    }

    public function test_it_denies_a_state_without_grant_acknowledgement(): void
    {
        $state = $this->activeState();
        $state['grant_acknowledged'] = false;

        self::assertFalse(
            $this->validateState($state, 'health')
        );
    }

    public function test_it_denies_a_state_with_an_expired_assertion(): void
    {
        $state = $this->activeState();
        $state['observed_at'] = gmdate(DATE_ATOM, time() - 600);
        $state['valid_until'] = gmdate(DATE_ATOM, time() - 300);

        self::assertFalse(
            $this->validateState($state, 'health')
        );
    }

    public function test_application_metrics_grant_allows_health_metrics_and_info(): void
    {
        $state = $this->activeState();
        $state['capabilities'] = ['application_metrics'];

        self::assertTrue($this->validateState($state, 'health'));
        self::assertTrue($this->validateState($state, 'metrics'));
        self::assertTrue($this->validateState($state, 'info'));
    }

    public function test_database_queue_scheduler_and_logs_map_only_to_their_own_endpoints(): void
    {
        $state = $this->activeState();
        $state['capabilities'] = [
            'database_monitoring',
            'queue_monitoring',
            'scheduler_monitoring',
            'application_logs',
        ];

        self::assertTrue($this->validateState($state, 'database'));
        self::assertTrue($this->validateState($state, 'queue'));
        self::assertTrue($this->validateState($state, 'scheduler'));
        self::assertTrue($this->validateState($state, 'application_logs'));
        self::assertFalse($this->validateState($state, 'health'));
        self::assertFalse($this->validateState($state, 'metrics'));
        self::assertFalse($this->validateState($state, 'info'));
    }

    public function test_system_logs_capability_does_not_block_application_metrics_access(): void
    {
        $state = $this->activeState();
        $state['capabilities'] = ['application_metrics', 'system_logs'];

        self::assertTrue($this->validateState($state, 'health'));
        self::assertTrue($this->validateState($state, 'metrics'));
        self::assertFalse($this->validateState($state, 'database'));
    }

    public function test_system_logs_capability_does_not_authorize_a_bridge_endpoint(): void
    {
        $state = $this->activeState();
        $state['capabilities'] = ['system_logs'];

        self::assertFalse($this->validateState($state, 'health'));
        self::assertFalse($this->validateState($state, 'metrics'));
        self::assertFalse($this->validateState($state, 'database'));
    }

    public function test_server_metrics_grant_does_not_authorize_a_bridge_endpoint(): void
    {
        $state = $this->activeState();
        $state['capabilities'] = ['server_metrics'];

        self::assertFalse($this->validateState($state, 'health'));
        self::assertFalse($this->validateState($state, 'database'));
    }

    public function test_it_denies_unknown_central_grant_capabilities(): void
    {
        $state = $this->activeState();
        $state['capabilities'] = ['application_metrics', 'unknown_capability'];

        self::assertFalse($this->validateState($state, 'health'));
    }

    private function validateState(array $state, string $capability): bool
    {
        $reader = new ActivationStateReader();
        $method = new ReflectionMethod($reader, 'isValidState');
        $method->setAccessible(true);

        return $method->invoke(
            $reader,
            $state,
            'central-app-123',
            'example-production-abc123',
            $capability
        );
    }

    private function activeState(): array
    {
        $now = time();

        return [
            'protocol_version' => 1,
            'application_id' => 'central-app-123',
            'application_key' => 'example-production-abc123',
            'activation_state' => 'active',
            'monitoring_enabled' => true,
            'grant_acknowledged' => true,
            'capabilities' => ['application_metrics', 'database_monitoring'],
            'observed_at' => gmdate(DATE_ATOM, $now),
            'valid_until' => gmdate(DATE_ATOM, $now + 120),
        ];
    }
}
