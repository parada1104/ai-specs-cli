# skill-source-precedence Specification

## Purpose

Define the multi-source skill scanning behavior and precedence rules used by
`sync-agent` to resolve skill paths across local, recipe-bundled, vendored
dependency, and CLI-bundled sources.

## Requirements

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

#### Scenario: All higher-tier sources present

- **GIVEN** a skill with `id = "test-skill"` exists in local, recipe, and dep
  sources
- **WHEN** `sync-agent` resolves the skill path
- **THEN** it SHALL select the copy from `ai-specs/skills/test-skill/`

#### Scenario: Only recipe and dep sources present

- **GIVEN** a skill exists in cache `.recipe/my-recipe/skills/test-skill/` and
  cache `.deps/my-dep/skills/test-skill/`
- **WHEN** `sync-agent` resolves the skill path
- **THEN** it SHALL select the copy from cache `.recipe/my-recipe/skills/test-skill/`

#### Scenario: Only dep source present

- **GIVEN** a skill exists only in cache `.deps/my-dep/skills/test-skill/`
- **WHEN** `sync-agent` resolves the skill path
- **THEN** it SHALL select the copy from cache `.deps/my-dep/skills/test-skill/`

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

### Requirement: Command merge

Commands SHALL resolve from three sources with source-level precedence,
highest to lowest:

1. `ai-specs/commands/{name}.md` — local hand-authored (highest)
2. `{cache}/commands/{name}.md` — recipe-managed
3. `{cache}/.bundled/commands/{name}.md` — CLI-bundled, flattened from
   `$AI_SPECS_HOME/bundled-commands/` (lowest)

CLI-bundled commands are recipe-independent and shipped by the CLI. They SHALL
NOT be materialized into `ai-specs/commands/` and SHALL NOT be committed to the
project. On conflict, hand-authored commands win over both lower tiers;
recipe-managed commands win over CLI-bundled commands. Fan-out targets remain
unchanged.

#### Scenario: Merge and fan-out

- **GIVEN** a command id present in both cache-managed and hand-authored locations
- **WHEN** fan-out runs
- **THEN** the hand-authored command wins
- **AND** agent command targets match pre-change behavior aside from source relocation

#### Scenario: Bundled command resolves from cache, not project

- **GIVEN** a CLI-bundled command `rules-audit` and no local or recipe-managed
  command of that name
- **WHEN** `sync` (or `refresh-bundled`) runs
- **THEN** it SHALL flatten `rules-audit.md` into `{cache}/.bundled/commands/`
- **AND** `ai-specs/commands/rules-audit.md` MUST NOT exist afterward
- **AND** the merged command set used for fan-out MUST still include
  `rules-audit`

#### Scenario: Local command shadows a CLI-bundled command of the same name

- **GIVEN** a local `ai-specs/commands/skills-as-rules.md` authored in the project
- **AND** a CLI-bundled command `skills-as-rules` in `{cache}/.bundled/commands/`
- **WHEN** commands are merged for fan-out
- **THEN** the local copy SHALL be selected
- **AND** it SHALL NOT be deleted during leftover cleanup only if its content
  differs from the bundled source (see Requirement: Tracked bundled-command
  leftover guidance, `project-doctor` capability, for the identical-content
  case)

#### Scenario: Recipe-managed command shadows a CLI-bundled command of the same name

- **GIVEN** a recipe-managed command in `{cache}/commands/{name}.md`
- **AND** a CLI-bundled command of the same name in `{cache}/.bundled/commands/`
- **AND** no local command of that name
- **WHEN** commands are merged for fan-out
- **THEN** the recipe-managed copy SHALL be selected
- **AND** no warning SHALL be emitted (both are CLI-driven tiers)

### Requirement: Precedence is source-level, not file-level

If a skill is selected from a higher-precedence source, the system SHALL use the
entire skill directory from that source. It SHALL NOT merge individual files
across sources.

#### Scenario: Skill selected from local source

- **GIVEN** `ai-specs/skills/test-skill/SKILL.md` exists but
  `ai-specs/skills/test-skill/assets/` does not
- **AND** cache `.recipe/my-recipe/skills/test-skill/assets/` exists
- **WHEN** the skill is resolved
- **THEN** the system SHALL use only files from `ai-specs/skills/test-skill/`
- **AND** it SHALL NOT fall back to cache recipe assets

### Requirement: Missing skill in all sources is an error

If a skill ID is referenced by the manifest or by a recipe but does not exist in
any of the four sources, `sync-agent` SHALL fail with an explicit error.

#### Scenario: Skill not found in any source

- **WHEN** a skill `id = "missing-skill"` is required but absent from all sources
- **THEN** `sync-agent` SHALL fail
- **AND** the error SHALL name the missing skill ID

### Requirement: Multiple recipes or deps with the same skill ID

When the same skill ID exists in multiple recipes or multiple dependencies, the
system SHALL apply first-seen ordering within the same precedence tier and emit
a warning.

#### Scenario: Same skill in two recipes

- **GIVEN** `.recipe/recipe-a/skills/shared-skill/` exists
- **AND** `.recipe/recipe-b/skills/shared-skill/` exists
- **WHEN** `sync-agent` resolves the skill
- **THEN** it SHALL select the copy from `recipe-a` (first in declaration order)
- **AND** it SHALL emit a warning naming both recipes and the skill ID
