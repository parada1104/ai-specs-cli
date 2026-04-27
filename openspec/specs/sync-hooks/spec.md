# sync-hooks Specification

## Purpose

Define how recipes declare sync-time lifecycle hooks and how the sync pipeline executes them.

## Requirements

### Requirement: Hook declaration in recipe.toml

A recipe MAY declare sync-time hooks using `[[hooks]]` tables. Each hook SHALL contain an `event` field and an `action` field. Both SHALL be non-empty strings.

#### Scenario: Valid hook declaration
- **GIVEN** a `recipe.toml` with `[[hooks]]` where `event = "on-sync"` and `action = "validate-config"`
- **WHEN** the recipe is parsed
- **THEN** validation SHALL pass
- **AND** the hook SHALL be registered for execution

#### Scenario: Missing hook event
- **GIVEN** a `recipe.toml` with `[[hooks]]` that omits the `event` field
- **WHEN** the recipe is parsed
- **THEN** validation SHALL fail with an explicit error naming the missing field

#### Scenario: Missing hook action
- **GIVEN** a `recipe.toml` with `[[hooks]]` that omits the `action` field
- **WHEN** the recipe is parsed
- **THEN** validation SHALL fail with an explicit error naming the missing field

#### Scenario: Recipe with no hooks
- **GIVEN** a `recipe.toml` that contains no `[[hooks]]` tables
- **WHEN** the recipe is parsed
- **THEN** validation SHALL pass
- **AND** no hook execution SHALL be scheduled

### Requirement: Hook execution during sync

Hooks SHALL execute after all primitives (skills, commands, MCP presets, templates, docs) have been materialized. Hook failures SHALL cause sync to fail.

#### Scenario: Successful hook execution
- **GIVEN** a recipe declares an `on-sync` hook with `action = "validate-config"`
- **AND** all required config values are present and valid
- **WHEN** sync executes the hook after materialization
- **THEN** the hook SHALL complete successfully
- **AND** sync SHALL continue to the next recipe

#### Scenario: Hook validation failure
- **GIVEN** a recipe declares an `on-sync` hook with `action = "validate-config"`
- **AND** a required config value is missing or invalid
- **WHEN** sync executes the hook after materialization
- **THEN** the hook SHALL raise an error
- **AND** sync SHALL fail with an explicit error naming the recipe and the failed hook action

#### Scenario: Hooks execute in declaration order
- **GIVEN** a recipe declares two `[[hooks]]` tables: first `validate-config`, then `ensure-directories`
- **WHEN** sync executes hooks for this recipe
- **THEN** `validate-config` SHALL execute before `ensure-directories`

#### Scenario: Unsupported hook event or action
- **GIVEN** a recipe declares a hook with `event = "on-sync"` and `action = "unknown-action"`
- **WHEN** sync encounters this hook
- **THEN** sync SHALL emit a warning naming the unsupported action
- **AND** sync SHALL skip the hook without failing
