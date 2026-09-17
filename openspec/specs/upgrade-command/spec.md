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

### Requirement: Compact upgrade output by default

`ai-specs upgrade` SHALL NOT forward raw `git` output to the terminal in its
default mode. Each mutating step SHALL print exactly one labelled progress line.

Output discipline SHALL match the established `run_step` contract already used
by `ai-specs sync` (`lib/sync.sh:119`).

#### Scenario: Successful upgrade prints no git transfer log
- **WHEN** `ai-specs upgrade` fast-forwards from one version to another
- **THEN** no `remote:` counting lines, transfer progress, or fast-forward
  diffstat appear
- **AND** one labelled line is printed per mutating step

#### Scenario: Verbose restores full detail
- **WHEN** `ai-specs upgrade -v` (or `--verbose`) runs
- **THEN** the complete unfiltered `git` output is printed

#### Scenario: A failing step always prints everything
- **WHEN** any step exits non-zero
- **THEN** that step's full captured stdout and stderr are printed regardless of
  verbosity
- **AND** the existing exit code for that failure is preserved unchanged

#### Scenario: Safety behavior is unchanged
- **WHEN** the installation is a dev checkout, the tree is dirty, or the local
  branch has diverged
- **THEN** the abort message and exit code are exactly what they were before
  this change

### Requirement: Version crossing summary

On a successful upgrade that changes the version, `ai-specs upgrade` SHALL
summarize what the user crossed, sourced from `CHANGELOG.md` in the upgraded
checkout.

#### Scenario: Single version step
- **WHEN** the upgrade moves from `0.21.0` to `0.22.0`
- **THEN** the summary covers the `0.22.0` entry

#### Scenario: Multiple versions crossed
- **WHEN** the upgrade moves from `0.19.0` to `0.22.0`
- **THEN** the summary covers `0.20.0`, `0.20.1`, `0.21.0` and `0.22.0`
- **AND** entries are ordered newest first

#### Scenario: Already up to date
- **WHEN** the installation is already at the target version
- **THEN** no summary is printed and the existing up-to-date message is
  preserved

#### Scenario: Changelog is unreadable
- **WHEN** `CHANGELOG.md` is missing, unparseable, or has no matching section
- **THEN** the upgrade still succeeds
- **AND** the plain `Upgraded: <old> -> <new>` line is printed
- **AND** no traceback or parser error reaches the user

### Requirement: Version-keyed upgrade notices

A release MAY declare post-upgrade actions. Notices SHALL be authored in
`CHANGELOG.md` under the version they belong to, in a subsection titled
`### Upgrade notes`.

`ai-specs upgrade` SHALL replay the notices of every version in the crossed
range, ordered oldest first, so that instructions are applied in release order.

Notices SHALL be unconditional prose. `ai-specs upgrade` operates on the global
installation and has no consumer project in scope, so a notice SHALL NOT express
project-dependent conditions and SHALL NOT be evaluated, templated, or executed.
Project-dependent guidance belongs to `ai-specs doctor`.

#### Scenario: Crossing a version that declares a notice
- **WHEN** the upgrade crosses a version whose changelog entry has an
  `### Upgrade notes` subsection
- **THEN** that notice text is printed under a clearly separated heading
- **AND** it is visually distinguishable from the version summary

#### Scenario: Notices replay in release order across multiple versions
- **WHEN** the upgrade crosses several versions and more than one declares a
  notice
- **THEN** every such notice is printed
- **AND** they appear oldest version first

#### Scenario: No notice declared
- **WHEN** no crossed version declares an `### Upgrade notes` subsection
- **THEN** no notice section is printed and no placeholder appears

#### Scenario: Notices are never suppressed by compact mode
- **WHEN** `ai-specs upgrade` runs without `--verbose`
- **THEN** declared notices are still printed in full

#### Scenario: A notice is not executed
- **WHEN** a notice contains a command such as `ai-specs sync`
- **THEN** the command is displayed as text and is never run by `upgrade`

### Requirement: Narrowed global installation

The global installation SHALL NOT materialize subtrees that the CLI does not
read at runtime. `openspec/`, `tests/`, `.github/` and `tmp/` SHALL be excluded
from the working tree of `~/.ai-specs`.

Narrowing SHALL use a partial clone (`--filter=blob:none`) with a cone-mode
sparse checkout. Narrowing SHALL NOT use a shallow clone, because
`ai-specs upgrade` depends on `git merge-base --is-ancestor` for its divergence
guard.

#### Scenario: Fresh install is narrowed
- **WHEN** `install.sh` provisions a new installation on a Git that supports
  partial clone and cone-mode sparse checkout
- **THEN** `openspec/`, `tests/`, `.github/` and `tmp/` are absent from the
  working tree
- **AND** every path the CLI reads at runtime is present

#### Scenario: Existing full install narrows on upgrade
- **WHEN** `ai-specs upgrade` runs against an installation that was cloned in
  full
- **THEN** the checkout is narrowed as part of the upgrade
- **AND** re-running the upgrade performs no further narrowing work

#### Scenario: Full commit history is preserved
- **WHEN** an installation has been narrowed
- **THEN** `git merge-base --is-ancestor HEAD origin/main` still resolves
  correctly
- **AND** the divergence guard behaves identically to a full clone

#### Scenario: Unsupported Git falls back
- **WHEN** the available Git does not support `--filter=blob:none` or cone-mode
  sparse checkout
- **THEN** the installation is provisioned as a full checkout
- **AND** install and upgrade both succeed

#### Scenario: Narrowing failure never blocks an upgrade
- **WHEN** narrowing fails for any reason
- **THEN** a warning is emitted
- **AND** the upgrade completes successfully with the checkout left usable
