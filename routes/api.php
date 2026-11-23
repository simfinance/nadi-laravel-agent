<?php

use Illuminate\Support\Facades\Route;
use Nadi\LaravelAgent\Http\Controllers\BridgeController;
use Nadi\LaravelAgent\Http\Middleware\AuthorizeNadiRequest;
use Nadi\LaravelAgent\Http\Middleware\AuthorizeNadiActivation;

$prefix = trim((string) config('nadi-agent.route.prefix', '_nadi/v1'), '/');
$middleware = array_merge(
    (array) config('nadi-agent.route.middleware', ['api']),
    [AuthorizeNadiRequest::class]
);

if (config('nadi-agent.route.throttle')) {
    $middleware[] = 'throttle:'.config('nadi-agent.route.throttle');
}

Route::prefix($prefix)
    ->middleware($middleware)
    ->as('nadi-agent.')
    ->group(function (): void {
        Route::get('/health', [BridgeController::class, 'health'])
            ->middleware(AuthorizeNadiActivation::class . ':health')
            ->name('health');
        Route::get('/metrics', [BridgeController::class, 'metrics'])
            ->middleware(AuthorizeNadiActivation::class . ':metrics')
            ->name('metrics');
        Route::get('/database', [BridgeController::class, 'database'])
            ->middleware(AuthorizeNadiActivation::class . ':database')
            ->name('database');
        Route::get('/queue', [BridgeController::class, 'queue'])
            ->middleware(AuthorizeNadiActivation::class . ':queue')
            ->name('queue');
        Route::get('/scheduler', [BridgeController::class, 'scheduler'])
            ->middleware(AuthorizeNadiActivation::class . ':scheduler')
            ->name('scheduler');
        Route::get('/info', [BridgeController::class, 'info'])
            ->middleware(AuthorizeNadiActivation::class . ':info')
            ->name('info');
        Route::get('/logs/application', [BridgeController::class, 'applicationLogs'])
            ->middleware(AuthorizeNadiActivation::class . ':application_logs')
            ->name('logs.application');
    });
