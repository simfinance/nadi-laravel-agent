<?php

namespace Nadi\LaravelAgent\Http\Middleware;

use Closure;
use Illuminate\Http\JsonResponse;
use Illuminate\Http\Request;
use Symfony\Component\HttpFoundation\Response;

class AuthorizeNadiRequest
{
    private const MAXIMUM_CREDENTIAL_FILE_SIZE = 4096;

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

        if (
            ! is_string($credentialPath)
            || $credentialPath === ''
            || ! str_starts_with($credentialPath, DIRECTORY_SEPARATOR)
        ) {
            return null;
        }

        clearstatcache(true, $credentialPath);

        if (is_link($credentialPath)) {
            return null;
        }

        $pathStat = @lstat($credentialPath);

        if (
            ! is_array($pathStat)
            || (($pathStat['mode'] & 0170000) !== 0100000)
            || (($pathStat['mode'] & 0777) !== 0600)
            || $pathStat['size'] < 1
            || $pathStat['size'] > self::MAXIMUM_CREDENTIAL_FILE_SIZE
        ) {
            return null;
        }

        if (
            function_exists('posix_geteuid')
            && $pathStat['uid'] !== posix_geteuid()
        ) {
            return null;
        }

        $handle = @fopen($credentialPath, 'rb');

        if ($handle === false) {
            return null;
        }

        try {
            $handleStat = fstat($handle);

            if (
                ! is_array($handleStat)
                || $handleStat['dev'] !== $pathStat['dev']
                || $handleStat['ino'] !== $pathStat['ino']
                || (($handleStat['mode'] & 0170000) !== 0100000)
                || (($handleStat['mode'] & 0777) !== 0600)
                || $handleStat['size'] < 1
                || $handleStat['size'] > self::MAXIMUM_CREDENTIAL_FILE_SIZE
                || (
                    function_exists('posix_geteuid')
                    && $handleStat['uid'] !== posix_geteuid()
                )
            ) {
                return null;
            }

            $credential = stream_get_contents(
                $handle,
                self::MAXIMUM_CREDENTIAL_FILE_SIZE + 1
            );

            if (
                ! is_string($credential)
                || $credential === ''
                || strlen($credential) > self::MAXIMUM_CREDENTIAL_FILE_SIZE
            ) {
                return null;
            }
        } finally {
            fclose($handle);
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

        if (! preg_match('/^Bearer\\s+(.+)$/i', trim($authorization), $matches)) {
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
