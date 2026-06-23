# cli-version-contract Specification

## Purpose

Define how projects declare an expected ai-specs CLI version, how the CLI records
the version used during sync in the lock file, and how sync enforces version policy
before mutating project files.

## ADDED Requirements

### Requirement: Tool section in manifest

The system SHALL support an optional `[tool]` table in `ai-specs/ai-specs.toml` with
the following fields:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `version` | string | no | Exact CLI version pin (semver) |
| `min_version` | string | no | Minimum acceptable CLI version (semver) |
| `policy` | string | no | `exact` or `min`; inferred when omitted |

When both `version` and `min_version` are absent, no sync enforcement SHALL occur.
When `policy` is omitted:
- if `version` is set → default `exact`
- if only `min_version` is set → default `min`

The system MUST reject `[tool]` when both `version` and `min_version` are set.
The system MUST reject unknown `policy` values.

#### Scenario: Exact pin declared

- **GIVEN** a manifest containing:
  ```toml
  [tool]
  version = "0.12.2"
  policy = "exact"
  ```
- **WHEN** the manifest is parsed for sync
- **THEN** the effective policy MUST be exact pin to `0.12.2`

#### Scenario: Min version inferred policy

- **GIVEN** a manifest containing:
  ```toml
  [tool]
  min_version = "0.11.0"
  ```
- **WHEN** the manifest is parsed for sync
- **THEN** the effective policy MUST be `min` with floor `0.11.0`

#### Scenario: Conflicting version fields rejected

- **GIVEN** a manifest containing both `[tool].version` and `[tool].min_version`
- **WHEN** sync or doctor validates the manifest
- **THEN** the command MUST fail with an explicit error
- **AND** MUST NOT mutate project files

### Requirement: Installed CLI version resolution

The system SHALL resolve the installed CLI version by reading the `VERSION` file
from `AI_SPECS_HOME` (the directory containing the running CLI's `lib/` tree).
If the file is missing or empty, the installed version MUST be reported as `unknown`.

#### Scenario: Version read from AI_SPECS_HOME

- **GIVEN** `AI_SPECS_HOME/VERSION` contains `0.12.2\n`
- **WHEN** the CLI resolves its installed version
- **THEN** the installed version MUST be `0.12.2`

#### Scenario: Missing VERSION file

- **GIVEN** `AI_SPECS_HOME/VERSION` does not exist
- **WHEN** the CLI resolves its installed version
- **THEN** the installed version MUST be `unknown`

### Requirement: Semver comparison

The system SHALL compare CLI versions using numeric semver tuples (`major.minor.patch`).
Pre-release suffixes (e.g. `-rc1`) MUST compare lower than the corresponding release.
Build metadata after `+` MUST be ignored for ordering.

#### Scenario: Patch ordering

- **GIVEN** installed version `0.12.2` and pinned version `0.12.3`
- **WHEN** exact policy is evaluated
- **THEN** the policy MUST NOT be satisfied

#### Scenario: Pre-release lower than release

- **GIVEN** installed version `0.12.2-rc1` and min_version `0.12.2`
- **WHEN** min policy is evaluated
- **THEN** the policy MUST NOT be satisfied

### Requirement: Lock file meta section

On every command that writes `ai-specs/.ai-specs.lock`, the system SHALL update a
`[meta]` table with:

| Key | Type | Description |
|-----|------|-------------|
| `cli_version` | string | Installed CLI version at write time |
| `synced_at` | string | ISO-8601 UTC timestamp (`YYYY-MM-DDTHH:MM:SSZ`) |

Existing hash tables in the lock file MUST remain unchanged in behavior. The `[meta]`
table MUST NOT affect refresh-bundled hash decisions.

#### Scenario: Meta written after sync

- **GIVEN** a project with a valid manifest
- **AND** installed CLI version `0.12.2`
- **WHEN** `ai-specs sync` completes successfully
- **THEN** `ai-specs/.ai-specs.lock` MUST contain `[meta].cli_version = "0.12.2"`
- **AND** `[meta].synced_at` MUST be a valid ISO-8601 UTC timestamp

#### Scenario: Legacy lock without meta remains valid

- **GIVEN** a lock file with skill/command hashes but no `[meta]` table
- **WHEN** refresh-bundled loads the lock
- **THEN** the command MUST succeed
- **AND** MUST treat last-synced CLI version as absent

### Requirement: Sync version gate

Before any project file mutation in `ai-specs sync`, the system SHALL evaluate the
effective `[tool]` policy against the installed CLI version.

- When policy is `exact` and versions differ → MUST abort with exit code 1.
- When policy is `min` and installed < min_version → MUST abort with exit code 1.
- When no `[tool]` policy is configured → MUST NOT abort solely for version reasons.

Error messages MUST name: installed version, required version/policy, and suggest
`ai-specs upgrade` or adjusting the manifest pin.

#### Scenario: Exact pin mismatch blocks sync

- **GIVEN** installed CLI `0.11.0`
- **AND** manifest `[tool].version = "0.12.2"` with policy `exact`
- **WHEN** `ai-specs sync` runs
- **THEN** sync MUST abort before writing files
- **AND** exit code MUST be 1
- **AND** stderr MUST mention both versions

#### Scenario: Min version satisfied allows sync

- **GIVEN** installed CLI `0.12.2`
- **AND** manifest `[tool].min_version = "0.11.0"`
- **WHEN** `ai-specs sync` runs
- **THEN** sync MUST proceed normally

#### Scenario: No tool section skips gate

- **GIVEN** a manifest without `[tool]`
- **WHEN** `ai-specs sync` runs
- **THEN** sync MUST NOT fail solely because of CLI version

### Requirement: Escape hatch flag

`ai-specs sync` SHALL accept `--ignore-cli-version` to skip the version gate.
When used, the command MUST print a warning to stderr that the pin was ignored.

#### Scenario: Ignore flag bypasses exact pin

- **GIVEN** installed CLI `0.11.0` and exact pin `0.12.2`
- **WHEN** `ai-specs sync --ignore-cli-version` runs
- **THEN** sync MUST proceed
- **AND** MUST print a warning that CLI version policy was ignored

### Requirement: Changelog for migrations

The repository SHALL maintain a root `CHANGELOG.md` following Keep a Changelog format.
Each release section MUST list user-visible CLI/manifest changes relevant to migration.
The initial file MUST document at least version `0.12.2` and note the introduction of
`[tool]` pinning in the unreleased/next section.

#### Scenario: Changelog exists at repo root

- **GIVEN** this change is applied
- **WHEN** a user opens the repository root
- **THEN** `CHANGELOG.md` MUST exist
- **AND** MUST contain a `[Unreleased]` or `[0.12.3]` section mentioning `[tool]` pinning
