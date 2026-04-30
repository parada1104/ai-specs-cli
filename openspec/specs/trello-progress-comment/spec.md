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