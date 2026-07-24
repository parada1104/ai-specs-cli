## MODIFIED Requirements

### Requirement: Lock is a provenance stamp

`ai-specs/.ai-specs.lock` SHALL record only CLI provenance — `[meta]` with
`cli_version` and `synced_at` — plus `[agents.*]` generated-file hashes used by
`doctor`'s stale-file check. It SHALL NOT contain per-file content hashes for
skills or recipes, and SHALL NOT contain `[commands]` or `[opted-out]`
sections. The lock is the CLI-provenance signal that travels with a fresh
clone (the machine-local cache `meta.toml` does not).

#### Scenario: Lock contents after sync

- **WHEN** `sync` (or `init`) completes
- **THEN** `.ai-specs.lock` contains a `[meta]` table with `cli_version` and
  `synced_at`
- **AND** it contains no `[skills.*]`, `[recipes.*]`, `[commands]`, or
  `[opted-out]` sections

#### Scenario: Legacy command hash sections dropped on migration

- **GIVEN** a `.ai-specs.lock` written by a prior CLI version with `[commands]`
  and `[opted-out]` sections
- **WHEN** `sync` (or `refresh-bundled`) runs
- **THEN** those sections are removed
- **AND** `[meta].cli_version` is updated to the running CLI version
