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
