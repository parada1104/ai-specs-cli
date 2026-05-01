# recipe-conflict-resolution Specification

## Purpose

Define explicit error behavior when two or more recipes declare the same primitive ID, and precedence rules when the same skill ID exists across local, recipe, and dependency sources.

## Requirements

### Requirement: Primitive ID uniqueness across recipes
During sync, the system SHALL maintain a registry of claimed primitive IDs. If a second recipe attempts to claim an ID already registered, sync SHALL fail with an explicit error message.

#### Scenario: Conflicting skills
- **WHEN** recipe A declares `skill.id = "openmemory-proactive"` and recipe B also declares `skill.id = "openmemory-proactive"`
- **THEN** sync SHALL fail
- **AND** the error SHALL name both recipes and the conflicting primitive ID

#### Scenario: Conflicting commands
- **WHEN** recipe A declares `command.id = "handoff"` and recipe B also declares `command.id = "handoff"`
- **THEN** sync SHALL fail with an explicit conflict error

#### Scenario: Conflicting MCP presets
- **WHEN** recipe A declares `mcp.id = "openmemory"` and recipe B also declares `mcp.id = "openmemory"`
- **THEN** sync SHALL fail with an explicit conflict error

### Requirement: Conflict scope
Conflict detection SHALL apply across `[recipes.*]` primitives (recipe-recipe conflicts). A local skill in `ai-specs/skills/{id}/` SHALL always take precedence over a recipe-bundled or dependency skill with the same ID, without being treated as an error and without emitting a warning.

#### Scenario: Local skill overrides recipe skill
- **WHEN** `ai-specs/skills/my-skill/` exists
- **AND** a recipe also provides `my-skill`
- **THEN** sync SHALL NOT fail
- **AND** sync SHALL NOT emit a warning
- **AND** the local version in `ai-specs/skills/my-skill/` SHALL be used

#### Scenario: Local skill overrides dependency skill
- **WHEN** `ai-specs/skills/my-skill/` exists
- **AND** a dependency also provides `my-skill`
- **THEN** sync SHALL NOT fail
- **AND** sync SHALL NOT emit a warning
- **AND** the local version in `ai-specs/skills/my-skill/` SHALL be used

#### Scenario: Recipe vs user-local command
- **WHEN** a recipe provides a command that already exists as a user-created file in `ai-specs/commands/`
- **THEN** sync SHALL emit a warning
- **AND** sync SHALL proceed with the recipe version
- **AND** sync SHALL NOT fail

### Requirement: Multi-source skill precedence
When the same skill ID exists in multiple sources, the system SHALL resolve it using source precedence: local (`ai-specs/skills/`) > recipe (`.recipe/{recipe-id}/skills/`) > dependency (`.deps/{dep-id}/skills/`). This is not a conflict error; it is a deterministic resolution rule.

#### Scenario: Same skill in local and recipe
- **WHEN** `ai-specs/skills/shared-skill/` exists
- **AND** `.recipe/my-recipe/skills/shared-skill/` also exists
- **THEN** the system SHALL use the local version
- **AND** no conflict error SHALL be raised

#### Scenario: Same skill in recipe and dep
- **WHEN** `.recipe/my-recipe/skills/shared-skill/` exists
- **AND** `.deps/my-dep/skills/shared-skill/` also exists
- **THEN** the system SHALL use the recipe version
- **AND** no conflict error SHALL be raised

#### Scenario: Same skill in local, recipe, and dep
- **WHEN** the same skill ID exists in all three sources
- **THEN** the system SHALL use the local version
- **AND** no conflict error SHALL be raised

### Requirement: Recipe-recipe primitive conflicts still fail
If two or more recipes declare the same primitive ID (skill, command, or MCP preset) at the recipe level, sync SHALL fail with an explicit error, regardless of whether those primitives would be materialized to different external directories.

#### Scenario: Conflicting skills across recipes
- **WHEN** recipe A declares `skill.id = "openmemory-proactive"` and recipe B also declares `skill.id = "openmemory-proactive"`
- **THEN** sync SHALL fail
- **AND** the error SHALL name both recipes and the conflicting primitive ID

#### Scenario: Conflicting commands across recipes
- **WHEN** recipe A declares `command.id = "handoff"` and recipe B also declares `command.id = "handoff"`
- **THEN** sync SHALL fail with an explicit conflict error

#### Scenario: Conflicting MCP presets across recipes
- **WHEN** recipe A declares `mcp.id = "openmemory"` and recipe B also declares `mcp.id = "openmemory"`
- **THEN** sync SHALL fail with an explicit conflict error

### Requirement: Error message format
Conflict error messages SHALL include: the primitive type (skill, command, mcp), the conflicting ID, and the names of the conflicting recipes.

#### Scenario: Readable error
- **WHEN** a conflict occurs
- **THEN** the error message SHALL be actionable, e.g.: "recipe A and recipe B both declare mcp.id='openmemory'. Resolve manually in ai-specs.toml."
