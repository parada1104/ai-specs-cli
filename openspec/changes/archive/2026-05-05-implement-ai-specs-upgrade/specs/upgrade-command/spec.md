# upgrade-command Specification

## Purpose

Define the behavior of `ai-specs upgrade`, a first-class CLI command that detects the current installation channel, validates pre-flight conditions, fast-forwards the global checkout safely, and reports the result.

## Requirements

### Requirement: Command availability and help

The system SHALL expose `ai-specs upgrade [--dry-run]` as a top-level subcommand dispatched from `bin/ai-specs`.

#### Scenario: Help lists upgrade
- **GIVEN** the CLI is installed
- **WHEN** a user runs `ai-specs help`
- **THEN** the help output MUST list `upgrade` as an available command
- **AND** the description MUST identify it as an update command for the global installation

#### Scenario: Upgrade accepts dry-run flag
- **GIVEN** a valid global installation exists
- **WHEN** a user runs `ai-specs upgrade --dry-run`
- **THEN** the command MUST print what would change without mutating the repository
- **AND** the exit code MUST be 0

### Requirement: Detection of a valid global install

The system SHALL detect a valid global installation by verifying, in order: the resolved path of the running `ai-specs` binary, the presence of `AI_SPECS_HOME`, the existence of `~/.ai-specs/.git`, and the validity of the symlink at `~/.local/bin/ai-specs` pointing into `~/.ai-specs/bin/ai-specs`.

#### Scenario: Valid global install detected
- **GIVEN** `AI_SPECS_HOME` is set to `~/.ai-specs`
- **AND** `~/.ai-specs/.git` exists
- **AND** `~/.local/bin/ai-specs` is a symlink resolving to `~/.ai-specs/bin/ai-specs`
- **WHEN** `ai-specs upgrade` runs
- **THEN** the command MUST identify the installation as the stable global channel
- **AND** the command MUST proceed to pre-flight checks

#### Scenario: Missing or broken install detected
- **GIVEN** `AI_SPECS_HOME` is unset or empty
- **OR** `~/.ai-specs/.git` does not exist
- **OR** `~/.local/bin/ai-specs` is missing, not a symlink, or resolves outside `~/.ai-specs`
- **WHEN** `ai-specs upgrade` runs
- **THEN** the command MUST abort with an explicit error message describing the broken installation
- **AND** the error message MUST recommend re-running `install.sh`
- **AND** the exit code MUST be non-zero

### Requirement: Dev channel protection

The system MUST refuse to upgrade installations that live outside the standard global path (`~/.ai-specs`), including `ai-specs-dev` checkouts and any local development clones, to prevent accidental mutation of a developer's working tree.

#### Scenario: Dev channel detected and protected
- **GIVEN** the resolved `ai-specs` binary lives outside `~/.ai-specs` (for example, a symlink to `ai-specs-dev/bin/ai-specs` or a local clone)
- **WHEN** `ai-specs upgrade` runs
- **THEN** the command MUST abort with an explicit message identifying the non-standard installation path
- **AND** the message MUST instruct the user to pull manually with `git pull` in the correct directory
- **AND** the command MUST NOT mutate any git repository
- **AND** the exit code MUST be non-zero

### Requirement: Pre-flight checks

The system SHALL verify that the target repository is on a branch that can fast-forward to `origin/main`, that `origin/main` is reachable, and that the working tree is clean. A dirty working tree MUST block the upgrade unless `--force` is passed.

#### Scenario: Dirty working tree blocks upgrade
- **GIVEN** a valid global installation
- **AND** the working tree contains uncommitted changes
- **AND** the user did not pass `--force`
- **WHEN** `ai-specs upgrade` runs
- **THEN** the command MUST abort with an explicit message listing the dirty state
- **AND** the message MUST suggest using `--force` or stashing the changes
- **AND** the command MUST NOT pull or mutate the repository
- **AND** the exit code MUST be non-zero

