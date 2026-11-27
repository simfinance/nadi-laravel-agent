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
            || ! function_exists('posix_geteuid')
            || ! $this->hasSafePathComponents($credentialPath)
        ) {
            return null;
        }

        clearstatcache(true, $credentialPath);
        $pathMetadata = @lstat($credentialPath);

        if (
            ! is_array($pathMetadata)
            || (($pathMetadata['mode'] & 0170000) !== 0100000)
            || $pathMetadata['uid'] !== posix_geteuid()
            || (($pathMetadata['mode'] & 07777) !== 0600)
            || $pathMetadata['nlink'] !== 1
            || $pathMetadata['size'] < 1
            || $pathMetadata['size'] > self::MAXIMUM_CREDENTIAL_FILE_SIZE
        ) {
            return null;
        }

        $handle = @fopen($credentialPath, 'rb');

        if ($handle === false) {
            return null;
        }

        try {
            $handleMetadata = fstat($handle);

            if (
                ! is_array($handleMetadata)
                || (($handleMetadata['mode'] & 0170000) !== 0100000)
                || $handleMetadata['uid'] !== posix_geteuid()
                || (($handleMetadata['mode'] & 07777) !== 0600)
                || $handleMetadata['nlink'] !== 1
                || $handleMetadata['dev'] !== $pathMetadata['dev']
                || $handleMetadata['ino'] !== $pathMetadata['ino']
                || $handleMetadata['size'] < 1
                || $handleMetadata['size'] > self::MAXIMUM_CREDENTIAL_FILE_SIZE
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

            return $credential;
        } finally {
            fclose($handle);
        }
    }

    private function hasSafePathComponents(string $path): bool
    {
        $segments = array_values(array_filter(
            explode(DIRECTORY_SEPARATOR, $path),
            static fn (string $segment): bool => $segment !== ''
        ));

        $currentPath = DIRECTORY_SEPARATOR;

        foreach ($segments as $index => $segment) {
            $currentPath = rtrim($currentPath, DIRECTORY_SEPARATOR)
                . DIRECTORY_SEPARATOR
                . $segment;

            clearstatcache(true, $currentPath);
            $metadata = @lstat($currentPath);

            if (! is_array($metadata) || (($metadata['mode'] & 0170000) === 0120000)) {
                return false;
            }

            $isFinalComponent = $index === count($segments) - 1;

            if ($isFinalComponent) {
                if (($metadata['mode'] & 0170000) !== 0100000) {
                    return false;
                }
            } elseif (($metadata['mode'] & 0170000) !== 0040000) {
                return false;
            }
        }

        return $segments !== [];
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
