# recipe-schema Specification

## Purpose

Define the canonical file format and directory layout for a recipe package.

## ADDED Requirements

### Requirement: Recipe package layout
A recipe SHALL be declared in a directory `catalog/recipes/<id>/` containing at minimum a `recipe.toml` file. The directory MAY contain `skills/`, `commands/`, `templates/`, and `docs/` subdirectories.

#### Scenario: Minimal valid recipe
- **WHEN** a recipe directory contains only `recipe.toml`
- **THEN** the recipe SHALL be considered valid
- **AND** sync SHALL process it without error

#### Scenario: Recipe with bundled assets
- **WHEN** a recipe directory contains `recipe.toml`, `skills/`, `commands/`, `templates/`, and `docs/`
- **THEN** sync SHALL materialize all declared primitives

### Requirement: recipe.toml schema
The file `recipe.toml` SHALL contain a `[recipe]` table with fields: `id` (string, required), `name` (string, required), `description` (string, required), `version` (string, required), `author` (string, optional), `license` (string, optional). It SHALL contain a `[provides]` table declaring primitives.

#### Scenario: Valid recipe.toml
- **WHEN** `recipe.toml` contains all required `[recipe]` fields and a valid `[provides]` table
- **THEN** validation SHALL pass

#### Scenario: Missing required field
- **WHEN** `recipe.toml` omits `id`, `name`, `description`, or `version`
- **THEN** validation SHALL fail with an explicit error

### Requirement: Primitive declarations in [provides]
The `[provides]` table SHALL support: `skills` (array of objects with `id` and `source`), `commands` (array of objects with `id` and `path`), `mcp` (array of tables with `id` and MCP fields), `templates` (array of tables with `source`, `target`, and `condition`), and `docs` (array of tables with `source` and `target`).

#### Scenario: Skills with bundled source
- **WHEN** a skill declares `source = "bundled"`
- **THEN** sync SHALL copy the skill from `catalog/recipes/<id>/skills/<skill-id>/`

#### Scenario: Skills with dep source
- **WHEN** a skill declares `source = "dep"`, `url` (string, required), and optionally `path` (string)
- **THEN** sync SHALL vendor the skill using the standard `[[deps]]` flow
- **AND** the skill SHALL be installed into `ai-specs/skills/<skill-id>/`

#### Scenario: Commands declaration
- **WHEN** a command is declared with `id` and `path`
- **THEN** sync SHALL copy the file at `path` into `ai-specs/commands/<id>.md`
- **AND** `path` SHALL be relative to the recipe directory

#### Scenario: MCP preset declaration
- **WHEN** an MCP preset is declared under `[[provides.mcp]]`
- **THEN** sync SHALL merge it into derived MCP configs

#### Scenario: Template with not_exists condition
- **WHEN** a template declares `condition = "not_exists"`, `source` (relative to recipe directory), and `target` (relative to project root)
- **THEN** sync SHALL copy it only if `target` does not already exist

#### Scenario: Docs declaration
- **WHEN** a doc is declared with `source` (relative to recipe directory) and `target` (relative to project root)
- **THEN** sync SHALL copy `source` to `target`
