## ADDED Requirements

### Requirement: Board reading on session start
The `trello-session-bootstrap` capability SHALL read the bootstrap marker file (`.recipe/trello-mcp-workflow/bootstrap-ready`) and the project's `ai-specs.toml` to obtain `board_id`, then query the Trello board via MCP at the start of an agent session.

#### Scenario: Recipe reads board on session start
- **WHEN** an agent session starts and the `trello-mcp-workflow` recipe is enabled
- **AND** the `bootstrap-ready` marker file exists
- **THEN** the capability SHALL read `board_id` from the marker and query the board via Trello MCP

#### Scenario: Board validation fails gracefully
- **WHEN** an agent session starts and the Trello MCP query fails (network, invalid board ID, API limit)
- **THEN** the capability SHALL emit a warning to the agent and continue without blocking the session

### Requirement: Active card detection
The capability SHALL detect the active Trello card for the current project by matching cards in the configured `default_list` or by linking to the current OpenSpec change.

#### Scenario: Detect active card from board
- **WHEN** the board query succeeds and one or more cards exist in the `default_list`
- **THEN** the capability SHALL prioritize cards linked to the current OpenSpec change, or fall back to the most recently updated card in the `default_list`

#### Scenario: No active card found
- **WHEN** the board query succeeds but no cards exist in the `default_list`
- **THEN** the capability SHALL report no active card and suggest creating one via the card-linking capability

### Requirement: Present recommended next task
The capability SHALL present the detected active card as a recommended task for the agent session.

#### Scenario: Present detected card as recommendation
- **WHEN** an active card is detected
- **THEN** the capability SHALL present the card name, list, and URL as a recommended task for the current session

### Requirement: Integration with 3-level consensus check
The bootstrap primitive SHALL feed its card detection result into the session's 3-level consensus check (Trello, OpenSpec, OpenMemory).

#### Scenario: Trello layer feeds consensus check
- **WHEN** the capability detects an active card
- **THEN** it SHALL provide the card ID, name, and list position to the consensus check process

#### Scenario: Consensus resolution with Trello ground truth
- **WHEN** the consensus check runs and Trello reports a different active card than OpenSpec or OpenMemory
- **THEN** Trello SHALL be treated as the source of truth for work state