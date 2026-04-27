# recipe-sync-materialization Specification

## Purpose

Define how `ai-specs sync` resolves, validates, and materializes recipes into the project workspace.

## ADDED Requirements

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
The system SHALL materialize primitives in this order: skills (bundled then deps), commands, MCP presets, templates, docs.

#### Scenario: Full materialization
- **WHEN** a valid recipe with all primitive types is processed
- **THEN** sync SHALL create all expected files in the project workspace
- **AND** derived artifacts (AGENTS.md, agent configs) SHALL reflect new skills and commands

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
