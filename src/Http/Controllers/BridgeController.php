<?php

namespace Nadi\LaravelAgent\Http\Controllers;

use Illuminate\Http\JsonResponse;
use Illuminate\Routing\Controller;
use Illuminate\Support\Facades\App;
use Nadi\LaravelAgent\Monitoring\DatabaseProbe;
use Nadi\LaravelAgent\Monitoring\ApplicationLogReader;

class BridgeController extends Controller
{
    public function health(): JsonResponse
    {
        return $this->success([
            'status' => 'ok',
            'timestamp' => now()->toISOString(),
        ]);
    }

    public function metrics(): JsonResponse
    {
        return $this->notImplemented('application_metrics');
    }

    public function database(DatabaseProbe $databaseProbe): JsonResponse
    {
        $result = $databaseProbe->check();
        $statusCode = $result['status'] === 'ok' ? 200 : 503;

        return response()->json([
            'success' => $result['status'] === 'ok',
            'data' => $result,
        ], $statusCode);
    }

    public function queue(): JsonResponse
    {
        return $this->notImplemented('queue_monitoring');
    }

    public function scheduler(): JsonResponse
    {
        return $this->notImplemented('scheduler_monitoring');
    }

    public function applicationLogs(ApplicationLogReader $logReader): JsonResponse
    {
        return $this->success($logReader->collect());
    }

    public function info(): JsonResponse
    {
        return $this->success([
            'application_name' => config('nadi-agent.info.application_name'),
            'environment' => config('nadi-agent.info.environment'),
            'laravel_version' => App::version(),
            'php_version' => PHP_VERSION,
        ]);
    }

    private function success(array $data): JsonResponse
    {
        return response()->json([
            'success' => true,
            'data' => $data,
        ]);
    }

    private function notImplemented(string $capability): JsonResponse
    {
        return response()->json([
            'success' => false,
            'error' => [
                'code' => 'capability_not_implemented',
                'message' => 'This monitoring capability is not available in this bridge version.',
                'capability' => $capability,
            ],
        ], 501);
    }
}
