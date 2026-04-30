# recipe-sync-materialization Specification

## Purpose

Define how `ai-specs sync` resolves, validates, and materializes recipes into the project workspace, including the external directory layout for recipe-bundled and dependency skills.

## Requirements

### Requirement: Sync reads recipe declarations
During sync, the system SHALL parse all `[recipes.*]` tables from `ai-specs.toml` and filter to those with `enabled = true`.

#### Scenario: Multiple recipes enabled
- **WHEN** three recipes are declared with `enabled = true`
- **THEN** sync SHALL process all three in declaration order

### Requirement: Recipe validation
Before materialization, the system SHALL validate that: the recipe directory exists in `catalog/recipes/<id>/`, `recipe.toml` is parseable, all required fields are present, and all referenced local paths (`skills/`, `commands/`, `templates/`, `docs/`) exist.

#### Scenario: Missing recipe.toml
- **WHEN** `catalog/recipes/<id>/` exists but lacks `recipe.toml`
- **THEN** sync SHALL fail with "recipe.toml not found"

#### Scenario: Missing referenced skill directory
- **WHEN** `recipe.toml` declares a bundled skill but `skills/<id>/` does not exist
- **THEN** sync SHALL fail with "bundled skill not found"

### Requirement: Materialization order
The system SHALL materialize primitives in this order: skills (bundled then deps), commands, MCP presets, templates, docs. Bundled recipe skills SHALL be materialized into `.recipe/{recipe-id}/skills/{skill-id}/`. Dependency skills SHALL be materialized into `.deps/{dep-id}/skills/{skill-id}/`. Commands, MCP presets, templates, and docs SHALL continue to be materialized into `ai-specs/` as before.

#### Scenario: Full materialization with external directories
- **WHEN** a valid recipe with bundled skills and deps is processed
- **THEN** bundled skills SHALL be created under `.recipe/{recipe-id}/skills/`
- **AND** dep skills SHALL be created under `.deps/{dep-id}/skills/`
- **AND** commands, templates, docs, and MCP presets SHALL be created in `ai-specs/`
- **AND** derived artifacts (registry artifact, agent configs) SHALL reflect new skills and commands

#### Scenario: Re-sync idempotency with external directories
- **WHEN** sync runs twice with no changes
- **THEN** the second run SHALL not fail
- **AND** no unintended modifications SHALL occur in `.recipe/`, `.deps/`, or `ai-specs/`

### Requirement: MCP preset merge strategy
When a recipe declares an MCP preset with the same `id` as an existing `[mcp.<id>]` in the project manifest, the system SHALL merge the recipe fields into the derived config with recipe values taking precedence. The system SHALL emit a warning naming the recipe and the MCP id.

#### Scenario: Recipe MCP overrides manifest MCP
- **WHEN** the project manifest declares `[mcp.openmemory]` and a recipe also declares `mcp.id = "openmemory"`
- **THEN** sync SHALL merge the recipe fields into the derived MCP config
- **AND** recipe fields SHALL take precedence on key overlap
- **AND** sync SHALL emit a warning: "recipe <name> overrides mcp.id='openmemory' from project manifest"

### Requirement: Idempotent sync
Running sync multiple times with the same manifest SHALL produce the same result.

#### Scenario: Re-sync unchanged recipe
- **WHEN** sync runs twice with no changes
- **THEN** the second run SHALL not fail
- **AND** no unintended modifications SHALL occur

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

### Requirement: Init remains separate from sync materialization

Recipe initialization MAY preview or propose templates, overrides, and manifest config needed before sync, but init SHALL NOT run `ai-specs sync` and SHALL NOT silently materialize recipe primitives. Sync SHALL remain responsible for materializing enabled recipe primitives.

#### Scenario: Init does not run sync

- **GIVEN** a recipe declares bundled skills, commands, MCP presets, templates, and docs
- **WHEN** `ai-specs recipe init <id>` runs
- **THEN** init SHALL NOT materialize those primitives
- **AND** init SHALL NOT update derived agent configs or registries through sync

#### Scenario: Init previews template target

- **GIVEN** a recipe declares a template with `target = "ai-specs/example.md"`
- **WHEN** `ai-specs recipe init <id>` runs
- **THEN** init MAY report that the template target is relevant to setup
- **AND** init MAY propose a reviewable create or update action
- **AND** sync SHALL remain the command that materializes the declared template primitive

#### Scenario: Human-reviewed init mutation remains idempotent

- **GIVEN** an init workflow has already created or updated a project override file after review
- **WHEN** `ai-specs recipe init <id>` runs again
- **THEN** init SHALL detect the existing file
- **AND** init SHALL propose skip, update, or diff guidance instead of creating a duplicate file

#### Scenario: Sync still materializes after init

- **GIVEN** init has proposed or applied reviewed manifest config for a recipe
- **AND** the recipe is enabled in `ai-specs/ai-specs.toml`
- **WHEN** `ai-specs sync` runs
- **THEN** sync SHALL validate and materialize the enabled recipe according to the sync materialization contract
- **AND** sync SHALL NOT assume init previously ran
