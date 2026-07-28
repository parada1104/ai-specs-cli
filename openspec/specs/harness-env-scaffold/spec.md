# harness-env-scaffold Specification

## Purpose

Define how interactive configure/init/recipe-add flows scaffold harness MCP
environment files: secrets in project-root `ai-specs.env`, committed examples, a
merge-safe project-root `.envrc` managed block, legacy migration, and
`direnv allow`.

## Requirements

### Requirement: Harness secrets live in ai-specs.env

When interactive configure/init/recipe-add collects MCP environment values for
enabled recipes, the system SHALL write those values to project-root
`ai-specs.env` as dotenv `KEY=value` lines. The system SHALL NOT write secret
values into project-root `.envrc`. The system SHALL NOT modify the application's
project-root `.env`.

#### Scenario: Wizard writes harness env file

- **GIVEN** enabled recipes reference `$TRELLO_API_KEY` in MCP env
- **AND** the user provides a value in the interactive prompt
- **WHEN** the env offer path completes successfully
- **THEN** `ai-specs.env` MUST contain `TRELLO_API_KEY=<value>`
- **AND** project-root `.envrc` MUST NOT contain that secret value inline

#### Scenario: Application .env is untouched

- **GIVEN** project-root `.env` already exists with application keys
- **WHEN** the env offer path writes harness values
- **THEN** project-root `.env` MUST be byte-identical to before the run

### Requirement: Committed ai-specs.env.example template

The system SHALL be able to generate project-root `ai-specs.env.example` from
enabled recipes' `[[provides.mcp]]` `$VAR` references, with empty values and
purpose/help comments. Existing `ai-specs.env.example` SHALL be backed up to
`ai-specs.env.example.bak` before overwrite. `ai-specs/.env.example` and
`ai-specs/.envrc.example` SHALL NOT be the primary template (deprecated stubs).

#### Scenario: Example lists required vars

- **GIVEN** enabled recipes require `TRELLO_API_KEY` and `CANONICAL_VAULT_PATH`
- **WHEN** `generate_env_example` runs
- **THEN** `ai-specs.env.example` MUST list both variables
- **AND** known vars MUST include curated help text when available

### Requirement: Merge-safe project-root .envrc managed block

The system SHALL ensure project-root `.envrc` contains a managed block bounded by
the markers `# managed-by: ai-specs (do not remove block)` and
`# end managed-by: ai-specs`, whose body is exactly:

```text
dotenv_if_exists .env
dotenv_if_exists ai-specs.env
```

If `.envrc` is missing, create it with the managed block. If present without
markers, append the managed block. If present with markers, replace only the
marked region. User content outside the markers MUST be preserved.

#### Scenario: Create root envrc when missing

- **GIVEN** no project-root `.envrc`
- **WHEN** `ensure_root_envrc` runs
- **THEN** `.envrc` MUST exist and contain the managed markers and both
  `dotenv_if_exists` lines

#### Scenario: Preserve custom direnv content

- **GIVEN** project-root `.envrc` with custom lines and no managed markers
- **WHEN** `ensure_root_envrc` runs
- **THEN** the custom lines MUST remain
- **AND** the managed block MUST appear after them

#### Scenario: Idempotent managed replace

- **GIVEN** project-root `.envrc` already containing the managed block
- **WHEN** `ensure_root_envrc` runs twice
- **THEN** exactly one managed block MUST be present

### Requirement: Legacy harness env migration

When `ai-specs/.envrc` exists, the system SHALL parse `export VAR=...` lines and
merge values into project-root `ai-specs.env` without overwriting non-empty
existing keys, rename the legacy file to a backup, and ensure the root managed
`.envrc` block. When nested `ai-specs/.env` exists, the system SHALL merge its
dotenv keys into `ai-specs.env` the same way and rename it to a backup.

#### Scenario: Migrate exports into ai-specs.env

- **GIVEN** `ai-specs/.envrc` contains `export TRELLO_TOKEN="abc"`
- **AND** `ai-specs.env` does not define `TRELLO_TOKEN`
- **WHEN** migration runs
- **THEN** `ai-specs.env` MUST contain `TRELLO_TOKEN=abc`
- **AND** `ai-specs/.envrc` MUST no longer exist as the active file
- **AND** a backup of the legacy file MUST exist

#### Scenario: Migrate nested ai-specs/.env

- **GIVEN** `ai-specs/.env` contains `TRELLO_API_KEY=legacy`
- **AND** `ai-specs.env` does not define `TRELLO_API_KEY`
- **WHEN** nested migration runs
- **THEN** `ai-specs.env` MUST contain `TRELLO_API_KEY=legacy`
- **AND** `ai-specs/.env` MUST no longer exist as the active file
- **AND** a backup of the nested file MUST exist

### Requirement: direnv allow on project root

After scaffolding harness env + root `.envrc`, interactive flows SHOULD run
`direnv allow <project_root>`. If `direnv` is missing, the flow SHALL soft-fail with
install guidance (and MAY offer opt-in install when TTY install is available).
Failures MUST NOT abort recipe configuration.

#### Scenario: Allow succeeds

- **GIVEN** `direnv` is on PATH
- **AND** root `.envrc` was ensured
- **WHEN** the env offer path finishes
- **THEN** the system MUST invoke `direnv allow` on the project root

#### Scenario: Soft-fail without direnv

- **GIVEN** `direnv` is not on PATH
- **WHEN** the env offer path finishes
- **THEN** configuration MUST still complete
- **AND** the user MUST see non-fatal guidance to install/allow direnv
