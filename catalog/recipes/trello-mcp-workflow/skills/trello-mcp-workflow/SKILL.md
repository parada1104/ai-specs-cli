---
name: trello-mcp-workflow
description: >
  Automated Trello board integration for ai-specs
  projects. Provides session bootstrapping, card linking, state
  synchronization, and progress reporting through Trello MCP tool calls.
license: MIT
metadata:
  author: ai-specs
  version: "1.0"
  scope: runtime
  auto_invoke:
    - "New structured change or feature request"
    - "Active change is missing a linked Trello card"
    - "Resuming work on a change with a stale or unknown card"
---

# Trello MCP Workflow

## Silence when unconfigured

This skill is inert unless the Trello MCP is reachable. Check that first: if the
server is not declared for the session, its credentials are missing, or a probe
tool call fails, STOP here and say nothing about the tracker. Do not name Trello,
do not offer to connect or configure it, do not propose creating or linking a
card, do not write a `## Tracker` section or `tracker.none`, and never gate,
delay, or block the user's work on a tracker that was never set up. An
unconfigured tracker is not an error to report — it is a tracker the project
chose not to use.

Everything below assumes that check passed.

## Prerequisites

- Trello MCP server configured and reachable in the runtime environment.
- On session start, read `board_id` from `[recipes.trello-mcp-workflow.config]` in `ai-specs/ai-specs.toml`. The board ID is a 24-hex-character string (not the 8-character shortLink from the Trello URL).
- Board ID available either in recipe config (`board_id`) or bootstrap marker file (`<AI_SPECS_HOME>/cache/projects/<hash>-<name>/.recipe/trello-mcp-workflow/bootstrap-ready`; legacy project-local fallback: `.recipe/trello-mcp-workflow/bootstrap-ready`).
- Agent has access to Trello MCP tools: `trello_get_active_board_info`, `trello_get_lists`, `trello_get_cards_by_list_id`, `trello_add_card_to_list`, `trello_add_comment`, `trello_move_card`, `trello_update_card_details`, `trello_get_card`.
- Forbidden tools: `trello_get_my_cards` and `trello_list_boards` MUST NOT be invoked. See Board Isolation section below.

## Configuration

| Field | Required | Default | Description |
|---|---|---|---|
| `board_id` | Yes | — | Trello board ID for the project. Example: `69ec097f13e2d38ecd89a557`. |
| `default_list` | No | `In Progress` | List name where new cards are created when no phase-specific list applies. |
| `epic_list` | No | `Epic` | List name where epic-type cards are placed. |
| `gate_mode` | No | `off` | Tracker card gate: `off` / `warn` / `always`. Opt in explicitly. |

Configuration is read from `[recipes.trello-mcp-workflow.config]` in `ai-specs/ai-specs.toml`.

---

## Board Isolation

The recipe enforces board isolation to prevent the agent from accessing Trello data outside the configured `board_id`. All capabilities MUST comply with these rules.

### Forbidden Tools

The following MCP tools MUST NOT be invoked by any capability:

| Tool | Restriction | Rationale |
|---|---|---|
| `trello_get_my_cards` | **Forbidden** — never invoke | Returns cards across all boards; leaks scope |
| `trello_list_boards` | **Forbidden** — never invoke | Enumerates all accessible boards; leaks scope |

If a step references one of these tools, the call MUST be skipped and a warning emitted to stderr. Violations MUST be logged to `.recipe/trello-mcp-workflow/warnings.log`.

### Restricted Tools

| Tool | Restriction | Rationale |
|---|---|---|
| `trello_set_active_board` | **Restricted** — bootstrap only (guard + setup) | Required once per session; later calls bypass guard |

`trello_set_active_board` MAY only be called during `trello-session-bootstrap`: inside the board guard precondition (guard step 3 — retry on mismatch) and during explicit session setup (bootstrap step 3). Any capability other than bootstrap that attempts this call MUST skip it and emit a warning.

### Board Guard

The board guard is a precondition check that verifies the Trello MCP server's active board matches the configured `board_id`. Every capability MUST run the board guard as its first step (step 0):

