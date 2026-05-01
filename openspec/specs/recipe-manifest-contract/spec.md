# recipe-manifest-contract Specification

## Purpose

Define how recipes are declared in a project's `ai-specs.toml` manifest.

## ADDED Requirements

### Requirement: Recipe instance declaration
A project MAY declare an installed recipe using a top-level `[recipes.<id>]` table. The table SHALL contain `enabled` (boolean, required) and `version` (string, required). The `id` MUST match a recipe in `catalog/recipes/`.

#### Scenario: Recipe enabled and pinned
- **WHEN** `[recipes.runtime-memory-openmemory]` declares `enabled = true` and `version = "1.0.0"`
- **THEN** sync SHALL validate the recipe exists in the catalog
- **AND** sync SHALL validate the version matches `recipe.toml`
- **AND** sync SHALL materialize the recipe

#### Scenario: Recipe disabled
- **WHEN** `[recipes.runtime-memory-openmemory]` declares `enabled = false`
- **THEN** sync SHALL skip materialization for this recipe
- **AND** sync SHALL NOT fail

#### Scenario: Version mismatch
- **WHEN** manifest pins `version = "1.0.0"` but catalog has `version = "1.1.0"`
- **THEN** sync SHALL fail with an explicit version-mismatch error

#### Scenario: Unknown recipe ID
- **WHEN** manifest declares `[recipes.unknown-id]`
- **THEN** sync SHALL fail with an explicit "recipe not found" error

### Requirement: Backward compatibility
The absence of any `[recipes.*]` section SHALL NOT cause validation or sync to fail.

#### Scenario: Manifest without recipes
- **WHEN** `ai-specs.toml` contains no `[recipes.*]` tables
- **THEN** sync SHALL proceed normally
- **AND** no recipe-related behavior SHALL be triggered

### Requirement: Durable recipe init output

Recipe initialization output that is intended to survive sync SHALL be stored in the existing manifest section responsible for that data. Per-recipe values SHALL be stored under `[recipes.<id>.config]` unless another manifest section is explicitly responsible for the value.

#### Scenario: Init stores per-recipe config

- **GIVEN** recipe `tracker` declares config field `board_id`
- **WHEN** init proposes durable storage for the selected board
- **THEN** the proposed manifest delta SHALL target `[recipes.tracker.config]`
- **AND** the proposed key SHALL be `board_id`

#### Scenario: Init stores existing manifest responsibility elsewhere

- **GIVEN** init discovers an MCP server that belongs in `[mcp.trello]`
- **WHEN** init proposes durable storage for MCP declaration data
- **THEN** the proposal MAY target `[mcp.trello]`
- **AND** it SHALL NOT duplicate that MCP declaration under `[recipes.tracker.config]` unless the recipe config schema explicitly requires a separate reference value

### Requirement: Init manifest deltas avoid duplicate declarations

When init proposes updates to `ai-specs/ai-specs.toml`, it SHALL update existing `[recipes.<id>]` and `[recipes.<id>.config]` declarations instead of appending duplicate tables or duplicate keys.

#### Scenario: Existing recipe table updated

- **GIVEN** the manifest already contains `[recipes.tracker]`
- **WHEN** init proposes changing `enabled` or `version`
- **THEN** the proposed manifest delta SHALL update the existing `[recipes.tracker]` table
- **AND** it SHALL NOT add a second `[recipes.tracker]` table

#### Scenario: Existing config key updated

- **GIVEN** the manifest already contains `[recipes.tracker.config]` with `board_id = "old"`
- **WHEN** init proposes `board_id = "new"`
- **THEN** the proposed manifest delta SHALL update the existing `board_id` key
- **AND** it SHALL NOT append a duplicate `board_id` key

#### Scenario: Missing config table added once

- **GIVEN** the manifest contains `[recipes.tracker]`
- **AND** it does not contain `[recipes.tracker.config]`
- **WHEN** init proposes per-recipe config values
- **THEN** the proposed manifest delta SHALL add exactly one `[recipes.tracker.config]` table
