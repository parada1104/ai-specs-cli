# project-doctor (delta)

## ADDED Requirements

### Requirement: direnv substrate diagnostics

When enabled recipes declare MCP env variable references, `ai-specs doctor` SHALL
report whether `direnv` is available on PATH. Missing `direnv` MUST be WARN (not
ERROR) and MUST include install guidance. Doctor MUST NOT install `direnv`.

#### Scenario: direnv missing with MCP env required

- **GIVEN** an enabled recipe MCP env references `$TRELLO_API_KEY`
- **AND** `direnv` is not on PATH
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include a WARN check for `direnv`
- **AND** guidance MUST mention how to install or enable direnv
- **AND** doctor MUST NOT run an installer

#### Scenario: No MCP env skips direnv warn

- **GIVEN** no enabled recipe MCP env references
- **AND** `direnv` is not on PATH
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST NOT require a `direnv` WARN solely for that absence

### Requirement: Managed root .envrc diagnostics

When enabled recipes declare MCP env variable references, doctor SHALL WARN if
project-root `.envrc` is missing or lacks the ai-specs managed-by markers.
Doctor MUST remain read-only.

#### Scenario: Missing managed block

- **GIVEN** MCP env vars are required by enabled recipes
- **AND** project-root `.envrc` exists without managed-by markers
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include a WARN recommending
  `ai-specs configure-recipes` (or equivalent) to ensure the managed block

### Requirement: Harness env key diagnostics

When enabled recipes declare MCP env variable references, doctor SHALL WARN for
each required variable that is missing or empty in `ai-specs/.env`. Doctor MUST
NOT print secret values.

#### Scenario: Empty harness key

- **GIVEN** enabled recipes require `TRELLO_TOKEN`
- **AND** `ai-specs/.env` exists but `TRELLO_TOKEN` is missing or empty
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST include a WARN naming `TRELLO_TOKEN`
- **AND** the message MUST NOT include any secret value

#### Scenario: Present harness key is OK

- **GIVEN** enabled recipes require `TRELLO_TOKEN`
- **AND** `ai-specs/.env` contains a non-empty `TRELLO_TOKEN`
- **WHEN** `ai-specs doctor` runs
- **THEN** the report MUST NOT WARN for that key as missing