1. Read `board_id` from `[recipes.trello-mcp-workflow.config]` in `ai-specs/ai-specs.toml`.
2. Call `trello_get_active_board_info()`.
3. If the returned board ID does not match the configured `board_id`, call `trello_set_active_board(board_id)` and retry the check.
4. If the mismatch persists after one retry, log a warning to `.recipe/trello-mcp-workflow/warnings.log` with timestamp and capability name, then skip Trello operations for this session.

### Card idBoard Validation

Before calling `trello_get_card` or `trello_add_comment` on any card, the agent MUST:

1. Call `trello_get_card(cardId, fields="idBoard")`.
2. If the returned `idBoard` does not match the configured `board_id`, log a warning to `.recipe/trello-mcp-workflow/warnings.log` with card ID and board ID, then abort the operation.

---

## Capability: trello-session-bootstrap

Detect the active card and recommend the next task for the session.

### Trigger

Session start when the bootstrap marker exists at `<AI_SPECS_HOME>/cache/projects/<hash>-<name>/.recipe/trello-mcp-workflow/bootstrap-ready` (legacy project-local fallback `.recipe/trello-mcp-workflow/bootstrap-ready`), or when the agent explicitly invokes bootstrap.

### Steps

1. **Board guard**: Run the board guard precondition (see Board Isolation section above). If the guard fails and Trello operations are skipped, terminate bootstrap gracefully.
2. Enforce forbidden-tools compliance: ensure `trello_get_my_cards` and `trello_list_boards` are never invoked. If a step references them, skip the call and emit a warning.
3. Read `board_id` from `[recipes.trello-mcp-workflow.config]` in `ai-specs/ai-specs.toml` (or from the marker file context). Call `trello_set_active_board(board_id)` to set the board as active for subsequent operations.
4. Query the board using `trello_get_active_board_info()` or `trello_get_lists(boardId: <board_id>)` to retrieve structural context.
5. Detect the active card:
   - Fetch cards in **In Progress** and **In Review** lists using `trello_get_cards_by_list_id(listId: <id>, boardId: <board_id>)`.
   - Match on labels, keywords in card names, or change references found in card comments/descriptions.
   - If multiple candidates exist, prefer the card with the most recent activity.
6. Present the recommended next task to the agent, including:
   - Card name, current list, current phase label.
   - Suggested action (continue phase, start next phase, review, etc.).
7. Feed structured primitives (card ID, list ID, label IDs) into the session's consensus check so subsequent capabilities can reference them without re-querying.
8. **Graceful degradation**: If any Trello MCP call fails, emit a warning to stderr and continue the session without Trello context. Log the failure to `.recipe/trello-mcp-workflow/warnings.log`.

---


## Card link section (`## Tracker`)

The sole card-link contract for active changes is a `## Tracker` section inside
the change's `proposal.md` (fallback: `tasks.md` for tasks-only changes). No
separate artifact file and no folder-schema `trello_card_id` field.

```markdown
## Tracker

- **card_id**: `<24-hex>`
- **shortLink**: `<8-char>`          # optional
- **url**: https://trello.com/c/...
- **list**: <list name>              # optional
- **pr**: https://github.com/...     # optional
```

**Validity** (shared by doctor and the tracker-card gate): the section exists
and yields a non-empty `card_id`. `url` is expected; its absence is an INFO
nudge, not a block. Vocabulary that still says `trello_card_id` means the
`card_id` recorded in this section.

After creating or linking a card, agents MUST write this section before
apply/production work. The only documented exemption is
`openspec/changes/<slug>/tracker.none` (conceptual name `tracker:none`) with a
one-line reason — log it; this is rare.

---

## Capability: trello-card-linking

Link a structured change to a Trello card. Create a card from a template when no existing card matches.

### Trigger

New structured change or feature request.

### Steps

0. **Board guard**: Run the board guard precondition (see Board Isolation section above). If the guard fails and Trello operations are skipped, terminate this capability gracefully.
1. Detect whether a Trello card is already linked:
   - Check the `## Tracker` section of the change's `proposal.md` (fallback `tasks.md`).
   - Search recent comments on candidate cards for references to the change folder path.
   - Before calling `trello_get_card`, run card idBoard validation (see Card idBoard Validation above).
