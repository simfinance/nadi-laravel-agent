<?php

namespace Nadi\LaravelAgent\Console;

use Illuminate\Console\Command;
use Nadi\LaravelAgent\Installer\Preflight;
use Nadi\LaravelAgent\Installer\ReleaseArtifactDownloader;
use Nadi\LaravelAgent\Installer\ReleaseManifestClient;
use RuntimeException;

class InstallCommand extends Command
{
    protected $signature = 'nadi:install';

    protected $description = 'Install and configure NADI Laravel Bridge and server Agent.';

    public function handle(
        Preflight $preflight,
        ReleaseManifestClient $releaseManifestClient,
        ReleaseArtifactDownloader $artifactDownloader
    ): int {
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

        try {
            $manifest = $releaseManifestClient->fetch();
            $artifact = $releaseManifestClient->selectArtifact(
                $manifest,
                $result['architecture']
            );

            $stagingRoot = storage_path('app/private/nadi/releases/staging');
            $stagingDirectory = $stagingRoot . DIRECTORY_SEPARATOR . $manifest['version'];

            if (is_link($stagingRoot) || is_link($stagingDirectory)) {
                throw new RuntimeException('Agent release staging path must not contain a symbolic link.');
            }

            if (! is_dir($stagingDirectory) && ! mkdir($stagingDirectory, 0700, true) && ! is_dir($stagingDirectory)) {
                throw new RuntimeException('Unable to create the private Agent release staging directory.');
            }

            $storageRoot = realpath(storage_path('app'));
            $resolvedStagingDirectory = realpath($stagingDirectory);

            if (
                $storageRoot === false
                || $resolvedStagingDirectory === false
                || ! str_starts_with($resolvedStagingDirectory, $storageRoot . DIRECTORY_SEPARATOR)
            ) {
                throw new RuntimeException('Agent release staging directory is outside private application storage.');
            }

            if (! chmod($stagingDirectory, 0700)) {
                throw new RuntimeException('Unable to secure the private Agent release staging directory permissions.');
            }

            $binaryPath = $artifactDownloader->download(
                $artifact,
                $stagingDirectory
            );
        } catch (RuntimeException $exception) {
            $this->error($exception->getMessage());
            $this->warn('No Agent binary was executed, no enrollment was submitted, and no monitoring was started.');

            return self::FAILURE;
        }

        $this->line('<fg=green>PASS</> release_manifest_signature');
        $this->line('<fg=green>PASS</> agent_artifact_sha256');
        $this->info(sprintf(
            'Agent %s verified and staged at %s.',
            $manifest['version'],
            $binaryPath
        ));

        $this->error(
            'The privileged provisioning adapter and secure enrollment handoff are not yet connected.'
        );
        $this->warn(
            'The verified binary remains non-executable in private staging. No enrollment or monitoring was started.'
        );

        return self::FAILURE;
    }
}
