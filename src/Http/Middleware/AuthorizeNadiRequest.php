<?php

namespace Nadi\LaravelAgent\Http\Middleware;

use Closure;
use Illuminate\Http\JsonResponse;
use Illuminate\Http\Request;
use Symfony\Component\HttpFoundation\Response;

class AuthorizeNadiRequest
{
    public function handle(Request $request, Closure $next): Response
    {
        if (! config('nadi-agent.enabled', false)) {
            return $this->error('bridge_disabled', 'NADI bridge is disabled.', 503);
        }

        if (! $this->isAllowedSourceIp($request)) {
            return $this->error('source_ip_not_allowed', 'Source IP is not allowed.', 403);
        }

        $configuredCredential = $this->readApplicationCredential();
        $providedCredential = $this->bearerToken($request);

        if (
            ! is_string($configuredCredential)
            || $configuredCredential === ''
            || ! is_string($providedCredential)
            || $providedCredential === ''
            || ! hash_equals($configuredCredential, $providedCredential)
        ) {
            return $this->error('invalid_application_credential', 'Application credential is invalid.', 401);
        }

        return $next($request);
    }

    private function readApplicationCredential(): ?string
    {
        $credentialPath = config('nadi-agent.credential_file');

        if (! is_string($credentialPath) || $credentialPath === '' || ! is_file($credentialPath)) {
            return null;
        }

        if (! is_readable($credentialPath)) {
            return null;
        }

        $credential = file_get_contents($credentialPath);

        if (! is_string($credential)) {
            return null;
        }

        $credential = trim($credential);

        return $credential !== '' ? $credential : null;
    }

    private function isAllowedSourceIp(Request $request): bool
    {
        $allowedIps = config('nadi-agent.allowed_ips', []);

        if (! is_array($allowedIps) || $allowedIps === []) {
            return false;
        }

        $sourceIp = $request->ip();

        if (! is_string($sourceIp) || $sourceIp === '') {
            return false;
        }

        foreach ($allowedIps as $allowedIp) {
            if (is_string($allowedIp) && hash_equals($allowedIp, $sourceIp)) {
                return true;
            }
        }

        return false;
    }

    private function bearerToken(Request $request): ?string
    {
        $authorization = $request->header('Authorization');

        if (! is_string($authorization)) {
            return null;
        }

        if (! preg_match('/^Bearer\s+(.+)$/i', trim($authorization), $matches)) {
            return null;
        }

        return trim($matches[1]);
    }

    private function error(string $code, string $message, int $status): JsonResponse
    {
        return response()->json([
            'success' => false,
            'error' => [
                'code' => $code,
                'message' => $message,
            ],
        ], $status);
    }
}