2. **If a card exists**: Post a structured linking comment using `trello_add_comment` with:
   - Before calling `trello_add_comment`, run card idBoard validation.
   - Change name.
   - Change folder path (relative to project root).
   - List of expected artifacts.
3. **If no card exists**: Prompt the agent to create one from a bundled template:
   - Select template type: `feature`, `bug`, `spike`, `epic`, or `handoff`.
   - Create the card in `default_list` using `trello_add_card_to_list(..., boardId: <board_id>)`.
   - Post the initial linking comment (same structure as step 2) using `trello_add_comment` with card idBoard validation.
4. **Record the link** in the `## Tracker` section of the change's `proposal.md` (or `tasks.md`) with at least `card_id` + `url`.
5. **Only omit a card** by writing `openspec/changes/<slug>/tracker.none` with a one-line reason; log the exemption (stderr and/or `.recipe/trello-mcp-workflow/warnings.log`). This is rare — declining card creation without `tracker.none` is not a free pass.

### Templates

Templates are located at `ai-specs/recipes/trello-mcp-workflow/overrides/templates/` and are installed by the recipe:

| Template | File | Use Case |
|---|---|---|
| Feature | `card-feature.md` | New capabilities, commands, recipes, skills. |
| Bug | `card-bug.md` | Regressions or fixes with reproduction steps. |
| Spike | `card-spike.md` | Research with go/no-go conclusion. |
| Epic | `card-epic.md` | Grouping cards; no direct code implementation. |
| Handoff | `card-handoff.md` | Session continuity between agents. |

---

## Capability: trello-state-sync

Synchronize project phase transitions with Trello card position and labels.

### Trigger

Phase transitions defined by the project's workflow (e.g., design → implementation → review → done).

### Steps

0. **Board guard**: Run the board guard precondition (see Board Isolation section above). If the guard fails and Trello operations are skipped, terminate this capability gracefully.
1. Identify the linked card (from session context or change metadata). Run card idBoard validation before calling `trello_get_card`.
2. Resolve the target list ID by name using board lists (query with `trello_get_lists(boardId: <board_id>)`).
3. Move the card to the target list using `trello_move_card(cardId, listId, boardId: <board_id>)`.
4. Update labels on the card using `trello_update_card_details(cardId, boardId: <board_id>, ...)`.
5. Post a phase-transition comment using `trello_add_comment`. Run card idBoard validation before calling `trello_add_comment`.

Phase-to-list and phase-to-label mappings are project-specific and configured in the recipe config or project conventions.

### Graceful Degradation

If the target list does not exist on the board, emit a warning, skip the move, and continue with the label update. Log the failure to `.recipe/trello-mcp-workflow/warnings.log`.

---

## Capability: trello-progress-comment

Post a structured progress comment on the linked card after significant implementation milestones.

### Trigger

After significant implementation milestones or at project-defined review points.

### Steps

0. **Board guard**: Run the board guard precondition (see Board Isolation section above). If the guard fails and Trello operations are skipped, terminate this capability gracefully.
1. Identify the linked card (from session context or change metadata). Run card idBoard validation before calling `trello_get_card`.
2. Collect available progress data (changed files, test results, review notes) from the project workspace.
3. Assemble a structured comment:
   ```markdown
   ## Progress: {phase}

   **Status**: {description of current state}
   **Files Changed**: {count} files — added: {list}, modified: {list}, removed: {list}
   ```
4. Post the comment using `trello_add_comment`. Run card idBoard validation before calling `trello_add_comment`.

### Graceful Degradation

If progress data files are unavailable, post the comment with available data and mark missing sections as `unavailable`.

---

## Graceful Degradation (General)

