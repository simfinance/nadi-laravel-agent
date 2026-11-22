<?php

namespace Nadi\LaravelAgent\Console;

use Illuminate\Console\Command;
use Nadi\LaravelAgent\Installer\Preflight;
use RuntimeException;

class InstallCommand extends Command
{
    protected $signature = 'nadi:install';

    protected $description = 'Install and configure NADI Laravel Bridge and server Agent.';

    public function handle(Preflight $preflight): int
    {
        $this->info('NADI Agent installer');

        try {
            $result = $preflight->inspect(base_path());
            $preflight->assertSupported($result);
        } catch (RuntimeException $exception) {
            $this->error($exception->getMessage());

            return self::FAILURE;
        }

        foreach ($result['checks'] as $name => $passed) {
            $this->line(sprintf(
                '%s %s',
                $passed ? '<fg=green>PASS</>' : '<fg=red>FAIL</>',
                $name
            ));
        }

        $releasePublicKey = (string) config('nadi-agent.release_public_key');

        if ($releasePublicKey === '') {
            $this->error(
                'The trusted Agent release public key is not embedded in this package release.'
            );
            $this->warn(
                'No enrollment token was requested and no system files were changed.'
            );

            return self::FAILURE;
        }

        $this->error(
            'The signed release downloader and privileged provisioning adapter are not yet connected.'
        );
        $this->warn(
            'No binary was executed, no enrollment was submitted, and no monitoring was started.'
        );

        return self::FAILURE;
    }
}
