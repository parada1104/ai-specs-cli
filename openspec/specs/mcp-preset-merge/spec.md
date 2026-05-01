## ADDED Requirements

### Requirement: Recipe MCP presets merge shallowly with manifest precedence
The system SHALL merge recipe MCP presets using shallow merge where the project manifest always prevails over recipe defaults.

#### Scenario: Project has existing MCP config
- **WHEN** a recipe provides an MCP preset with the same ID as one already configured in the project's `ai-specs.toml`
- **THEN** the project manifest keys are preserved
- **AND** recipe keys not present in the manifest are added
- **AND** a warning is emitted for every conflicting key

#### Scenario: Project has no existing MCP config
- **WHEN** a recipe provides an MCP preset with an ID not present in the project's `ai-specs.toml`
- **THEN** the entire MCP preset from the recipe is created in the merged configuration

#### Scenario: Multiple recipes provide the same MCP preset
- **WHEN** two or more recipes provide MCP presets with the same ID
- **THEN** the project manifest still prevails
- **AND** recipe keys not in the manifest are added from the first recipe encountered
- **AND** subsequent recipes only add keys not already present in the accumulated configuration

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
