<?php

namespace Nadi\LaravelAgent\Monitoring;

use Illuminate\Support\Facades\Cache;
use Illuminate\Support\Facades\DB;
use Throwable;

class DatabaseProbe
{
    private const CACHE_KEY = 'nadi-agent:database-probe:v1';

    public function check(): array
    {
        $cacheSeconds = max(
            0,
            (int) config('nadi-agent.database.cache_seconds', 10)
        );

        if ($cacheSeconds > 0)
        {
            try
            {
                $cached = Cache::get(self::CACHE_KEY);

                if (is_array($cached))
                {
                    $cached['cache_hit'] = true;

                    return $cached;
                }
            }
            catch (Throwable)
            {
                // A cache outage must not prevent the database probe.
            }
        }

        $startedAt = microtime(true);

        try
        {
            $connection = DB::connection();
            $driver = $connection->getDriverName();

            if ($driver === 'mysql')
            {
                $connection->selectOne(
                    'SELECT /*+ MAX_EXECUTION_TIME(5000) */ 1 AS nadi_probe'
                );
            }
            elseif ($driver === 'pgsql')
            {
                $connection->transaction(function () use ($connection): void
                {
                    $connection->statement(
                        "SET LOCAL statement_timeout = '5000ms'"
                    );
                    $connection->selectOne('SELECT 1 AS nadi_probe');
                });
            }
            elseif ($driver === 'sqlite')
            {
                $connection->selectOne('SELECT 1 AS nadi_probe');
            }
            else
            {
                return $this->failure(
                    'unsupported_database_driver',
                    $startedAt
                );
            }

            $result = [
                'status' => 'ok',
                'driver' => $driver,
                'latency_ms' => $this->elapsedMilliseconds($startedAt),
                'checked_at' => now()->toISOString(),
                'cache_hit' => false,
            ];

            if ($cacheSeconds > 0)
            {
                try
                {
                    Cache::put(self::CACHE_KEY, $result, $cacheSeconds);
                }
                catch (Throwable)
                {
                    // Cache persistence is best-effort; the probe result remains valid.
                }
            }

            return $result;
        }
        catch (Throwable)
        {
            return $this->failure(
                'database_connection_failed',
                $startedAt
            );
        }
    }

    private function failure(string $code, float $startedAt): array
    {
        return [
            'status' => 'failed',
            'error_code' => $code,
            'latency_ms' => $this->elapsedMilliseconds($startedAt),
            'checked_at' => now()->toISOString(),
            'cache_hit' => false,
        ];
    }

    private function elapsedMilliseconds(float $startedAt): float
    {
        return round((microtime(true) - $startedAt) * 1000, 2);
    }
}
