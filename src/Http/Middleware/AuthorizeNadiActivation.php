<?php

namespace Nadi\LaravelAgent\Http\Middleware;

use Closure;
use Illuminate\Http\Request;
use Nadi\LaravelAgent\Monitoring\ActivationStateReader;
use Symfony\Component\HttpFoundation\Response;

class AuthorizeNadiActivation
{
    public function handle(
        Request $request,
        Closure $next,
        string $capability
    ): Response {
        if (! app(ActivationStateReader::class)->allows($capability)) {
            return response()->json([
                'success' => false,
                'error' => [
                    'code' => 'monitoring_not_active',
                    'message' => 'NADI monitoring is not active for this application.',
                ],
            ], 403);
        }

        return $next($request);
    }
}
