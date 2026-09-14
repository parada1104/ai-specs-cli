# trello-progress-comment Specification

## Purpose

Post structured progress comments on Trello cards after successful apply and verification of OpenSpec changes, summarizing files changed, test results, and verification verdict.

## Requirements

### Requirement: Trigger after apply and verify
The `trello-progress-comment` capability SHALL be invoked after a successful apply and verification of an OpenSpec change.

#### Scenario: Capability invoked on successful verification
- **WHEN** an OpenSpec change has completed the verify phase with a passing verdict
- **THEN** the `trello-progress-comment` capability SHALL be invoked for the linked Trello card

#### Scenario: No invocation on failed verification
- **WHEN** an OpenSpec change has completed the verify phase with a failing verdict
- **THEN** the capability SHALL still be invoked but SHALL include the failure verdict in the comment

### Requirement: Summarize files changed
The capability SHALL collect the list of files changed during the apply phase from the apply-progress metadata.

#### Scenario: Files changed collected
- **WHEN** the capability runs
- **THEN** it SHALL read the list of changed files from the change's `apply-progress.md`

#### Scenario: File summary structured
- **WHEN** the capability posts a progress comment
- **THEN** the comment SHALL include a summary of files changed, grouped by modification type (added, modified, removed)

### Requirement: Include test results
The capability SHALL include the test results from the verification phase in the progress comment.

#### Scenario: Test results retrieved
- **WHEN** the capability runs after verification
- **THEN** it SHALL extract the test count and pass/fail summary from the verify report

### Requirement: Include verification verdict
The capability SHALL include the verification verdict (PASS/FAIL) in the progress comment.

#### Scenario: Verdict included in comment
- **WHEN** the capability posts a progress comment
- **THEN** the comment SHALL prominently include the verification verdict

### Requirement: Link to archived change
The capability SHALL include a link to the archived change when available.

#### Scenario: Archive link included
- **WHEN** the change has been archived
- **THEN** the comment SHALL include a reference to the archive location

#### Scenario: Structured progress comment posted
- **WHEN** all data is collected
- **THEN** the capability SHALL post a single structured comment on the linked Trello card containing: verdict, test results summary, files changed summary, and archive link if applicable

### Requirement: Per-phase brief rules for progress comments

The recipe `[provides.brief].workflow_rules` and skill guidance SHALL require invoking
`trello-progress-comment` on milestones that update stakeholders — at minimum after
apply/verify outcomes (existing trigger) and when posting material progress that the
phase map treats as a comment checkpoint — when the `trello-mcp-workflow` recipe is
enabled and an active change has a valid `## Tracker` link section.

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
