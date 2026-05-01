## MODIFIED Requirements

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