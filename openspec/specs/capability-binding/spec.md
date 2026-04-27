# capability-binding Specification

## Purpose

Define how projects explicitly bind semantic capabilities to concrete recipes via `[[bindings]]` in `ai-specs.toml`.

## Requirements

### Requirement: Binding table syntax

A project MAY declare capability bindings using top-level `[[bindings]]` tables. Each binding SHALL contain a `capability` field and a `recipe` field.

#### Scenario: Valid explicit binding
- **GIVEN** an `ai-specs.toml` with `[[bindings]]` where `capability = "tracker"` and `recipe = "trello-pm"`
- **AND** the recipe `"trello-pm"` is enabled and declares the `"tracker"` capability
- **WHEN** sync resolves bindings
- **THEN** the binding SHALL be accepted as valid

#### Scenario: Binding references disabled recipe
- **GIVEN** an `ai-specs.toml` with `[[bindings]]` where `recipe = "trello-pm"`
- **AND** the recipe `"trello-pm"` is declared but `enabled = false`
- **WHEN** sync resolves bindings
- **THEN** sync SHALL fail with an explicit error stating the bound recipe is disabled

#### Scenario: Binding references recipe without capability
- **GIVEN** an `ai-specs.toml` with `[[bindings]]` where `capability = "tracker"` and `recipe = "trello-pm"`
- **AND** the recipe `"trello-pm"` is enabled but does NOT declare the `"tracker"` capability
- **WHEN** sync resolves bindings
- **THEN** sync SHALL fail with an explicit error stating the recipe does not provide the capability

#### Scenario: Binding references unknown recipe
- **GIVEN** an `ai-specs.toml` with `[[bindings]]` where `recipe = "unknown-recipe"`
- **AND** no recipe with that id exists in the catalog
- **WHEN** sync resolves bindings
- **THEN** sync SHALL fail with an explicit error stating the recipe is not found

#### Scenario: Multiple bindings for same capability
- **GIVEN** an `ai-specs.toml` with two `[[bindings]]` tables both having `capability = "tracker"` but different `recipe` values
- **WHEN** sync resolves bindings
- **THEN** sync SHALL fail with an explicit error stating duplicate capability bindings are not allowed

#### Scenario: No bindings declared
- **GIVEN** an `ai-specs.toml` with no `[[bindings]]` tables
- **WHEN** sync resolves bindings
- **THEN** sync SHALL proceed without error
- **AND** auto-binding SHALL be evaluated for unbound capabilities
