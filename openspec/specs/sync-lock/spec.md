# sync-lock Specification

## Purpose

Define `ai-specs/.ai-specs.lock` as a CLI provenance stamp that travels with a
fresh clone. The lock records which CLI version last synced the project and may
carry scoped integrity records for CLI-owned override targets; it is not a
general integrity manifest for committed content.

## Requirements

### Requirement: Lock is a provenance stamp

`ai-specs/.ai-specs.lock` SHALL record CLI provenance — `[meta]` with
`cli_version` and `synced_at` — plus `[agents.*]` generated-file hashes used by
`doctor`'s stale-file check. It MAY record `[managed."<project-relative-path>"]`
entries for CLI-owned governed overrides. Managed entries SHALL contain the
normalized SHA-256 of the bytes last written by the CLI and MAY contain recipe,
source, kind, and effective policy provenance. It SHALL NOT contain per-file
content hashes for skills or recipes, and SHALL NOT contain `[commands]` or
`[opted-out]` sections. The lock is the CLI-provenance signal that travels with
a fresh clone (the machine-local cache `meta.toml` does not).

#### Scenario: Lock contents after sync

- **WHEN** `sync` (or `init`) completes on a project with no governed overrides
- **THEN** `.ai-specs.lock` contains a `[meta]` table with `cli_version` and
  `synced_at`
- **AND** it contains no `[skills.*]`, `[recipes.*]`, `[commands]`, or
  `[opted-out]` sections

#### Scenario: Lock records managed override after template seed

- **WHEN** sync materializes a governed `condition = "not_exists"` template
- **THEN** `.ai-specs.lock` contains a `[managed."<target>"]` table with
  `sha256` equal to the normalized bytes written
- **AND** it still contains no skill/recipe/command hash sections

#### Scenario: Legacy command hash sections dropped on migration

- **GIVEN** a `.ai-specs.lock` written by a prior CLI version with `[commands]`
  and `[opted-out]` sections
- **WHEN** `sync` (or `refresh-bundled`) runs
- **THEN** those sections are removed
- **AND** `[meta].cli_version` is updated to the running CLI version
- **AND** valid `[managed.*]` entries are preserved

### Requirement: Lock is not an integrity manifest for committed content

The lock SHALL NOT be used to detect user edits of committed content; git is the
integrity/diff source for the committed project surface. Version-drift detection
in `doctor`/`upgrade` SHALL read `[meta].cli_version` only. `[managed.*]` is a
scoped exception used solely for governed override ownership classification and
SHALL NOT reintroduce skill, recipe, dependency, or command hashes.

#### Scenario: Doctor reads version from meta

- **GIVEN** a project whose `.ai-specs.lock` `[meta].cli_version` is older than
  the installed CLI
- **WHEN** `doctor` runs
- **THEN** it reports version drift based on `[meta].cli_version`
- **AND** it does NOT compute per-file skill/recipe hashes
