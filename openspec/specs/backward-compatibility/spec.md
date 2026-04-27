# backward-compatibility Specification

## Purpose

Ensure that recipe.toml V1 and ai-specs.toml V1 continue to function unchanged after the introduction of recipe protocol V2.

## Requirements

### Requirement: V1 recipe.toml compatibility

A `recipe.toml` that omits all V2 tables (`[[capabilities]]`, `[[hooks]]`, `[config]`) MUST be parsed and materialized exactly as before.

#### Scenario: V1 recipe without V2 tables
- **GIVEN** a `recipe.toml` containing only `[recipe]` and `[provides]` tables
- **WHEN** the recipe is parsed
- **THEN** validation SHALL pass
- **AND** the recipe SHALL materialize all declared primitives
- **AND** no V2-specific behavior SHALL be triggered

#### Scenario: V1 recipe with only primitives
- **GIVEN** a V1 recipe declaring skills, commands, templates, docs, and MCP presets
- **WHEN** sync materializes the recipe
- **THEN** all primitives SHALL be copied in the correct order
- **AND** version pinning SHALL be enforced
- **AND** primitive conflict detection SHALL continue to work

### Requirement: V1 manifest compatibility

An `ai-specs.toml` that omits all V2 additions (`[[bindings]]`, `[recipes.<id>.config]`) MUST continue to parse and process correctly.

#### Scenario: V1 manifest without bindings or recipe config
- **GIVEN** an `ai-specs.toml` with `[recipes.my-recipe]` containing only `enabled` and `version`
- **WHEN** sync reads the manifest
- **THEN** the recipe SHALL be recognized as enabled
- **AND** sync SHALL proceed without error
- **AND** no binding resolution SHALL be attempted for that recipe

#### Scenario: Manifest with no recipes section
- **GIVEN** an `ai-specs.toml` with no `[recipes.*]` tables at all
- **WHEN** sync runs
- **THEN** sync SHALL proceed normally
- **AND** no recipe-related behavior SHALL be triggered

### Requirement: V1 conflict detection preserved

The existing primitive conflict detection for skills, commands, and MCP presets across recipes MUST continue to work unchanged.

#### Scenario: V1 skill conflict still detected
- **GIVEN** two enabled V1 recipes both declare a bundled skill with `id = "same-skill"`
- **WHEN** sync runs conflict detection
- **THEN** sync SHALL fail with an explicit error naming the conflicting skill and both recipes

#### Scenario: V1 command conflict still detected
- **GIVEN** two enabled V1 recipes both declare a command with `id = "same-cmd"`
- **WHEN** sync runs conflict detection
- **THEN** sync SHALL fail with an explicit error naming the conflicting command and both recipes

### Requirement: V2 additions are opt-in

No V2 table SHALL be required in either `recipe.toml` or `ai-specs.toml`. Their absence MUST NOT alter V1 behavior.

#### Scenario: Mixed V1 and V2 recipes in same manifest
- **GIVEN** one V1 recipe with no V2 tables
- **AND** one V2 recipe with `[[capabilities]]` and `[config]`
- **AND** both are enabled in the manifest
- **WHEN** sync processes both recipes
- **THEN** the V1 recipe SHALL materialize exactly as before
- **AND** the V2 recipe SHALL materialize with its capabilities, config, and hooks
- **AND** neither recipe's behavior SHALL interfere with the other
