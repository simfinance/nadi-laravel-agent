<?php

namespace Nadi\LaravelAgent;

use Illuminate\Support\ServiceProvider;
use Nadi\LaravelAgent\Console\InstallCommand;
use Nadi\LaravelAgent\Installer\BridgeApplicationConfiguration;
use Throwable;

class NadiAgentServiceProvider extends ServiceProvider
{
    public function register(): void
    {
        $this->mergeConfigFrom(
            __DIR__ . '/../config/nadi-agent.php',
            'nadi-agent'
        );
    }

    public function boot(): void
    {
        $this->loadManagedApplicationConfiguration();

        $this->loadRoutesFrom(__DIR__ . '/../routes/api.php');

        if ($this->app->runningInConsole()) {
            $this->commands([
                InstallCommand::class,
            ]);
        }
    }

    private function loadManagedApplicationConfiguration(): void
    {
        config([
            'nadi-agent.enabled' => false,
            'nadi-agent.allowed_ips' => [],
            'nadi-agent.credential_file' => null,
            'nadi-agent.activation_state_file' => null,
            'nadi-agent.activation_application_id' => null,
            'nadi-agent.activation_application_key' => null,
        ]);

        try {
            $configuration = $this->app
                ->make(BridgeApplicationConfiguration::class)
                ->load(base_path());

            if ($configuration !== null) {
                $values = [];

                foreach ($configuration as $key => $value) {
                    $values['nadi-agent.' . $key] = $value;
                }

                config($values);
            }
        } catch (Throwable) {
            // Keep the Bridge disabled when managed configuration is unavailable.
        }
    }
}
