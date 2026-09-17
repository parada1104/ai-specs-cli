# Specification: upgrade-experience

Requirements for what `ai-specs upgrade` prints, what a release may tell the
user to do, and what the global installation carries on disk.

## ADDED Requirements

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
