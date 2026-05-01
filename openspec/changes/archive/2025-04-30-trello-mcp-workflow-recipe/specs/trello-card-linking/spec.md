## ADDED Requirements

### Requirement: Trigger on change creation
The `trello-card-linking` capability SHALL be invoked when a new OpenSpec change is created and the `trello-mcp-workflow` recipe is enabled.

#### Scenario: Capability invoked on new change
- **WHEN** `openspec new change` is executed and the `trello-mcp-workflow` recipe is enabled
- **THEN** the `trello-card-linking` capability SHALL be invoked

### Requirement: Link change to existing Trello card
The capability SHALL detect if a Trello card is already linked to the current session context and post a structured comment on that card.

#### Scenario: Comment posted on existing card
- **WHEN** a linked Trello card is detected from the session context
- **THEN** the capability SHALL post a comment on the card with the change name and change folder path

#### Scenario: Comment content structure
- **WHEN** the capability posts a linking comment
- **THEN** the comment SHALL contain the change name, the change folder path relative to the project root, and a list of expected artifacts

### Requirement: Create card from template when absent
When no Trello card is linked, the capability SHALL offer to create a card from a bundled template.

#### Scenario: Offer card creation when no card exists
- **WHEN** no linked Trello card is detected for the current change
- **THEN** the capability SHALL offer the agent a choice to create a card using one of the bundled templates

#### Scenario: Card creation from bundled template
- **WHEN** the agent accepts the card creation offer
- **THEN** the capability SHALL create a Trello card using the selected template (feature, bug, spike, epic, or handoff)
- **AND** the card SHALL be placed in the configured `default_list`
- **AND** the capability SHALL post an initial linking comment on the new card

#### Scenario: Skip card creation
- **WHEN** the agent declines the offer to create a card
- **THEN** the capability SHALL skip linking and allow the OpenSpec change to exist without a Trello card