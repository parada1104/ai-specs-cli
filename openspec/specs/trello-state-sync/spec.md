# trello-state-sync Specification

## Purpose

Synchronize OpenSpec change phase transitions to Trello card state by moving cards to corresponding lists and updating phase labels.

## Requirements

### Requirement: Trigger on SDD phase transitions
The `trello-state-sync` capability SHALL be invoked when an OpenSpec change transitions from one SDD phase to another.

#### Scenario: Capability invoked on phase transition
- **WHEN** an OpenSpec change transitions from one SDD phase to another (e.g., `proposal` → `specs`, `specs` → `design`, `design` → `tasks`, `tasks` → `apply`, `apply` → `verify`, or `verify` → `archive`)
- **THEN** the `trello-state-sync` capability SHALL be invoked for the linked Trello card

### Requirement: Move card to corresponding list
The capability SHALL move the linked Trello card to the list that corresponds to the new SDD phase.

#### Scenario: Card moved to mapped list
- **WHEN** a phase transition occurs and a phase-to-list mapping is defined in the skill
- **THEN** the capability SHALL move the card to the Trello list corresponding to the new phase

#### Scenario: List mapping resolution
- **WHEN** a phase transition occurs and no explicit mapping exists for the target phase
- **THEN** the capability SHALL use the `default_list` as the fallback target list

### Requirement: Update card labels
The capability SHALL update the card's phase label to reflect the current SDD phase.

#### Scenario: Labels updated to reflect phase
- **WHEN** a card is moved to a new list for a phase transition
- **THEN** the capability SHALL replace the existing phase label with a label corresponding to the new phase

#### Scenario: Label mapping applied
- **WHEN** the target phase has a corresponding label defined in the skill
- **THEN** the capability SHALL remove the previous phase label and add the new phase label on the card

### Requirement: Comment phase change
The capability SHALL post a comment on the card documenting the phase transition.

#### Scenario: Phase change comment posted
- **WHEN** a card is moved to a new list for a phase transition
- **THEN** the capability SHALL post a comment documenting the transition (e.g., "Phase changed: design → tasks")

#### Scenario: Comment includes transition metadata
- **WHEN** the capability posts a phase-transition comment
- **THEN** the comment SHALL include the previous phase, the new phase, and a link to the relevant OpenSpec artifact

### Requirement: Per-phase brief rules for state sync

The recipe `[provides.brief].workflow_rules` and skill guidance SHALL require invoking
`trello-state-sync` on SDD phase transitions for the linked card (list move, phase
label update, and phase comment per existing capability requirements) when the
`trello-mcp-workflow` recipe is enabled and an active change has a valid `## Tracker`
link section.

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
