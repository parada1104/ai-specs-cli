# recipe-sync-materialization Specification

## Purpose

Define how `ai-specs sync` resolves, validates, and materializes recipes into the project workspace, including the external directory layout for recipe-bundled and dependency skills.

## MODIFIED Requirements

### Requirement: Materialization order
The system SHALL materialize primitives in this order: skills (bundled then deps), commands, MCP presets, templates, docs. Bundled recipe skills SHALL be materialized into `.recipe/{recipe-id}/skills/{skill-id}/`. Dependency skills SHALL be materialized into `.deps/{dep-id}/skills/{skill-id}/`. Commands, MCP presets, templates, and docs SHALL continue to be materialized into `ai-specs/` as before.

#### Scenario: Full materialization with external directories
- **WHEN** a valid recipe with bundled skills and deps is processed
- **THEN** bundled skills SHALL be created under `.recipe/{recipe-id}/skills/`
- **AND** dep skills SHALL be created under `.deps/{dep-id}/skills/`
- **AND** commands, templates, docs, and MCP presets SHALL be created in `ai-specs/`
- **AND** derived artifacts (AGENTS.md, agent configs) SHALL reflect new skills and commands

#### Scenario: Re-sync idempotency with external directories
- **WHEN** sync runs twice with no changes
- **THEN** the second run SHALL not fail
- **AND** no unintended modifications SHALL occur in `.recipe/`, `.deps/`, or `ai-specs/`

## ADDED Requirements

### Requirement: Recipe skill materialization path
Bundled skills from a recipe SHALL be materialized to `.recipe/{recipe-id}/skills/{skill-id}/`, preserving the directory structure from `catalog/recipes/<id>/skills/`.

#### Scenario: Single bundled skill
- **WHEN** a recipe declares a bundled skill `id = "my-skill"`
- **THEN** sync SHALL create `.recipe/{recipe-id}/skills/my-skill/SKILL.md`
- **AND** any assets under `catalog/recipes/<id>/skills/my-skill/assets/` SHALL be copied to `.recipe/{recipe-id}/skills/my-skill/assets/`

### Requirement: Dependency skill materialization path
Dependency skills from a recipe's `[[deps]]` table SHALL be materialized to `.deps/{dep-id}/skills/{skill-id}/`.

#### Scenario: Single dependency skill
- **WHEN** a recipe declares a dependency skill `id = "vendor-skill"` from `dep-id = "vendor-lib"`
- **THEN** sync SHALL create `.deps/vendor-lib/skills/vendor-skill/SKILL.md`
- **AND** the skill contents SHALL match the vendored source

### Requirement: Local skills directory untouched
Sync SHALL NOT write bundled or dependency skills into `ai-specs/skills/`. `ai-specs/skills/` SHALL remain exclusively for local, project-owned skills.

#### Scenario: Sync with existing local skills
- **GIVEN** `ai-specs/skills/local-skill/` exists
- **WHEN** sync materializes recipe and dep skills
- **THEN** `ai-specs/skills/local-skill/` SHALL remain unchanged
- **AND** no new directories SHALL be created under `ai-specs/skills/`
