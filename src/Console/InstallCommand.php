<?php

namespace Nadi\LaravelAgent\Console;

use Illuminate\Console\Command;
use Nadi\LaravelAgent\Installer\EnrollmentManifestBuilder;
use Nadi\LaravelAgent\Installer\Preflight;
use Nadi\LaravelAgent\Installer\PrivilegedProvisioner;
use Nadi\LaravelAgent\Installer\ReleaseArtifactDownloader;
use Nadi\LaravelAgent\Installer\ReleaseManifestClient;
use RuntimeException;
use Throwable;

class InstallCommand extends Command
{
    protected $signature = 'nadi:install';

    protected $description = 'Install and configure NADI Laravel Bridge and server Agent.';

    public function handle(
        Preflight $preflight,
        ReleaseManifestClient $releaseManifestClient,
        ReleaseArtifactDownloader $artifactDownloader,
        EnrollmentManifestBuilder $manifestBuilder,
        PrivilegedProvisioner $provisioner
    ): int {
        $this->info('NADI Agent installer');

        try {
            $applicationManifest = $manifestBuilder->build();
            $applicationTargets = $manifestBuilder->buildTargets();

            $result = $preflight->inspect(
                base_path(),
                $applicationTargets
            );
            $preflight->assertSupported($result);
        } catch (Throwable $exception) {
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
            $signedRelease = $releaseManifestClient->fetchSigned();
            $manifest = $signedRelease['manifest'];
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
        } catch (Throwable $exception) {
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

        try {
            $provisioner->assertAvailable();
        } catch (Throwable $exception) {
            $this->error($exception->getMessage());
            $this->warn('The verified Agent binary remains in private staging. No privileged command or enrollment was executed.');

            return self::FAILURE;
        }

        $enrollmentToken = $this->secret('Enter the one-time NADI enrollment token');

        if (! is_string($enrollmentToken) || trim($enrollmentToken) === '') {
            $this->error('Enrollment token is required.');
            $this->warn('No privileged command or enrollment was executed.');

            return self::FAILURE;
        }

        try {
            $provisioned = $provisioner->installAgent(
                $signedRelease,
                $binaryPath,
                $applicationManifest,
                $applicationTargets,
                $result['php_fpm_identities'],
                trim($enrollmentToken)
            );
        } catch (Throwable $exception) {
            $this->error($exception->getMessage());
            $this->warn('Do not enable monitoring manually. Verify the provisioning helper and rollback state before retrying.');

            return self::FAILURE;
        } finally {
            if (function_exists('sodium_memzero')) {
                sodium_memzero($enrollmentToken);
            }
            unset($enrollmentToken);
        }

        $this->line('<fg=green>PASS</> privileged_agent_provisioning');
        $this->line('<fg=green>PASS</> agent_enrollment');
        $this->info(sprintf(
            'NADI Agent %s enrolled with %d application(s). Control Poll is enabled; monitoring remains subject to Central approval and a valid acknowledged Activation Grant.',
            $provisioned['agent_version'],
            $provisioned['application_count']
        ));

        return self::SUCCESS;
    }
}
