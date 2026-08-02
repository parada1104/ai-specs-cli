# tracker-card-gate (delta)

## ADDED Requirements

### Requirement: Pre-tool-use tracker card gate hook

The `trello-mcp-workflow` recipe SHALL distribute a portable `pre-tool-use`
runtime hook script `hooks/tracker-card-gate.sh` (via `[[provides.hooks]]`,
`blocking = true`) that machine-enforces card-link presence before production
work. Semantic model is **plan-build-gate** (artifact must exist before
production edits), not worktree protected-branch logic.

The hook SHALL follow the normalized hook contract: read stdin JSON
`{event, tool_name, tool_input, cwd}` (accepting harness path/command field
aliases as needed), exit `0` to allow, exit `2` to block, and fail open
(exit `0`) on parse or lookup errors.

The gate MUST NOT call Trello MCP. Presence of a valid
`openspec/changes/<slug>/trello.md` (or `tracker.none`) is the proof.

#### Scenario: Hook is declared as pre-tool-use blocking

- **GIVEN** the catalog `trello-mcp-workflow` recipe after this change
- **WHEN** `recipe.toml` is inspected
- **THEN** it MUST declare a `[[provides.hooks]]` entry for
  `hooks/tracker-card-gate.sh` with `event = "pre-tool-use"` and
  `blocking = true`

### Requirement: Gate activation predicate

The gate is active only when **all** hold:

1. `trello-mcp-workflow` is enabled for the project
2. The bootstrap-ready marker is present at the runtime cache path
   `cache/projects/<hash>-<name>/.recipe/trello-mcp-workflow/bootstrap-ready`
3. Gate mode ≠ `off`

Supported modes: `off` | `warn` | `always`, selected via recipe config and/or
env stamp (design locks exact config/env key names). Dogfood default for this
repository SHALL be `warn`. Mode `always` is opt-in.

When inactive, the hook MUST exit `0` without blocking.

#### Scenario: Inactive when recipe disabled

- **GIVEN** `trello-mcp-workflow` is not enabled
- **AND** a production-path `Write` targets `lib/foo.py` for an active change
  without `trello.md`
- **WHEN** `tracker-card-gate.sh` runs
- **THEN** it MUST exit `0`

#### Scenario: Inactive when bootstrap marker absent

- **GIVEN** the recipe is enabled and mode is `always`
- **AND** the bootstrap-ready marker is absent
- **AND** a production-path edit lacks a card artifact
- **WHEN** the hook runs
- **THEN** it MUST exit `0`

#### Scenario: Inactive when mode is off

- **GIVEN** the recipe is enabled and bootstrap marker is present
- **AND** gate mode is `off`
- **AND** a production-path edit lacks a card artifact
- **WHEN** the hook runs
- **THEN** it MUST exit `0`

### Requirement: Production-path enforcement by mode

When the gate is active, and a matched file-write tool targets a path under the
configured production directory set (default includes at least `lib`, `catalog`,
and `bin`; exact set locked in design / config), and the active non-archive
change lacks both a valid `trello.md` and a `tracker.none` exemption:

- mode=`always` → exit `2` and stderr remediation naming `trello.md` /
  create-or-link card
- mode=`warn` → exit `0` and emit a stderr warning (dogfood default)

When a valid `trello.md` or `tracker.none` is present for the active change,
production writes MUST be allowed (exit `0`) regardless of `warn`/`always`.

#### Scenario: always blocks production write without card

- **GIVEN** gate active with mode=`always`
- **AND** an active change exists without valid `trello.md` and without
  `tracker.none`
- **AND** a `Write`/`Edit` targets a production path (e.g. `lib/foo.py`)
- **WHEN** the hook receives the normalized event
- **THEN** it MUST exit `2`
- **AND** stderr MUST mention remediation via `trello.md` / card create-or-link

#### Scenario: warn allows production write with stderr warning

- **GIVEN** gate active with mode=`warn`
- **AND** an active change exists without valid `trello.md` and without
  `tracker.none`
- **AND** a production-path `Write` is attempted
- **WHEN** the hook runs
- **THEN** it MUST exit `0`
- **AND** stderr MUST include a warning about the missing card-link artifact

#### Scenario: valid trello.md allows production write

- **GIVEN** gate active with mode=`always`
- **AND** the active change has valid `trello.md` (`card_id` + `url`)
- **AND** a production-path `Write` is attempted
- **WHEN** the hook runs
- **THEN** it MUST exit `0`

#### Scenario: tracker:none allows production write

- **GIVEN** gate active with mode=`always`
- **AND** the active change has `tracker.none` and no `trello.md`
- **AND** a production-path `Write` is attempted
- **WHEN** the hook runs
- **THEN** it MUST exit `0`

### Requirement: openspec paths never blocked

