# Spec Delta: config-wizard-help

## ADDED Requirements

### Requirement: MCP env-var prompting uses valid questionary API

When `ai-specs configure-recipes` (or recipe-add / init) prompts for MCP environment
variables collected from enabled recipes, secret vars SHALL be prompted with
`questionary.password` (or `questionary.text` with `is_password=True`). The prompt
SHALL NOT pass a `password=` keyword argument to `questionary.text`.

#### Scenario: Secret env var prompt does not raise TypeError
- **GIVEN** an enabled recipe that requires `TRELLO_API_KEY` / `TRELLO_TOKEN`
- **WHEN** the env-var prompt constructs the secret input question
- **THEN** construction SHALL succeed without `TypeError`
- **AND** the input SHALL be masked

#### Scenario: configure-recipes soft-fails on env prompt errors
- **GIVEN** `prompt_env_vars` raises an unexpected exception
- **WHEN** `_offer_envrc` runs after recipe config
- **THEN** configure-recipes SHALL print a non-fatal warning and continue
- **AND** SHALL NOT abort the process with an unhandled traceback

### Requirement: Curated how-to-get help for known MCP env vars

`envrc-scaffold` SHALL maintain a curated help map for known MCP env var names.
When prompting or generating `.envrc.example`, known vars SHALL include how-to-get
guidance (with a URL when applicable). Unknown vars SHALL keep the existing purpose
string (`required by <mcp> (<recipe>)`).

#### Scenario: Trello env vars include help links in .envrc.example
- **GIVEN** an enabled `trello-mcp-workflow` recipe
- **WHEN** `.envrc.example` is generated
- **THEN** the `TRELLO_API_KEY` and `TRELLO_TOKEN` lines SHALL include help text
  referencing https://trello.com/power-ups/admin

### Requirement: Config type alias boolean normalizes to bool

When a recipe config field declares `type = "boolean"`, the schema parser SHALL
normalize the stored type to `"bool"` so the config wizard uses a confirm prompt.

#### Scenario: Catalog-style boolean field parses as bool
- **GIVEN** a `recipe.toml` with `[config.auto_switch_account]` where
  `required = false`, `type = "boolean"`, `default = false`
- **WHEN** the recipe is parsed
- **THEN** `config_schema.fields["auto_switch_account"].type` SHALL equal `"bool"`

### Requirement: Catalog config fields expose help_text for the wizard

Catalog recipes that declare ConfigFields SHALL include `help_text` describing
what the field means and, where useful, where to obtain the value (with a link).
The config wizard SHALL display `help_text` before prompting (existing behavior).

#### Scenario: board_id and integration_branch have help_text
- **GIVEN** the catalog recipes `trello-mcp-workflow` and `worktree-flow`
- **WHEN** their `recipe.toml` config tables are read
- **THEN** `board_id` and `integration_branch` SHALL each have non-empty `help_text`
