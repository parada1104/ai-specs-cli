# recipe-schema (delta)

## MODIFIED Requirements

### Requirement: Primitive declarations in [provides]

The `[provides]` table SHALL support: `skills` (array of objects with `id` and
`source`), `commands` (array of objects with `id` and `path`), `mcp` (array of
tables with `id` and MCP fields), `templates` (array of tables with `source`,
`target`, and `condition`), `docs` (array of tables with `source` and `target`),
and `hooks` (array of tables declaring agent-runtime lifecycle hooks).

Each `[[provides.hooks]]` entry SHALL contain `id` (string, required, unique
within the recipe), `event` (string, required, one of the known abstract
events), and `script` (string, required, a path inside the recipe directory).
It MAY contain `matcher` (string), `blocking` (boolean, default `false`), and
`description` (string). Runtime hooks are declared ONLY in `recipe.toml`; they
are never declared in the project manifest.

#### Scenario: Skills with bundled source
- **WHEN** a skill is declared with `source = "bundled"`
- **THEN** validation SHALL pass and the skill SHALL resolve from the recipe's `skills/` directory

#### Scenario: Hooks declaration is valid
- **WHEN** `[[provides.hooks]]` contains `id`, a known `event`, and a `script` path inside the recipe directory
- **THEN** validation SHALL pass
- **AND** the hook SHALL be registered for runtime-hook distribution

#### Scenario: Hook with missing required field
- **GIVEN** a `[[provides.hooks]]` entry that omits `id`, `event`, or `script`
- **WHEN** the recipe is parsed
- **THEN** validation SHALL fail with an explicit error naming the missing field

#### Scenario: Hook with unknown event
- **GIVEN** a `[[provides.hooks]]` entry whose `event` is not a known abstract event
- **WHEN** the recipe is parsed
- **THEN** validation SHALL fail with an error listing the known events

#### Scenario: Hook script path escapes the recipe directory
- **GIVEN** a `[[provides.hooks]]` entry whose `script` resolves outside the recipe directory (absolute path or `../` traversal)
- **WHEN** the recipe is parsed
- **THEN** validation SHALL fail with an explicit path-escape error

#### Scenario: Recipe with no hooks
- **GIVEN** a `recipe.toml` with no `[[provides.hooks]]` tables
- **WHEN** the recipe is parsed
- **THEN** validation SHALL pass and no runtime hooks SHALL be registered
