## ADDED Requirements

### Requirement: CLI version diagnostics

The system MUST report CLI version state as part of `ai-specs doctor` output.

The report MUST include, when available:

- **installed** — version from `AI_SPECS_HOME/VERSION`
- **pinned** — from manifest `[tool]` when configured
- **last_synced** — from `ai-specs/.ai-specs.lock` `[meta].cli_version` when present

#### Scenario: All version sources present and aligned

- **GIVEN** installed CLI `0.12.2`
- **AND** manifest `[tool].version = "0.12.2"`
- **AND** lock `[meta].cli_version = "0.12.2"`
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include an `OK` check named `cli-version`
- **AND** the message MUST mention installed, pinned, and last-synced values

#### Scenario: No pin configured with last sync recorded

- **GIVEN** installed CLI `0.12.2`
- **AND** no `[tool]` section in the manifest
- **AND** lock `[meta].cli_version = "0.10.1"`
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include a `WARN` check named `cli-version`
- **AND** the message MUST note installed differs from last-synced
- **AND** the message SHOULD suggest running `ai-specs sync` or adding a `[tool]` pin

#### Scenario: Exact pin mismatch is ERROR

- **GIVEN** installed CLI `0.11.0`
- **AND** manifest `[tool].version = "0.12.2"` with policy `exact`
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include an `ERROR` check named `cli-version`
- **AND** the command MUST exit non-zero

#### Scenario: Min version violation is ERROR

- **GIVEN** installed CLI `0.10.0`
- **AND** manifest `[tool].min_version = "0.11.0"`
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include an `ERROR` check named `cli-version`
- **AND** the command MUST exit non-zero

#### Scenario: Lock meta absent is INFO

- **GIVEN** installed CLI `0.12.2`
- **AND** no `[tool]` section
- **AND** lock file exists without `[meta]`
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include an `INFO` or `WARN` check noting last-synced is unknown
- **AND** the message SHOULD recommend running `ai-specs sync` to record meta

#### Scenario: Doctor remains read-only

- **GIVEN** any project state
- **WHEN** `ai-specs doctor` inspects CLI version
- **THEN** it MUST NOT modify the manifest, lock file, or any derived artifacts