#### Scenario: Dirty working tree with force flag
- **GIVEN** a valid global installation
- **AND** the working tree contains uncommitted changes
- **AND** the user passed `--force`
- **WHEN** `ai-specs upgrade` runs
- **THEN** the command MUST print a warning about the dirty tree
- **AND** the command MAY proceed with the fast-forward pull
- **AND** the exit code on success MUST be 0

### Requirement: Fast-forward upgrade

The system SHALL perform the upgrade by fetching `origin/main` and merging it with `--ff-only`. If a fast-forward is not possible, the command MUST abort with actionable guidance.

#### Scenario: Successful fast-forward upgrade
- **GIVEN** a valid global installation on a branch behind `origin/main`
- **AND** the working tree is clean
- **WHEN** `ai-specs upgrade` runs
- **THEN** the command MUST execute `git fetch origin main`
- **AND** the command MUST execute `git merge --ff-only origin/main`
- **AND** on success the command MUST print a summary of the upgrade
- **AND** the exit code MUST be 0

#### Scenario: Non-fast-forward blocked
- **GIVEN** a valid global installation
- **AND** the local branch has diverged from `origin/main` such that `--ff-only` would fail
- **WHEN** `ai-specs upgrade` runs
- **THEN** the merge MUST NOT be attempted with `--no-ff` or any other fallback
- **AND** the command MUST abort with an explicit message explaining the divergence
- **AND** the message MUST recommend manual resolution or a fresh `install.sh`
- **AND** the exit code MUST be non-zero

### Requirement: Dry-run mode behavior

The system SHALL support `--dry-run` to preview the upgrade without modifying the repository. In dry-run mode, the command MUST perform all read-only detection and pre-flight checks and print the expected version change, but MUST NOT fetch, merge, or otherwise mutate the target repository.

#### Scenario: Dry-run previews the upgrade
- **GIVEN** a valid global installation that is behind `origin/main`
- **WHEN** `ai-specs upgrade --dry-run` runs
- **THEN** the command MUST perform detection and pre-flight checks
- **AND** the command MUST print the current version and the version that would be installed
- **AND** the command MUST explicitly state that no changes were made
- **AND** the command MUST NOT fetch, merge, or write to the repository
- **AND** the exit code MUST be 0

### Requirement: Post-upgrade version verification

After a successful pull, the system SHALL verify that the upgraded checkout reports a different version than before and print a clear old-to-new version diff.

#### Scenario: Version diff printed after upgrade
- **GIVEN** a successful fast-forward upgrade from version `A` to version `B`
- **WHEN** the pull completes
- **THEN** the command MUST read the new version from the repository
- **AND** the command MUST print the old version `A` and the new version `B`
- **AND** if `A` equals `B`, the command MUST print a message stating that the installation was already up to date
- **AND** the exit code MUST be 0

### Requirement: Symlink integrity check

After a successful pull, the system SHALL verify that the symlink at `~/.local/bin/ai-specs` still resolves to `bin/ai-specs` inside the upgraded checkout. If the symlink is broken or points elsewhere, the command MUST warn the user and recommend running `install.sh`.

#### Scenario: Symlink remains valid after upgrade
- **GIVEN** a successful fast-forward upgrade
- **AND** `~/.local/bin/ai-specs` was a valid symlink before the upgrade
- **WHEN** post-upgrade verification runs
- **THEN** the command MUST confirm the symlink still resolves to `~/.ai-specs/bin/ai-specs`
- **AND** the confirmation MUST be printed as part of the upgrade summary

#### Scenario: Symlink broken after upgrade
- **GIVEN** a successful fast-forward upgrade
- **AND** `~/.local/bin/ai-specs` no longer resolves to `~/.ai-specs/bin/ai-specs` after the pull (for example, because the file was renamed)
- **WHEN** post-upgrade verification runs
- **THEN** the command MUST emit a warning
- **AND** the warning MUST recommend re-running `install.sh` to repair the symlink
- **AND** the command MUST exit non-zero
