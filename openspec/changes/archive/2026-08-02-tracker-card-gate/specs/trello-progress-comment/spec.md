# trello-progress-comment (delta)

## ADDED Requirements

### Requirement: Per-phase brief rules for progress comments

When the `trello-mcp-workflow` recipe is enabled and an active change has a
valid `## Tracker` link section, the recipe `[provides.brief].workflow_rules`
and skill guidance SHALL require invoking `trello-progress-comment` on
milestones that update stakeholders — at minimum after apply/verify outcomes
(existing trigger) and when posting material progress that the phase map treats
as a comment checkpoint.

Agents MUST resolve the target card from the `## Tracker` link section.

#### Scenario: Brief requires progress comment on verify milestone

- **GIVEN** the catalog `trello-mcp-workflow` recipe after this change
- **AND** an active change has valid `## Tracker` link section
- **WHEN** the change completes verify with a PASS or FAIL verdict
- **THEN** brief/skill rules MUST require a structured progress comment on the
  linked card
- **AND** the card identity MUST be read from the `## Tracker` link section

#### Scenario: No progress-comment obligation under tracker:none

- **GIVEN** an active change has a `tracker.none` exemption and no `## Tracker` link section
- **WHEN** the change reaches an apply/verify milestone
- **THEN** brief/skill rules MUST NOT require a Trello progress comment for that change

### Requirement: Degrade on availability failure only

`trello-progress-comment` SHALL continue to degrade gracefully when Trello MCP /
network / API is unavailable: emit a warning, skip the comment, and do not block
local verify/archive work. A missing `## Tracker` link section on a non-exempt active change
is a missing-artifact condition for doctor/gate surfaces, not an availability
failure for this capability.

#### Scenario: MCP unavailable warns and continues

- **GIVEN** a verify milestone is reached for a change with valid `## Tracker` link section
- **AND** `trello_add_comment` fails due to unavailability
- **WHEN** `trello-progress-comment` runs
- **THEN** it MUST emit a warning and MUST NOT block local completion of the phase

#### Scenario: Missing Tracker link section is not reported as unavailable

- **GIVEN** an active non-exempt change lacks the `## Tracker` link section
- **WHEN** an agent considers whether to post a progress comment
- **THEN** the agent MUST treat the gap as a missing link artifact
- **AND** MUST NOT claim Trello is unavailable as the reason the card was never linked
