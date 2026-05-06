# agents-md-runtime-brief Specification (Delta)

## ADDED Requirements

### Requirement: Recipe config fields rendered in runtime brief
When `ai-specs.toml` declares enabled recipes with non-empty config schemas, the generated `AGENTS.md` SHALL include a per-recipe subsection listing each config field with its `required`, `type`, `default`, and `validation` attributes.

#### Scenario: Enabled recipe with config fields
- **GIVEN** an enabled recipe with `[config.board_id]` where `required = true`, `type = "string"`, and `validation.regex = "^[0-9a-fA-F]{24}$"`
- **AND** `[config.default_list]` where `required = false`, `type = "string"`, `default = "In Progress"`
- **WHEN** `ai-specs sync` generates `AGENTS.md`
- **THEN** the runtime brief SHALL contain a config fields table for that recipe
- **AND** the table SHALL list `board_id` with `required`, `type`, and `validation`
- **AND** the table SHALL list `default_list` with `required`, `type`, and `default`

#### Scenario: Recipe without config schema omits subsection
- **GIVEN** an enabled recipe with no `[config]` table
- **WHEN** `ai-specs sync` generates `AGENTS.md`
- **THEN** the runtime brief SHALL NOT contain a config fields subsection for that recipe

#### Scenario: Config field with regex validation shows pattern
- **GIVEN** an enabled recipe with `[config.board_id]` where `validation.regex = "^[0-9a-fA-F]{24}$"`
- **WHEN** `ai-specs sync` generates `AGENTS.md`
- **THEN** the runtime brief SHALL display the regex pattern next to the field name
