## MODIFIED Requirements

### Requirement: Config values in manifest

A project MAY provide per-recipe config values in `[recipes.<id>.config]`. Sync SHALL merge these values with recipe defaults. Init workflows MAY propose values for `[recipes.<id>.config]`, but proposed values SHOULD align with the recipe `[config]` schema and SHALL NOT bypass sync-time required-field validation.

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

#### Scenario: Init proposes schema-aligned config

- **GIVEN** a recipe with `[config.board_id]` where `required = true` and `type = "string"`
- **WHEN** `ai-specs recipe init my-recipe` proposes durable manifest changes
- **THEN** the proposal SHOULD place `board_id` under `[recipes.my-recipe.config]`
- **AND** the proposed value SHOULD be compatible with the recipe config schema

#### Scenario: Init does not satisfy sync by assertion

- **GIVEN** a recipe with required config field `board_id`
- **AND** `ai-specs recipe init my-recipe` proposes `board_id = "abc123"`
- **WHEN** sync later validates the recipe
- **THEN** sync SHALL independently validate the effective config
- **AND** sync SHALL fail if the committed manifest still omits the required field

#### Scenario: Init surfaces unknown config proposal

- **GIVEN** a recipe whose `[config]` schema does not declare `unknown_key`
- **WHEN** `ai-specs recipe init my-recipe` would propose `unknown_key` under `[recipes.my-recipe.config]`
- **THEN** the init brief SHALL warn or reject the out-of-schema key
- **AND** the behavior SHALL NOT weaken sync-time unknown-config warnings