- All runtime Trello **availability** failures (MCP/network/API down) emit warnings to stderr and continue — never block.
- Optionally log warnings to `.recipe/trello-mcp-workflow/warnings.log` with timestamp, capability, and error detail.
- A **missing `## Tracker` link section** is **not** an availability failure. Do not claim 'Trello unavailable' to skip it; create/link the card and write the section (or write `tracker.none` with a logged reason). The tracker-card gate may warn or block production/PR-archive actions when the artifact is missing.
- If the Trello MCP server is unreachable, skip Trello MCP calls for the remainder of the session and log a single warning — but still do not invent an availability excuse for a missing link section once MCP is back.

---

## MCP Tools Reference

| Tool | Used By | Purpose | Board Isolation |
|---|---|---|---|
| `trello_get_active_board_info` | session-bootstrap | Retrieve board structure and current state. | Board guard uses this to verify active board. |
| `trello_get_lists` | session-bootstrap, state-sync | Resolve list names to IDs. | Must pass `boardId: <board_id>`. |
| `trello_get_cards_by_list_id` | session-bootstrap | Fetch cards in active lists to detect the current card. | Must pass `boardId: <board_id>`. |
| `trello_get_card` | card-linking, state-sync, progress-comment | Retrieve card details for matching and comment assembly. | Must validate `idBoard` before use. |
| `trello_add_card_to_list` | card-linking | Create a new card from a template. | Must pass `boardId: <board_id>`. |
| `trello_add_comment` | card-linking, state-sync, progress-comment | Post structured comments on cards. | Must validate `idBoard` before use. |
| `trello_move_card` | state-sync | Move a card to a new list on phase transition. | Must pass `boardId: <board_id>`. |
| `trello_update_card_details` | state-sync | Replace phase labels on a card. | Must pass `boardId: <board_id>`. |

---

## Card Contract

The automation above (capabilities, board isolation, templates) defines *how* the
agent drives Trello. This section defines *what* a good card looks like and how PM
work is structured. Board IDs and list names are never hardcoded here — read them
from `[recipes.trello-mcp-workflow.config]` in `ai-specs/ai-specs.toml`.

### Separation of concerns

- **Trello** tracks state, priority, dependencies, and PM/CTO visibility.
- **Canonical store** holds durable decisions and handoffs.
- **Operational memory** holds searchable session continuity.
- **SDD artifacts** hold specs/design/tasks when a card requires durable change.

Do not mix these: Trello is state, not memory. A card links to its decision/spec
records; it does not duplicate them.

### Card types

| Type | Use | Template |
|---|---|---|
| `epic` | Groups cards; no direct code. | `card-epic.md` |
| `feature` | New capability, command, recipe, skill, or behavior. | `card-feature.md` |
| `bug` | Regression or fix with reproduction + regression test. | `card-bug.md` |
| `spike` | Research with a go/no-go conclusion. | `card-spike.md` |
| `decision` | Tradeoff that must be recorded in the canonical store. | `card-decision.md` |
| `handoff` | Session-to-session continuity when the active card is not enough. | `card-handoff.md` |

Templates install to `ai-specs/recipes/trello-mcp-workflow/overrides/templates/`.

### Card base structure

Every work card carries: Context (why it exists), Objective (one sentence),
Scope (checklist of deliverables), Out of scope, Acceptance Criteria (verifiable),
Dependencies (blocking cards/changes), and Notes (links, decisions).

### Flow rules

- One session works one explicit request, one card, or one change.
- A card that implies implementation, durable design, or a complex technical
  decision should map to an SDD cycle.
- A card may block another; dependencies must be explicit in Trello.
- A card should link or name its SDD change when one exists.
- Do not move a blocked card into apply while its dependencies stay open.

### SDD checklist for features

Add to the card when applicable:

```markdown
- [ ] Change created in a dedicated worktree
- [ ] Proposal complete
- [ ] Specs complete
- [ ] Design complete
- [ ] Tasks complete
- [ ] Apply executed
- [ ] Verify report generated
- [ ] PR / merge done if applicable
- [ ] Change archived if applicable
```

### Card close ritual

Before moving a card to Review/Done:

- Verify the acceptance criteria.
- Confirm the state of SDD artifacts if SDD was used.
- Record the decision/handoff in the canonical store if it changes project canon.
- Save operational memory only if it helps future sessions.
- Leave links to the PR, change, verify report, or handoff.
