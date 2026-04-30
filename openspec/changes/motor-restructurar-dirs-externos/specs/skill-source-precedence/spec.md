# skill-source-precedence Specification

## Purpose

Define the multi-source skill scanning behavior and precedence rules used by `sync-agent` to resolve skill paths across local, recipe-bundled, and vendored dependency sources.

## ADDED Requirements

### Requirement: Three-tier skill source scanning
`sync-agent` SHALL scan exactly three sources for skills, in this order:
1. `ai-specs/skills/{id}/` — local project skills (highest precedence)
2. `.recipe/{recipe-id}/skills/{id}/` — recipe-bundled skills (middle precedence)
3. `.deps/{dep-id}/skills/{id}/` — vendored dependency skills (lowest precedence)

#### Scenario: All three sources present
- **GIVEN** a skill with `id = "test-skill"` exists in all three sources
- **WHEN** `sync-agent` resolves the skill path
- **THEN** it SHALL select the copy from `ai-specs/skills/test-skill/`

#### Scenario: Only recipe and dep sources present
- **GIVEN** a skill exists in `.recipe/my-recipe/skills/test-skill/` and `.deps/my-dep/skills/test-skill/`
- **WHEN** `sync-agent` resolves the skill path
- **THEN** it SHALL select the copy from `.recipe/my-recipe/skills/test-skill/`

#### Scenario: Only dep source present
- **GIVEN** a skill exists only in `.deps/my-dep/skills/test-skill/`
- **WHEN** `sync-agent` resolves the skill path
- **THEN** it SHALL select the copy from `.deps/my-dep/skills/test-skill/`

### Requirement: Precedence is source-level, not file-level
If a skill is selected from a higher-precedence source, the system SHALL use the entire skill directory from that source. It SHALL NOT merge individual files across sources.

#### Scenario: Skill selected from local source
- **GIVEN** `ai-specs/skills/test-skill/SKILL.md` exists but `ai-specs/skills/test-skill/assets/` does not
- **AND** `.recipe/my-recipe/skills/test-skill/assets/` exists
- **WHEN** the skill is resolved
- **THEN** the system SHALL use only files from `ai-specs/skills/test-skill/`
- **AND** it SHALL NOT fall back to `.recipe/my-recipe/skills/test-skill/assets/`

### Requirement: Missing skill in all sources is an error
If a skill ID is referenced by the manifest or by a recipe but does not exist in any of the three sources, `sync-agent` SHALL fail with an explicit error.

#### Scenario: Skill not found in any source
- **WHEN** a skill `id = "missing-skill"` is required but absent from all sources
- **THEN** `sync-agent` SHALL fail
- **AND** the error SHALL name the missing skill ID

### Requirement: Multiple recipes or deps with the same skill ID
When the same skill ID exists in multiple recipes or multiple dependencies, the system SHALL apply first-seen ordering within the same precedence tier and emit a warning.

#### Scenario: Same skill in two recipes
- **GIVEN** `.recipe/recipe-a/skills/shared-skill/` exists
- **AND** `.recipe/recipe-b/skills/shared-skill/` exists
- **WHEN** `sync-agent` resolves the skill
- **THEN** it SHALL select the copy from `recipe-a` (first in declaration order)
- **AND** it SHALL emit a warning naming both recipes and the skill ID
