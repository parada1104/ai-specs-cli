# recipe-conflict-resolution Specification

## Purpose

Define explicit error behavior when two or more recipes declare the same primitive ID.

## ADDED Requirements

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
Conflict detection SHALL apply only across `[recipes.*]` primitives. A recipe primitive conflicting with a user-created file in `ai-specs/skills/` or `ai-specs/commands/` SHALL NOT be treated as a recipe-recipe conflict; the recipe takes precedence with a warning.

#### Scenario: Recipe vs user-local skill
- **WHEN** a recipe provides a skill that already exists in `ai-specs/skills/`
- **THEN** sync SHALL emit a warning
- **AND** sync SHALL proceed with the recipe version
- **AND** sync SHALL NOT fail

### Requirement: Error message format
Conflict error messages SHALL include: the primitive type (skill, command, mcp), the conflicting ID, and the names of the conflicting recipes.

#### Scenario: Readable error
- **WHEN** a conflict occurs
- **THEN** the error message SHALL be actionable, e.g.: "recipe A and recipe B both declare mcp.id='openmemory'. Resolve manually in ai-specs.toml."
