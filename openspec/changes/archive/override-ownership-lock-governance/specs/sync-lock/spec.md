# sync-lock (delta)

## MODIFIED Requirements

### Requirement: Lock is a provenance stamp

`ai-specs/.ai-specs.lock` SHALL record CLI provenance — `[meta]` with
`cli_version` and `synced_at` — plus `[agents.*]` generated-file hashes used by
`doctor`'s stale-file check, and MAY record `[managed.*]` integrity entries for
CLI-owned override targets as defined by the override-ownership capability.

It SHALL NOT contain per-file content hashes for skills or recipes, and SHALL
NOT contain `[commands]` or `[opted-out]` sections. The lock remains the
CLI-provenance signal that travels with a fresh clone (the machine-local cache
`meta.toml` does not). `[managed.*]` is a **scoped** exception for override
governance, not a general integrity manifest for the committed project surface.

#### Scenario: Lock contents after sync

- **WHEN** `sync` (or `init`) completes on a project with no governed overrides
- **THEN** `.ai-specs.lock` contains a `[meta]` table with `cli_version` and
  `synced_at`
- **AND** it contains no `[skills.*]`, `[recipes.*]`, `[commands]`, or
  `[opted-out]` sections

#### Scenario: Lock records managed override after template seed

- **WHEN** `sync` materializes a governed `condition=not_exists` template
- **THEN** `.ai-specs.lock` contains a `[managed."<project-relative-path>"]`
  table with at least `sha256` equal to the bytes written
- **AND** still contains no skill/recipe/command hash sections

#### Scenario: Legacy command hash sections dropped on migration

- **GIVEN** a `.ai-specs.lock` written by a prior CLI version with `[commands]`
  and `[opted-out]` sections
- **WHEN** `sync` (or `refresh-bundled`) runs
- **THEN** those sections are removed
- **AND** `[meta].cli_version` is updated to the running CLI version
- **AND** any valid `[managed.*]` entries present before the write are preserved
  or refreshed according to override-ownership rules

## MODIFIED Requirements

### Requirement: Lock is not a general integrity manifest for committed content

The lock SHALL NOT be used as a general detector of user edits across the
committed project surface; git remains the integrity/diff source for ordinary
committed content. Version-drift detection in `doctor`/`upgrade` SHALL read
`[meta].cli_version` only.

**Exception:** `[managed.*]` entries SHALL be used solely to classify
CLI-managed override targets under the override-ownership capability (managed
vs user-modified vs untracked). They SHALL NOT reintroduce skill or recipe
content hashing.

#### Scenario: Doctor reads version from meta

- **GIVEN** a project whose `.ai-specs.lock` `[meta].cli_version` is older than
  the installed CLI
- **WHEN** `doctor` runs
- **THEN** it reports version drift based on `[meta].cli_version`
- **AND** it does NOT compute per-file skill/recipe hashes
