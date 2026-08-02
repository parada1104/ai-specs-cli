# trello-state-sync (delta)

## ADDED Requirements

### Requirement: Per-phase brief rules for state sync

When the `trello-mcp-workflow` recipe is enabled and an active change has a
valid `## Tracker` link section, the recipe `[provides.brief].workflow_rules`
and skill guidance SHALL require invoking `trello-state-sync` on SDD phase
transitions for that linked card (list move, phase label update, and phase
comment per existing capability requirements).

Agents MUST resolve the target card from the `## Tracker` link section
(`card_id` / `url`) rather than from a mythical folder-schema `trello_card_id`
field.

#### Scenario: Brief requires state sync on phase transition

- **GIVEN** the catalog `trello-mcp-workflow` recipe after this change
- **AND** an active change has valid `## Tracker` link section
- **WHEN** the change transitions SDD phase (e.g. `design` → `tasks`)
- **THEN** brief/skill rules MUST require `trello-state-sync` for the linked card
- **AND** the card identity MUST be read from the `## Tracker` link section

#### Scenario: No state-sync obligation under tracker:none

- **GIVEN** an active change has a `tracker.none` exemption and no `## Tracker` link section
- **WHEN** the change transitions SDD phase
- **THEN** brief/skill rules MUST NOT require a Trello list/label sync for that change

### Requirement: Degrade on availability failure only

`trello-state-sync` SHALL continue to degrade gracefully when Trello MCP /
network / API is unavailable: emit a warning, skip the sync mutation, and do
not block the local phase transition. A missing `## Tracker` link section on a non-exempt
active change is a missing-artifact condition for doctor/gate surfaces, not an
availability failure for this capability.

#### Scenario: MCP unavailable warns and continues

- **GIVEN** a phase transition occurs for a change with valid `## Tracker` link section
- **AND** Trello MCP move/label/comment calls fail due to unavailability
- **WHEN** `trello-state-sync` runs
- **THEN** it MUST emit a warning and MUST NOT block the local phase transition

#### Scenario: Missing Tracker link section is not reported as unavailable

- **GIVEN** an active non-exempt change lacks the `## Tracker` link section
- **WHEN** an agent considers whether to run `trello-state-sync`
- **THEN** the agent MUST treat the gap as a missing link artifact
- **AND** MUST NOT claim Trello is unavailable as the reason to skip linking
