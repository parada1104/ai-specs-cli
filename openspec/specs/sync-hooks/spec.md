# sync-hooks Specification

## Purpose

Define how recipes declare sync-time lifecycle hooks and how the sync pipeline executes them.

## Requirements

### Requirement: Hook declaration in recipe.toml

A recipe MAY declare sync-time hooks using `[[hooks]]` tables. Each hook SHALL contain an `event` field and an `action` field. Both SHALL be non-empty strings.

#### Scenario: Valid hook declaration
- **GIVEN** a `recipe.toml` with `[[hooks]]` where `event = "on-sync"` and `action = "validate-config"`
- **WHEN** the recipe is parsed
- **THEN** validation SHALL pass
- **AND** the hook SHALL be registered for execution

#### Scenario: Missing hook event
- **GIVEN** a `recipe.toml` with `[[hooks]]` that omits the `event` field
- **WHEN** the recipe is parsed
- **THEN** validation SHALL fail with an explicit error naming the missing field

#### Scenario: Missing hook action
- **GIVEN** a `recipe.toml` with `[[hooks]]` that omits the `action` field
- **WHEN** the recipe is parsed
- **THEN** validation SHALL fail with an explicit error naming the missing field

#### Scenario: Recipe with no hooks
- **GIVEN** a `recipe.toml` that contains no `[[hooks]]` tables
- **WHEN** the recipe is parsed
- **THEN** validation SHALL pass
- **AND** no hook execution SHALL be scheduled

### Requirement: Hook execution during sync

Hooks SHALL execute after all primitives (skills, commands, MCP presets, templates, docs) have been materialized. Hook failures SHALL cause sync to fail. Sync SHALL recognize the following hook actions: `validate-config`, `bootstrap-board`, `link-trello-card`, `sync-card-state`, `comment-verification`. Recipes MAY declare additional hook actions; unrecognized actions SHALL emit a warning and be skipped.

#### Scenario: Successful hook execution
- **GIVEN** a recipe declares an `on-sync` hook with `action = "validate-config"`
- **AND** all required config values are present and valid
- **WHEN** sync executes the hook after materialization
- **THEN** the hook SHALL complete successfully
- **AND** sync SHALL continue to the next recipe

#### Scenario: Hook validation failure
- **GIVEN** a recipe declares an `on-sync` hook with `action = "validate-config"`
- **AND** a required config value is missing or invalid
- **WHEN** sync executes the hook after materialization
- **THEN** the hook SHALL raise an error
- **AND** sync SHALL fail with an explicit error naming the recipe and the failed hook action

#### Scenario: Hooks execute in declaration order
- **GIVEN** a recipe declares two `[[hooks]]` tables: first `validate-config`, then `bootstrap-board`
- **WHEN** sync executes hooks for this recipe
- **THEN** `validate-config` SHALL execute before `bootstrap-board`

#### Scenario: Bootstrap-board hook creates marker file
- **GIVEN** a recipe declares an `on-sync` hook with `action = "bootstrap-board"`
- **AND** the `board_id` config value is present
- **WHEN** sync executes the hook after materialization
- **THEN** the hook SHALL create directory `.recipe/<recipe-id>/` under the project root if it does not exist
- **AND** write a file `.recipe/<recipe-id>/bootstrap-ready` containing the board_id, default_list, and epic_list config values

#### Scenario: Bootstrap-board hook fails on missing board_id
- **GIVEN** a recipe declares an `on-sync` hook with `action = "bootstrap-board"`
- **AND** the `board_id` config value is missing
- **WHEN** sync executes the hook
- **THEN** the hook SHALL raise an error naming the missing required config field

#### Scenario: Bootstrap-board hook receives project root
- **GIVEN** a recipe declares an `on-sync` hook with `action = "bootstrap-board"`
- **WHEN** sync executes the hook
- **THEN** `execute_hooks()` SHALL receive the project root path as a parameter
- **AND** the marker file SHALL be written at `<project_root>/.recipe/<recipe-id>/bootstrap-ready`

#### Scenario: Deferred hook action prints informational notice
- **GIVEN** a recipe declares an `on-sync` hook with `action = "link-trello-card"`, `action = "sync-card-state"`, or `action = "comment-verification"`
- **WHEN** sync executes the hook after materialization
- **THEN** the hook SHALL print an informational notice (not a warning) that the action is deferred to agent runtime
- **AND** sync SHALL continue without failure

#### Scenario: Unsupported hook event or action
- **GIVEN** a recipe declares a hook with `event = "on-sync"` and `action = "unknown-action"`
- **WHEN** sync encounters this hook
- **THEN** sync SHALL emit a warning naming the unsupported action
- **AND** sync SHALL skip the hook without failing

### Requirement: Paso "ensure mcp-proxy daemon" insertado antes del fan-out

El pipeline `on-sync` SHALL incluir un nuevo paso "ensure mcp-proxy daemon" que se ejecuta después de la materialización de recipes y antes de cualquier paso `mcp-render` (fan-out por agente). El paso SHALL invocarse únicamente cuando la materialización haya detectado al menos un MCP con `mode = "shared"`.

#### Scenario: Paso daemon insertado cuando existen MCPs shared

- **WHEN** la materialización de recipes produce al menos un MCP con `mode = "shared"`
- **THEN** `sync.sh` SHALL ejecutar el paso "ensure mcp-proxy daemon" antes de iniciar el fan-out de configs por agente
- **AND** el daemon SHALL estar disponible antes de que cualquier agente reciba una config con `url`

#### Scenario: Paso daemon omitido cuando no hay MCPs shared

- **WHEN** la materialización de recipes produce cero MCPs con `mode = "shared"`
- **THEN** `sync.sh` SHALL omitir el paso "ensure mcp-proxy daemon"
- **AND** el pipeline SHALL continuar directamente al fan-out sin intentar iniciar ni verificar el daemon

#### Scenario: Fallo del paso daemon detiene el sync

- **WHEN** el paso "ensure mcp-proxy daemon" falla por una razón irrecuperable (por ejemplo, puerto no asignable o `mcp-proxy` crashea al spawnear) — nótese que `uvx` ausente NO entra en este escenario; lo cubre la degradación de `mcp-shared-daemon` (WARN + render stdio + exit 0)
- **THEN** `sync.sh` SHALL detenerse con un error explícito
- **AND** el fan-out de configs por agente SHALL NOT ejecutarse
