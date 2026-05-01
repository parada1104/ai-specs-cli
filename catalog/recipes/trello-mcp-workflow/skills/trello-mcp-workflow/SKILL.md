---
metadata:
  name: trello-mcp-workflow
  scope: runtime
  auto_invoke: false
---

# Trello MCP Workflow

Automated Trello board integration for ai-specs SDD (Spec-Driven Development) workflows. Provides session bootstrapping, card linking, state synchronization, and progress reporting through Trello MCP tool calls.

## Prerequisites

- Trello MCP server configured and reachable in the runtime environment.
- Board ID available either in recipe config (`board_id`) or bootstrap marker file (`.recipe/trello-mcp-workflow/bootstrap-ready`).
- Agent has access to Trello MCP tools: `trello_get_active_board_info`, `trello_get_lists`, `trello_get_cards_by_list_id`, `trello_add_card_to_list`, `trello_add_comment`, `trello_move_card`, `trello_update_card_details`, `trello_get_card`.

## Configuration

| Field | Required | Default | Description |
|---|---|---|---|
| `board_id` | Yes | — | Trello board ID for the project. Example: `69ec0a2099ea20956e371d62`. |
| `default_list` | No | `In Progress` | List name where new cards are created when no phase-specific list applies. |
| `epic_list` | No | `Epic` | List name where epic-type cards are placed. |

Configuration is read from `[recipes.trello-mcp-workflow.config]` in `ai-specs/ai-specs.toml`.

---

## Capability: trello-session-bootstrap

Detect the active card and recommend the next task for the session.

### Trigger

Session start when the marker file `.recipe/trello-mcp-workflow/bootstrap-ready` exists, or when the agent explicitly invokes bootstrap.

### Steps

1. Read the marker file at `.recipe/trello-mcp-workflow/bootstrap-ready`. If absent, skip bootstrap (not an error).
2. Query the board using `trello_get_active_board_info` or `trello_get_lists` to retrieve structural context.
3. Detect the active card:
   - Fetch cards in **In Progress** and **In Review** lists using `trello_get_cards_by_list_id`.
   - Match on labels (e.g., `sdd:apply`, `sdd:verify`) or on OpenSpec change references found in card comments/descriptions.
   - If multiple candidates exist, prefer the card with the most recent activity.
4. Present the recommended next task to the agent, including:
   - Card name, current list, current phase label.
   - Suggested action (continue phase, start next phase, review, etc.).
5. Feed structured primitives (card ID, list ID, label IDs) into the session's consensus check so subsequent capabilities can reference them without re-querying.
6. **Graceful degradation**: If any Trello MCP call fails, emit a warning to stderr and continue the session without Trello context. Log the failure to `.recipe/trello-mcp-workflow/warnings.log`.

---

## Capability: trello-card-linking

Link an OpenSpec change to a Trello card. Create a card from a template when no existing card matches.

### Trigger

OpenSpec change creation (`openspec-new-change` or `openspec-propose`).

### Steps

1. Detect whether a Trello card is already linked:
   - Check change metadata (e.g., `trello_card_id` field in the change folder).
   - Search recent comments on candidate cards for references to the change folder path.
2. **If a card exists**: Post a structured linking comment using `trello_add_comment` with:
   - Change name.
   - Change folder path (relative to project root, e.g., `openspec/changes/my-feature`).
   - List of expected artifacts: proposal, specs, design, tasks, apply, verify, archive.
3. **If no card exists**: Prompt the agent to create one from a bundled template:
   - Select template type: `feature`, `bug`, `spike`, `epic`, or `handoff`.
   - Create the card in `default_list` using `trello_add_card_to_list`.
   - Post the initial linking comment (same structure as step 2).
4. **Allow the agent to skip card creation** if the change is exploratory or the agent determines linking is unnecessary.
5. Store the card ID in change metadata for future reference.

### Templates

Templates are located at `ai-specs/recipes/trello-mcp-workflow/templates/` and are installed by the recipe:

