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