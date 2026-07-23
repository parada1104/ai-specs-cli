## MODIFIED Requirements

### Requirement: Four-tier skill source scanning

`sync-agent` SHALL scan exactly four sources for skills, in this order:

1. `ai-specs/skills/{id}/` — local project skills (highest precedence)
2. `{cache}/.recipe/{recipe-id}/skills/{id}/` — recipe-bundled skills
3. `{cache}/.deps/{dep-id}/skills/{id}/` — vendored dependency skills
4. `{cache}/.bundled/skills/{id}/` — CLI-bundled skills, flattened from
   `$AI_SPECS_HOME/bundled-skills/` (lowest precedence)

CLI-bundled skills are recipe-independent and shipped by the CLI. They SHALL NOT
be materialized into `ai-specs/skills/` and SHALL NOT be committed to the
project. Fan-out targets remain unchanged.

#### Scenario: Bundled skill resolves from cache, not project

- **GIVEN** a CLI-bundled skill `id = "harness-lifecycle"` and no local skill of
  that id
- **WHEN** `sync-agent` resolves the skill path
- **THEN** it SHALL select the copy from `{cache}/.bundled/skills/harness-lifecycle/`
- **AND** `ai-specs/skills/harness-lifecycle/` MUST NOT exist after sync

#### Scenario: Local skill shadows a CLI-bundled skill of the same id

- **GIVEN** a local `ai-specs/skills/skill-creator/` authored in the project
- **AND** a CLI-bundled skill `skill-creator` in `{cache}/.bundled/skills/`
- **WHEN** `sync-agent` resolves the skill path
- **THEN** it SHALL select the local copy
- **AND** it SHALL NOT delete the locally-authored copy during cleanup
