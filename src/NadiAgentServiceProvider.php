<?php

namespace Nadi\LaravelAgent;

use Illuminate\Support\ServiceProvider;
use Nadi\LaravelAgent\Console\InstallCommand;

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
        $this->loadRoutesFrom(__DIR__ . '/../routes/api.php');

        if ($this->app->runningInConsole()) {
            $this->commands([
                InstallCommand::class,
            ]);
        }
    }
}