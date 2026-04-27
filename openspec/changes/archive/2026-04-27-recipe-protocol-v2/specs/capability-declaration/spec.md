# capability-declaration Specification

## Purpose

Define how recipes declare semantic capabilities via the `[[capabilities]]` table in `recipe.toml`.

## Requirements

### Requirement: Capability table syntax

A recipe MAY declare zero or more capabilities using `[[capabilities]]` tables. Each table SHALL contain an `id` field whose value is a non-empty kebab-case string.

#### Scenario: Valid single capability declaration
- **GIVEN** a `recipe.toml` with `[[capabilities]]` and `id = "tracker"`
- **WHEN** the recipe is parsed
- **THEN** validation SHALL pass
- **AND** the capability SHALL be recorded for binding resolution

#### Scenario: Valid multiple capability declarations
- **GIVEN** a `recipe.toml` with two `[[capabilities]]` tables having ids `"tracker"` and `"canonical-memory"`
- **WHEN** the recipe is parsed
- **THEN** validation SHALL pass
- **AND** both capabilities SHALL be recorded

#### Scenario: Missing capability id
- **GIVEN** a `recipe.toml` with `[[capabilities]]` that omits the `id` field
- **WHEN** the recipe is parsed
- **THEN** validation SHALL fail with an explicit error naming the missing field

#### Scenario: Empty capability id
- **GIVEN** a `recipe.toml` with `[[capabilities]]` and `id = ""`
- **WHEN** the recipe is parsed
- **THEN** validation SHALL fail with an explicit error indicating an invalid capability id

#### Scenario: Recipe with no capabilities
- **GIVEN** a `recipe.toml` that contains no `[[capabilities]]` tables
- **WHEN** the recipe is parsed
- **THEN** validation SHALL pass
- **AND** the recipe SHALL be treated as having zero declared capabilities

#### Scenario: Duplicate capability ids within a recipe
- **GIVEN** a `recipe.toml` with two `[[capabilities]]` tables both having `id = "tracker"`
- **WHEN** the recipe is parsed
- **THEN** validation SHALL fail with an explicit error naming the duplicate capability id
