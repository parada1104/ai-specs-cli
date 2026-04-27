# recipe-config Specification

## Purpose

Define how recipes declare configuration schemas and how per-project config values are merged during sync.

## Requirements

### Requirement: Config schema declaration in recipe.toml

A recipe MAY declare a configuration schema in a top-level `[config]` table. Each key in `[config]` SHALL represent a config field. Each field SHALL be declared as a table containing at minimum `required` (boolean). The field MAY also declare `type` (string) and `default` (any).

#### Scenario: Valid config schema with required field
- **GIVEN** a `recipe.toml` with `[config.board_id]` where `required = true` and `type = "string"`
- **WHEN** the recipe is parsed
- **THEN** validation SHALL pass
- **AND** the config schema SHALL be recorded

#### Scenario: Valid config schema with default value
- **GIVEN** a `recipe.toml` with `[config.timeout]` where `required = false`, `type = "integer"`, and `default = 30`
- **WHEN** the recipe is parsed
- **THEN** validation SHALL pass
- **AND** the default value SHALL be recorded

#### Scenario: Recipe with no config schema
- **GIVEN** a `recipe.toml` that contains no `[config]` table
- **WHEN** the recipe is parsed
- **THEN** validation SHALL pass
- **AND** the recipe SHALL be treated as having an empty config schema

### Requirement: Config values in manifest

A project MAY provide per-recipe config values in `[recipes.<id>.config]`. Sync SHALL merge these values with recipe defaults.

#### Scenario: Manifest overrides default
- **GIVEN** a recipe with `[config.timeout]` where `default = 30`
- **AND** the manifest declares `[recipes.my-recipe.config]` with `timeout = 60`
- **WHEN** sync merges config
- **THEN** the effective value for `timeout` SHALL be `60`

#### Scenario: Manifest provides required value
- **GIVEN** a recipe with `[config.board_id]` where `required = true`
- **AND** the manifest declares `[recipes.my-recipe.config]` with `board_id = "abc123"`
- **WHEN** sync merges config
- **THEN** the effective value for `board_id` SHALL be `"abc123"`
- **AND** sync SHALL proceed without error

#### Scenario: Missing required config value
- **GIVEN** a recipe with `[config.board_id]` where `required = true`
- **AND** the manifest does NOT provide `board_id` under `[recipes.my-recipe.config]`
- **WHEN** sync merges config
- **THEN** sync SHALL fail with an explicit error naming the missing required field

#### Scenario: Manifest config for recipe without schema
- **GIVEN** a recipe with no `[config]` table
- **AND** the manifest declares `[recipes.my-recipe.config]` with `foo = "bar"`
- **WHEN** sync merges config
- **THEN** sync SHALL emit a warning about unknown config fields
- **AND** sync SHALL NOT fail

#### Scenario: Config available to hooks
- **GIVEN** a recipe with `[config.board_id]` where `required = true`
- **AND** the manifest provides `board_id` under `[recipes.my-recipe.config]`
- **AND** the recipe declares an `on-sync` hook with `action = "validate-config"`
- **WHEN** sync executes the hook
- **THEN** the merged config values SHALL be accessible to the hook context