| Template | File | Use Case |
|---|---|---|
| Feature | `card-feature.md` | New capabilities, commands, recipes, skills. |
| Bug | `card-bug.md` | Regressions or fixes with reproduction steps. |
| Spike | `card-spike.md` | Research with go/no-go conclusion. |
| Epic | `card-epic.md` | Grouping cards; no direct code implementation. |
| Handoff | `card-handoff.md` | Session continuity between agents. |

---

## Capability: trello-state-sync

Synchronize SDD phase transitions with Trello card position and labels.

### Trigger

SDD phase transitions: proposal→specs, specs→design, design→tasks, tasks→apply, apply→verify, verify→archive.

### Phase-to-List Mapping

| SDD Phase | Trello List |
|---|---|
| proposal | Backlog |
| specs | Design |
| design | Design |
| tasks | Ready |
| apply | In Progress |
| verify | In Review |
| archive | Done |

### Phase-to-Label Mapping

Each phase maps to a single label. On transition, the previous phase label is removed and the new one is applied:

| SDD Phase | Label |
|---|---|
| proposal | `sdd:proposal` |
| specs | `sdd:specs` |
| design | `sdd:design` |
| tasks | `sdd:tasks` |
| apply | `sdd:apply` |
| verify | `sdd:verify` |
| archive | `sdd:archive` |

### Steps

1. Identify the linked card (from session context or change metadata).
2. Resolve the target list ID by name using board lists (query with `trello_get_lists`).
3. Move the card to the target list using `trello_move_card`.
4. Replace the phase label on the card: remove the previous `sdd:*` label, add the new one using `trello_update_card_details`.
5. Post a phase-transition comment using `trello_add_comment`:
   ```
   Phase transition: {old_phase} → {new_phase}
   Timestamp: {ISO-8601 timestamp}
   ```

### Graceful Degradation

If the target list does not exist on the board, emit a warning, skip the move, and continue with the label update. Log the failure to `.recipe/trello-mcp-workflow/warnings.log`.

---

## Capability: trello-progress-comment

Post a structured progress comment on the linked card after apply and verify phases.

### Trigger

After successful completion of `apply` and `verify` SDD phases.

### Steps

1. Identify the linked card (from session context or change metadata).
2. Collect changed files from `apply-progress.md` in the change folder.
3. Read test results from `verify-report.md` in the change folder.
4. Assemble a structured comment:
   ```markdown
   ## SDD Progress: {phase}

   **Verdict**: {pass|fail}
   **Test Results**: {X/Y tests passing}
   **Files Changed**: {count} files — added: {list}, modified: {list}, removed: {list}
   **Archive**: {link to archive folder if archive phase, otherwise N/A}
   ```
5. Post the comment using `trello_add_comment`.

### Graceful Degradation

If `apply-progress.md` or `verify-report.md` are missing, post the comment with available data and mark missing sections as `unavailable`.

---

## Graceful Degradation (General)

- All runtime Trello failures emit warnings to stderr.
- Optionally log warnings to `.recipe/trello-mcp-workflow/warnings.log` with timestamp, capability, and error detail.
- Trello failures **never block agent progress**. The agent continues its SDD workflow regardless of Trello availability.
- If the Trello MCP server is unreachable, skip all Trello capabilities for the remainder of the session and log a single warning.

---

## MCP Tools Reference

| Tool | Used By | Purpose |
|---|---|---|
| `trello_get_active_board_info` | session-bootstrap | Retrieve board structure and current state. |
| `trello_get_lists` | session-bootstrap, state-sync | Resolve list names to IDs. |
| `trello_get_cards_by_list_id` | session-bootstrap | Fetch cards in active lists to detect the current card. |
| `trello_get_card` | card-linking, state-sync, progress-comment | Retrieve card details for matching and comment assembly. |
| `trello_add_card_to_list` | card-linking | Create a new card from a template. |
| `trello_add_comment` | card-linking, state-sync, progress-comment | Post structured comments on cards. |
| `trello_move_card` | state-sync | Move a card to a new list on phase transition. |
| `trello_update_card_details` | state-sync | Replace phase labels on a card. |
