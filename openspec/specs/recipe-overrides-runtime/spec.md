# recipe-overrides-runtime Specification

## Purpose

Define the runtime override loading contract for recipe-bundled skills, allowing projects to customize recipe-provided behavior without modifying cache-staged origin files.

## Requirements

### Requirement: Override config loading

For each enabled recipe, the system SHALL check for `ai-specs/recipes/{recipe-id}/overrides/config.toml`. If present, override values SHALL take precedence over bundled defaults at runtime.

#### Scenario: Override config exists

- **GIVEN** recipe `my-recipe` is enabled
- **AND** `ai-specs/recipes/my-recipe/overrides/config.toml` exists
- **WHEN** the bundled skill reads its configuration
- **THEN** override values take precedence over bundled defaults

#### Scenario: Override config missing

- **GIVEN** an enabled recipe without `ai-specs/recipes/{recipe-id}/overrides/config.toml`
- **WHEN** the bundled skill reads its configuration
- **THEN** bundled defaults are used without error

### Requirement: Override template loading

For each enabled recipe, the system SHALL check for `ai-specs/recipes/{recipe-id}/overrides/templates/`. Templates in this directory SHALL override identically named bundled templates.

#### Scenario: Override template exists

- **GIVEN** `ai-specs/recipes/my-recipe/overrides/templates/custom.md` exists
- **AND** the bundled skill provides `custom.md`
- **WHEN** the skill renders the template
- **THEN** the override version is used

### Requirement: Overrides apply only to their parent recipe

Override files under `ai-specs/recipes/{recipe-id}/overrides/` SHALL affect only skills bundled by that recipe.

#### Scenario: Override isolation between recipes

- **GIVEN** overrides under `ai-specs/recipes/recipe-a/overrides/`
- **WHEN** a skill runs under `recipe-b`
- **THEN** overrides from `recipe-a` do not apply

### Requirement: Override migration from legacy origin

When leftover in-project `.recipe/{recipe-id}/overrides/` exists and `ai-specs/recipes/{recipe-id}/overrides/` is absent, sync SHALL migrate overrides to the in-project recipes path before deleting legacy origin trees.

#### Scenario: Migrate before leftover rm

- **GIVEN** leftover `ai-specs/.recipe/my-recipe/overrides/`
- **AND** `ai-specs/recipes/my-recipe/overrides/` does not exist
- **WHEN** sync runs
- **THEN** overrides are migrated to `ai-specs/recipes/my-recipe/overrides/`
- **AND** legacy in-project origin trees are removed afterward