Writes under `openspec/**` (including `openspec/changes/**` planning files and
`trello.md` itself) MUST never be blocked by this gate, in any mode. Agents MUST
be able to create the link artifact and planning files without deadlock.

#### Scenario: Writing proposal without card is allowed

- **GIVEN** gate active with mode=`always`
- **AND** no `trello.md` exists for the active change
- **AND** a `Write` targets `openspec/changes/<slug>/proposal.md`
- **WHEN** the hook runs
- **THEN** it MUST exit `0`

#### Scenario: Writing trello.md itself is allowed

- **GIVEN** gate active with mode=`always`
- **AND** a `Write` targets `openspec/changes/<slug>/trello.md`
- **WHEN** the hook runs
- **THEN** it MUST exit `0`

### Requirement: Fail-open on parse and lookup errors

Malformed stdin JSON, missing path/command fields needed for evaluation, and
unexpected lookup errors MUST fail open (exit `0`). A buggy guard must never
wedge all editing.

#### Scenario: Malformed JSON fails open

- **GIVEN** stdin is malformed JSON
- **WHEN** the hook runs
- **THEN** it MUST exit `0`

#### Scenario: Missing file_path on a file-write event fails open

- **GIVEN** a file-write-shaped event with no extractable path
- **WHEN** the hook runs
- **THEN** it MUST exit `0`

### Requirement: Optional shell-mode PR and archive coverage

The recipe MAY additionally distribute a sibling shell hook
(dual-hook pattern: distinct hook id, matcher covering
`Bash|Shell|Execute|Terminal`) that invokes the same
`hooks/tracker-card-gate.sh` so Cursor's file-write matcher skip rule cannot
swallow shell coverage.

When shell-mode coverage ships, high-confidence commands that open a PR or
archive a change (at minimum `gh pr create`, plus design-locked archive helper
patterns) SHALL be subject to the same activation predicate and mode semantics
as production-path writes. Low-confidence / ambiguous shell commands MUST fail
open.

#### Scenario: High-confidence gh pr create blocked without card in always

- **GIVEN** gate active with mode=`always` and shell-mode coverage enabled
- **AND** the active change lacks valid `trello.md` and `tracker.none`
- **AND** stdin is a shell event whose command is high-confidence `gh pr create`
- **WHEN** the hook runs
- **THEN** it MUST exit `2`
- **AND** stderr MUST name remediation via `trello.md`

#### Scenario: Shell PR warn mode exits zero with warning

- **GIVEN** gate active with mode=`warn` and shell-mode coverage enabled
- **AND** the active change lacks valid `trello.md` and `tracker.none`
- **AND** stdin is a high-confidence `gh pr create` shell event
- **WHEN** the hook runs
- **THEN** it MUST exit `0`
- **AND** stderr MUST warn about the missing card-link artifact

#### Scenario: Ambiguous shell command fails open

- **GIVEN** gate active with mode=`always` and shell-mode coverage enabled
- **AND** the shell command is ambiguous or not a high-confidence PR/archive
  action
- **WHEN** the hook runs
- **THEN** it MUST exit `0`

#### Scenario: Dual-hook registration when shell coverage ships

- **GIVEN** shell-mode coverage is included in the recipe
- **WHEN** hooks are rendered for harnesses that match tools by name
- **THEN** there MUST be a file-write hook entry and a distinct shell hook entry
  sharing `hooks/tracker-card-gate.sh`
- **AND** Cursor MUST receive a separate shell-only registration that does not
  merge file-write matcher tokens into the shell matcher

### Requirement: Dogfood default warn

This repository's dogfood manifest SHALL set tracker-card gate mode to `warn`
explicitly so planning and applying this change does not self-deadlock.
Promoting dogfood to `always` is a later config flip outside this requirement's
default.

#### Scenario: Dogfood ai-specs.toml sets warn

- **GIVEN** this repository's `ai-specs/ai-specs.toml` after this change
- **WHEN** the `trello-mcp-workflow` recipe config is read
- **THEN** gate mode MUST be set to `warn` explicitly

### Requirement: Anti-bypass brief and skill guidance

Recipe skill and `[provides.brief].workflow_rules` MUST state that a gate
warn/block for a missing card-link artifact is never grounds to bypass via
shell writes, skipping sync, or claiming "Trello unavailable" when the failure
is a missing `trello.md`. The correct response is to create/link the card and
write `trello.md` (or an explicit logged `tracker:none` exemption when
intentionally untracked).

#### Scenario: Brief forbids unavailable excuse for missing artifact

- **GIVEN** the catalog `trello-mcp-workflow` recipe after this change
- **WHEN** skill and brief workflow rules are read
- **THEN** they MUST forbid treating a missing `trello.md` as Trello
  unavailability
- **AND** MUST direct the agent to write `trello.md` (or `tracker:none`) instead
  of bypassing the gate
