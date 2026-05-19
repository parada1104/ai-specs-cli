# Delta Spec: Reforzar Board Scope en Trello MCP Workflow

## ADDED Requirements

### Requirement: Forbidden Tools List

The system MUST maintain a forbidden-tools list that prohibits invocation of `trello_get_my_cards` and `trello_list_boards` by any recipe capability.

`tre llo_set_active_board` SHALL be restricted to `trello-session-bootstrap` step 2 only.

#### Scenario: Agent attempts forbidden tool during bootstrap

- GIVEN a session bootstrap in progress
- WHEN any capability other than bootstrap attempts `trello_set_active_board`
- THEN the agent MUST skip the call and emit a warning to stderr

#### Scenario: Agent attempts board enumeration

- GIVEN any capability execution
- WHEN the agent attempts `trello_list_boards` or `trello_get_my_cards`
- THEN the agent MUST abort the call and log the violation to `warnings.log`

### Requirement: Board Isolation Configuration Schema

The recipe.toml MUST include a `[config.board_isolation]` block with:
- `forbidden_tools`: list of strings (`trello_get_my_cards`, `trello_list_boards`)
- `restricted_tools`: list of strings (`trello_set_active_board`)
- `card_validation_required`: boolean (`true`)

#### Scenario: Recipe configuration validation

- GIVEN `ai-specs sync` runs
- WHEN recipe.toml is parsed
- THEN `[config.board_isolation]` fields MUST be present and typed correctly

### Requirement: Board Guard at Bootstrap

After `trello_set_active_board`, the agent MUST verify the active board matches the configured `board_id` via `trello_get_active_board_info`.

On mismatch: emit warning, retry once, and if still mismatched, abort Trello operations and log to `warnings.log`.

#### Scenario: Successful board guard verification

- GIVEN `board_id` is configured correctly
- WHEN bootstrap calls `trello_set_active_board(board_id)` then `trello_get_active_board_info`
- THEN the returned board `id` MUST equal the configured `board_id`

#### Scenario: Board guard detects mismatch and recovers

- GIVEN a transient MCP state issue
- WHEN the first verification fails but the retry succeeds
- THEN the agent MUST continue bootstrap with a warning logged

#### Scenario: Board guard persistent mismatch

- GIVEN a persistent board mismatch or misconfiguration
- WHEN verification fails after one retry
- THEN the agent MUST abort Trello operations and log the failure to `warnings.log`

### Requirement: Card idBoard Validation

Before `trello_get_card` or `trello_add_comment`, the agent MUST fetch the card's `idBoard` field and validate it matches the configured `board_id`. On mismatch, abort the operation and log to `warnings.log`.

#### Scenario: Valid card operation

- GIVEN a card belonging to the configured board
- WHEN the agent calls `trello_get_card` with `fields=idBoard`
- THEN the operation proceeds only if `idBoard` equals `board_id`

#### Scenario: Cross-board card rejected

- GIVEN a card ID from a different board
- WHEN the agent attempts `trello_get_card` or `trello_add_comment`
- THEN the operation MUST abort and a warning MUST be logged

## MODIFIED Requirements

### Requirement: trello-session-bootstrap Capability

The bootstrap capability MUST read `board_id` from `ai-specs.toml`, call `trello_set_active_board(board_id)`, then verify via `trello_get_active_board_info`. The capability MUST enforce the forbidden-tools list. (Previously: bootstrap set active board without post-verification and had no tool restrictions.)

#### Scenario: Bootstrap with board guard

- GIVEN the marker file exists and `board_id` is configured
- WHEN bootstrap executes steps 1–7
- THEN step 2 MUST include verification and step 1 MUST check forbidden-tools compliance

### Requirement: trello-card-linking Capability

The card-linking capability MUST pass `boardId` explicitly in `trello_add_card_to_list`. It MUST run board guard as step 0. It MUST validate `idBoard` before `trello_get_card` and `trello_add_comment`. (Previously: relied on implicit active board and did not validate card board ownership.)

#### Scenario: Create card with explicit board isolation

- GIVEN a new change requires a Trello card
- WHEN the agent calls `trello_add_card_to_list`
- THEN the call MUST include `boardId=<configured_board_id>`

### Requirement: trello-state-sync Capability

The state-sync capability MUST pass `boardId` explicitly in `trello_get_lists`, `trello_move_card`, and `trello_update_card_details`. It MUST run board guard as step 0. It MUST validate `idBoard` before `trello_get_card`. (Previously: used implicit active board without explicit boardId or card validation.)

#### Scenario: Move card with explicit board scope

- GIVEN a phase transition requires moving a card
- WHEN the agent calls `trello_move_card`
- THEN the call MUST include `boardId=<configured_board_id>`

### Requirement: trello-progress-comment Capability

The progress-comment capability MUST run board guard as step 0. It MUST validate `idBoard` before `trello_add_comment`. (Previously: lacked board guard precondition and card board validation.)

#### Scenario: Post progress with validation

- GIVEN a milestone is reached
- WHEN the agent calls `trello_add_comment`
- THEN the card's `idBoard` MUST be validated against `board_id` first

## REMOVED Requirements

### Requirement: SDD Checklist Section in card-feature.md

(Reason: Replaced by reference to `trello-pm-workflow` skill, which provides the canonical SDD checklist.)

#### Scenario: Template generation

- GIVEN a new card is created from the feature template
- WHEN the template is rendered
- THEN it MUST NOT contain a section titled "SDD Checklist"
