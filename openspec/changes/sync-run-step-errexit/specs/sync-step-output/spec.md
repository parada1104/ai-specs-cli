# Specification: sync-step-output

Requirements for the errexit contract of `run_step` in `lib/sync.sh` and
`lib/sync-agent.sh`.

## MODIFIED Requirements

### Requirement: `run_step` restores errexit only after its own cleanup

`run_step` disables errexit while running the wrapped command so it can capture
the exit status, then restores it. The restore SHALL happen after the helper has
finished printing captured output and removing its temporary files — not
immediately after capturing the status.

Restoring earlier means a failure inside the helper's own output handling (a
`cat` hitting SIGPIPE on an early-closed stdout, or a full disk) aborts the
script from inside the helper: the temporary files leak, the caller's error
handling never runs, and bash's status is returned instead of the wrapped
command's.

`run_step` SHALL NOT leave errexit disabled on return. `set` options are
shell-global rather than function-local, so a helper that disables errexit and
does not restore it silently disables it for the remainder of the script.

#### Scenario: errexit is active after a successful step
- **WHEN** `run_step` returns from a command that exited 0
- **THEN** errexit is enabled
- **AND** a subsequent failing command aborts the script

#### Scenario: errexit is active after a failed step
- **WHEN** `run_step` returns from a command that exited non-zero, in a context
  where the caller handles the failure
- **THEN** errexit is enabled for the statements that follow

#### Scenario: a bare failing step still aborts
- **WHEN** `run_step` is invoked without `if !` or `||` and its command fails
- **THEN** the script aborts
- **AND** the wrapped command's exit status is preserved

#### Scenario: a guarded failing step yields the real status
- **WHEN** `run_step` is invoked as `if ! run_step …` and its command exits 42
- **THEN** the caller observes 42

#### Scenario: temporary files do not survive a step
- **WHEN** `run_step` returns, for either a successful or a failing command
- **THEN** neither of its temporary files remains on disk

### Requirement: a temporary-file failure names itself

`run_step` SHALL detect a `mktemp` failure and report it as such rather than
letting it surface as whatever abort message the wrapped command's caller
happens to produce. The step SHALL still run.

#### Scenario: unusable TMPDIR
- **WHEN** `mktemp` cannot create a file
- **THEN** a message naming the temporary-file failure is printed to stderr
- **AND** the wrapped command still runs and its exit status is preserved
