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

### Requirement: Optional recipe init declaration

A recipe MAY declare an initialization workflow in `recipe.toml` using a top-level `[init]` table. The `[init]` table SHALL be optional, and recipes without `[init]` SHALL remain valid and SHALL preserve their existing parse, add, and sync behavior.

The `[init]` table SHALL support `prompt` (string, required when `[init]` exists), `description` (string, optional), `needs_manifest` (boolean, optional), and `needs_mcp` (array of strings, optional). Additional init fields SHALL be rejected unless a later spec explicitly defines them.

#### Scenario: Recipe without init declaration

- **GIVEN** a valid recipe `recipe.toml` without `[init]`
- **WHEN** the recipe is parsed
- **THEN** validation SHALL pass
- **AND** the recipe SHALL be treated as having no init workflow
- **AND** existing recipe add and sync behavior SHALL be unchanged

#### Scenario: Recipe with valid init declaration

- **GIVEN** a valid recipe `recipe.toml` with `[init]`
- **AND** `[init]` declares `prompt = "docs/init.md"`
- **AND** `docs/init.md` exists under the recipe directory
- **WHEN** the recipe is parsed
- **THEN** validation SHALL pass
- **AND** the parsed recipe metadata SHALL include the init prompt path and optional init fields

#### Scenario: Init declaration without prompt

- **GIVEN** a valid recipe `recipe.toml` with `[init]`
- **AND** `[init]` does not declare `prompt`
- **WHEN** the recipe is parsed
- **THEN** validation SHALL fail with an explicit error naming `[init].prompt`

#### Scenario: Unknown init field

- **GIVEN** a valid recipe `recipe.toml` with `[init]`
- **AND** `[init]` declares an unsupported field
- **WHEN** the recipe is parsed
- **THEN** validation SHALL fail with an explicit error naming the unsupported init field

### Requirement: Init prompt path validation

The init `prompt` value SHALL be a relative path inside the recipe directory. The parser SHALL reject absolute paths, parent-directory traversal, empty paths, directory paths, and paths that do not exist.

#### Scenario: Prompt path inside recipe directory

- **GIVEN** `[init]` declares `prompt = "docs/init.md"`
- **AND** `catalog/recipes/example/docs/init.md` exists as a file
- **WHEN** the recipe is parsed
- **THEN** validation SHALL pass

#### Scenario: Absolute prompt path

- **GIVEN** `[init]` declares `prompt = "/tmp/init.md"`
- **WHEN** the recipe is parsed
- **THEN** validation SHALL fail with an explicit error that init prompt paths MUST be relative to the recipe directory

#### Scenario: Prompt path escapes recipe directory

- **GIVEN** `[init]` declares `prompt = "../init.md"`
- **WHEN** the recipe is parsed
- **THEN** validation SHALL fail with an explicit error that init prompt paths MUST stay inside the recipe directory

#### Scenario: Missing prompt file

- **GIVEN** `[init]` declares `prompt = "docs/missing.md"`
- **AND** the file does not exist under the recipe directory
- **WHEN** the recipe is parsed
- **THEN** validation SHALL fail with an explicit error naming the missing init prompt file
