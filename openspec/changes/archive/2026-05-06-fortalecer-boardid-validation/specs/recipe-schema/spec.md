# recipe-schema Specification (Delta)

## ADDED Requirements

### Requirement: ConfigField supports optional validation attribute
The `ConfigField` dataclass SHALL support an optional `validation` attribute. When present, it SHALL contain a `regex` string representing a format constraint.

#### Scenario: ConfigField parsed with validation
- **GIVEN** a `recipe.toml` with `[config.board_id]` where `required = true`, `type = "string"`, and `validation.regex = "^[0-9a-fA-F]{24}$"`
- **WHEN** `recipe_schema.py` parses the recipe
- **THEN** the resulting `ConfigField` SHALL have `validation` set to a dict containing `{"regex": "^[0-9a-fA-F]{24}$"}`

#### Scenario: ConfigField parsed without validation
- **GIVEN** a `recipe.toml` with `[config.timeout]` where `required = false`, `type = "integer"`, and no `validation` table
- **WHEN** `recipe_schema.py` parses the recipe
- **THEN** the resulting `ConfigField` SHALL have `validation` set to `None`

#### Scenario: ConfigField rejects malformed validation table
- **GIVEN** a `recipe.toml` with `[config.board_id]` where `validation` is a table but contains an unknown key (e.g., `validation.min = 5`)
- **WHEN** `recipe_schema.py` parses the recipe
- **THEN** validation SHALL fail with an explicit error naming the unsupported validation key
