<?php

namespace Nadi\LaravelAgent\Installer;

use RuntimeException;
use Symfony\Component\Process\Process;

class AgentEnrollmentRunner
{
    public function enroll(
        string $binaryPath,
        string $manifestPath,
        string $stateDirectory,
        string $activationPublicKeyPath,
        string $activationKeyId,
        string $enrollmentToken
    ): string {
        foreach ([
            'Agent binary' => $binaryPath,
            'Enrollment manifest' => $manifestPath,
            'Activation public key' => $activationPublicKeyPath,
        ] as $label => $path) {
            if (! is_file($path) || ! is_readable($path)) {
                throw new RuntimeException($label . ' is missing or unreadable.');
            }
        }

        if (! is_executable($binaryPath)) {
            throw new RuntimeException('Agent binary is not executable.');
        }

        if (trim($enrollmentToken) === '') {
            throw new RuntimeException('Enrollment token is required.');
        }

        if (! is_dir($stateDirectory) || ! is_writable($stateDirectory)) {
            throw new RuntimeException('Agent state directory is not writable by the Agent runtime user.');
        }

        $environment = [
            'NADI_STATE_DIR' => $stateDirectory,
            'NADI_ACTIVATION_PUBLIC_KEY' => $activationPublicKeyPath,
            'NADI_ACTIVATION_KEY_ID' => $activationKeyId,
            'NADI_CENTRAL_URL' => rtrim((string) config('nadi-agent.central_url'), '/'),
            'NADI_ENROLLMENT_TOKEN' => null,
            'NADI_AGENT_TOKEN' => null,
        ];

        $process = new Process([
            $binaryPath,
            '--enroll',
            '--manifest',
            $manifestPath,
            '--enrollment-token-stdin',
        ]);

        $process->setEnv($environment);
        $process->setInput($enrollmentToken . "\n");
        $process->setTimeout(120);
        $process->run();

        if (! $process->isSuccessful()) {
            $errorOutput = trim($process->getErrorOutput());

            if ($errorOutput === '') {
                $errorOutput = 'Agent enrollment process failed without diagnostic output.';
            }

            throw new RuntimeException($this->sanitizeOutput($errorOutput));
        }

        return trim($process->getOutput());
    }

    private function sanitizeOutput(string $output): string
    {
        $output = preg_replace(
            '/(enrollment_token["\']?\s*[:=]\s*)[^\s,"]+/i',
            '$1[REDACTED]',
            $output
        );

        return substr((string) $output, 0, 2000);
    }
}
