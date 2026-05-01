## ADDED Requirements

### Requirement: Init-time MCP discovery is guidance-only

Recipe initialization MAY inspect current project MCP declarations and recipe-declared MCP presets to guide setup. Init-time MCP discovery SHALL be read-only unless a human-reviewed manifest delta is explicitly applied, and it SHALL NOT change sync-time MCP merge semantics.

#### Scenario: Init discovers configured MCP server

- **GIVEN** the project manifest declares `[mcp.trello]`
- **AND** recipe `tracker` declares `needs_mcp = ["trello"]` in `[init]`
- **WHEN** `ai-specs recipe init tracker` runs
- **THEN** the initialization brief SHALL report that `trello` is configured
- **AND** it SHALL use that fact only as setup guidance

#### Scenario: Init discovers missing MCP server

- **GIVEN** recipe `tracker` declares `needs_mcp = ["trello"]` in `[init]`
- **AND** the project manifest does not declare `[mcp.trello]`
- **WHEN** `ai-specs recipe init tracker` runs
- **THEN** the initialization brief SHALL report that `trello` is not configured
- **AND** it MAY point to recipe MCP presets or setup instructions
- **AND** it SHALL NOT silently create `[mcp.trello]`

#### Scenario: Init considers recipe MCP preset

- **GIVEN** recipe `tracker` declares a `trello` MCP preset under `[[provides.mcp]]`
- **WHEN** `ai-specs recipe init tracker` runs
- **THEN** the initialization brief MAY describe the preset as available guidance
- **AND** any proposed durable MCP change SHALL be reviewable before mutation

### Requirement: Init redacts MCP secrets

Init output SHALL NOT expose env-backed secrets, literal secret values, tokens, API keys, or passwords from project manifest MCP declarations or recipe MCP presets. Redacted values SHALL remain sufficient for an agent to know that a value exists and where it is sourced.

#### Scenario: Env-backed MCP value redacted

- **GIVEN** the project manifest declares an MCP env value such as `TRELLO_TOKEN = "$TRELLO_TOKEN"`
- **WHEN** `ai-specs recipe init tracker` prints MCP context
- **THEN** the output SHALL redact the token value
- **AND** it MAY indicate that `TRELLO_TOKEN` is provided by environment reference

#### Scenario: Literal secret value redacted

- **GIVEN** an MCP declaration contains a field whose name includes `token`, `secret`, `password`, or `key`
- **WHEN** `ai-specs recipe init tracker` prints MCP context
- **THEN** the output SHALL redact the field value
- **AND** it SHALL NOT print the literal secret

### Requirement: Init preserves manifest-precedence MCP sync semantics

Init-time MCP inspection SHALL preserve the manifest-precedence semantics of MCP preset merging. Init SHALL NOT present recipe MCP preset values as overriding existing project manifest MCP keys during sync.

#### Scenario: Project MCP key takes precedence over recipe preset

- **GIVEN** the project manifest declares `[mcp.openmemory]` with `command = "project-command"`
- **AND** a recipe declares an MCP preset `id = "openmemory"` with `command = "recipe-command"`
- **WHEN** `ai-specs recipe init <id>` reports MCP merge guidance
- **THEN** the guidance SHALL state that the project manifest key takes precedence during sync
- **AND** it SHALL NOT instruct the agent to rely on the recipe preset overriding the project key
