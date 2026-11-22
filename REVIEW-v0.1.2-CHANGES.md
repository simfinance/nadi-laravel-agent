# Review bundle v0.1.2

## Changes

- Fixed the Composer `test` script to pass the `tests` directory to PHPUnit.
- Kept the existing test fixture correction that uses a temporary application path.

## Validation status

- PHP syntax checks passed in the previous v0.1.1 review run.
- Agent binary checksums and architecture checks passed in the previous v0.1.1 review run.
- PHPUnit was run directly on the v0.1.1 candidate: 2 tests, 4 assertions, all passing.
- The updated Composer test script is configured to run the same test directory and TestDox output.

This remains an internal review candidate, not a production-ready installer. Release signature verification, complete Agent provisioning, enrollment/credential handoff, and Bridge monitoring endpoints still require implementation and validation.
