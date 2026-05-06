# recipe-config Specification (Delta)

## ADDED Requirements

### Requirement: Config field regex validation declaration
A config field MAY declare an optional `validation` table containing `regex` (string). The regex SHALL be treated as a format constraint for the merged config value.

#### Scenario: Valid regex validation declaration
- **GIVEN** a `recipe.toml` with `[config.board_id]` where `required = true`, `type = "string"`, and `validation.regex = "^[0-9a-fA-F]{24}$"`
- **WHEN** the recipe is parsed
- **THEN** validation SHALL pass
- **AND** the config schema SHALL record the regex pattern

#### Scenario: Invalid validation structure rejected
- **GIVEN** a `recipe.toml` with `[config.board_id]` where `validation` is not a table (e.g., `validation = "string"`)
- **WHEN** the recipe is parsed
- **THEN** validation SHALL fail with an explicit error naming the invalid validation structure

#### Scenario: Config field without validation parses successfully
- **GIVEN** a `recipe.toml` with `[config.timeout]` where `required = false`, `type = "integer"`, and no `validation` table
- **WHEN** the recipe is parsed
- **THEN** validation SHALL pass
- **AND** the config schema SHALL treat validation as absent

### Requirement: Sync enforces config field regex validation
During sync, the `validate-config` hook SHALL evaluate `validation.regex` against merged config values. If a value does not match the declared regex, sync SHALL fail with an explicit error.

#### Scenario: Sync passes when value matches regex
- **GIVEN** a recipe with `[config.board_id]` where `validation.regex = "^[0-9a-fA-F]{24}$"`
- **AND** the manifest declares `[recipes.my-recipe.config]` with `board_id = "69ec0a2099ea20956e371d62"`
- **WHEN** sync executes the `validate-config` hook
- **THEN** sync SHALL proceed without error

#### Scenario: Sync fails when value violates regex
- **GIVEN** a recipe with `[config.board_id]` where `validation.regex = "^[0-9a-fA-F]{24}$"`
- **AND** the manifest declares `[recipes.my-recipe.config]` with `board_id = "short123"`
- **WHEN** sync executes the `validate-config` hook
- **THEN** sync SHALL fail with an explicit error naming the field `board_id` and the expected pattern

#### Scenario: Sync ignores missing validation
- **GIVEN** a recipe with `[config.timeout]` where `required = false`, `type = "integer"`, and no `validation` table
- **AND** the manifest declares `[recipes.my-recipe.config]` with `timeout = 60`
- **WHEN** sync executes the `validate-config` hook
- **THEN** sync SHALL validate presence only and SHALL NOT fail
