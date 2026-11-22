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

    public function test_it_denies_a_state_when_the_requested_capability_is_missing(): void
    {
        $state = $this->activeState();
        $state['capabilities'] = ['database'];

        self::assertFalse(
            $this->validateState($state, 'health')
        );
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
            'capabilities' => ['health', 'database'],
            'observed_at' => gmdate(DATE_ATOM, $now),
            'valid_until' => gmdate(DATE_ATOM, $now + 120),
        ];
    }
}
