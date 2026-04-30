## ADDED Requirements

### Requirement: Durable recipe init output

Recipe initialization output that is intended to survive sync SHALL be stored in the existing manifest section responsible for that data. Per-recipe values SHALL be stored under `[recipes.<id>.config]` unless another manifest section is explicitly responsible for the value.

#### Scenario: Init stores per-recipe config

- **GIVEN** recipe `tracker` declares config field `board_id`
- **WHEN** init proposes durable storage for the selected board
- **THEN** the proposed manifest delta SHALL target `[recipes.tracker.config]`
- **AND** the proposed key SHALL be `board_id`

#### Scenario: Init stores existing manifest responsibility elsewhere

- **GIVEN** init discovers an MCP server that belongs in `[mcp.trello]`
- **WHEN** init proposes durable storage for MCP declaration data
- **THEN** the proposal MAY target `[mcp.trello]`
- **AND** it SHALL NOT duplicate that MCP declaration under `[recipes.tracker.config]` unless the recipe config schema explicitly requires a separate reference value

### Requirement: Init manifest deltas avoid duplicate declarations

When init proposes updates to `ai-specs/ai-specs.toml`, it SHALL update existing `[recipes.<id>]` and `[recipes.<id>.config]` declarations instead of appending duplicate tables or duplicate keys.

#### Scenario: Existing recipe table updated

- **GIVEN** the manifest already contains `[recipes.tracker]`
- **WHEN** init proposes changing `enabled` or `version`
- **THEN** the proposed manifest delta SHALL update the existing `[recipes.tracker]` table
- **AND** it SHALL NOT add a second `[recipes.tracker]` table

#### Scenario: Existing config key updated

- **GIVEN** the manifest already contains `[recipes.tracker.config]` with `board_id = "old"`
- **WHEN** init proposes `board_id = "new"`
- **THEN** the proposed manifest delta SHALL update the existing `board_id` key
- **AND** it SHALL NOT append a duplicate `board_id` key

#### Scenario: Missing config table added once

- **GIVEN** the manifest contains `[recipes.tracker]`
- **AND** it does not contain `[recipes.tracker.config]`
- **WHEN** init proposes per-recipe config values
- **THEN** the proposed manifest delta SHALL add exactly one `[recipes.tracker.config]` table
