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
    - "Reconciling the ledger card against live Trello state"
---

# Trello MCP Workflow

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
| `review_list` | No | `Review` | List a card must reach when the `review` lifecycle event reconciles. |
| `done_list` | No | `Done` | List a card must reach when the `merge` lifecycle event reconciles (the default merge target). |
| `published_list` | No | — | Optional list that means the merge was also published/released. When set, the `merge` event reconciles against it instead of `done_list`. No default is invented: leave it unset when the board has no such list. |
| `gate_mode` | No | `warn` | Tracker card gate: `off` / `warn` / `always`. |
| `reconcile` | No | — | Declarative remote-reconciliation mapping (`scope_field`, `max_age_seconds`, `expectations`). The recipe declares the lifecycle mapping by default (delivery/review/merge); a project `[recipes.trello-mcp-workflow.config.reconcile]` block, when present, overrides it. Re-run `ai-specs sync` after recipe changes to propagate defaults. |

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

**The ledger records the item; the artifact only presents it.** An item is opened,
linked, closed, and exempted by explicit writes — a parsed `## Tracker` never opens
one, and grading never writes:

```bash
worktree-gate --ledger --checkpoint apply-start --write '{"kind":"open"}'
worktree-gate --ledger --checkpoint apply-start \
  --write '{"kind":"link","item_id":"<24-hex>","url":"https://trello.com/c/...","native_type":"card","state":"in-progress"}'
```

A failed write exits `2`, persists nothing, and prints no verdict JSON; a retried
`open` reports `already-open` instead of creating a second item. For `tracker.none`,
the human/agent records the exemption with an explicit write so every checkpoint
honors it and it never registers as a conflict:

```bash
worktree-gate --ledger --checkpoint apply-start \
  --write '{"kind":"exempt","reason":"<one line>"}'
```

The file itself is presentation/evidence only — no host auto-records it (R1); hosts
treat it as blank `code` evidence and never create, modify, or delete the file.
Removing the file does not revoke a recorded exemption — reopen evidence with
`worktree-gate --ledger --checkpoint <name> --decide '{"kind":"adjudicate","choice":"<side>"}'`.
Hosts grade with `local`/`code`/`git` evidence built from local facts; the `remote`
side is unwired in this slice.

### Tracker lifecycle checkpoints (`pre-merge`, `archive-close`)

The ledger's lifecycle is **Plan Build-independent**: this recipe never needs an
`openspec/` tree to grade a tracker checkpoint. Run the Tracker-domain host
directly — this is the generic lifecycle command, and a future Jira/Linear recipe
reuses it unchanged:

```bash
python3 "${AI_SPECS_HOME:-$HOME/.ai-specs}/lib/_internal/tracker_ledger_host.py" \
  <slug> --root "$PWD" --checkpoint pre-merge
python3 "${AI_SPECS_HOME:-$HOME/.ai-specs}/lib/_internal/tracker_ledger_host.py" \
  <slug> --root "$PWD" --checkpoint archive-close
```

`--root` is required and is the resolved project root, never the process cwd. The
slug is optional: a tracker-only project has no change tree, and the binding
witness supplies the identity. `--stage pre-merge|pre-archive` remains a
compatibility alias for `--checkpoint pre-merge|archive-close`.

**`archive-close` is tracker item closure, not an OpenSpec archive.** It asks the
one Go predicate whether the bound tracker item can be closed. It never moves
`openspec/changes/<slug>/`, never reads an OpenSpec archive to infer or select
closure, and never writes. The host is acquisition only: it resolves the mode,
builds local evidence through `ledger_bridge.py`, calls
`worktree-gate --ledger --checkpoint <name>`, and maps the verdict (`0`
allow/ask/dormant/unevaluable, nonzero when blocked). No provider write, no
network, no second grader.

The host and the ledger belong to the generic Tracker domain, not to Trello. A
provider recipe contributes only its `[config.reconcile]` native-state mapping;
it never enables, configures, or implements the ledger, and it never grades.

### Remote reconciliation (explicit, off the hot path)

The grade above compares local facts. Reconciling against what Trello *actually*
reports is a separate, explicit comparison: the agent acquires the remote data
through the Trello MCP tools and the Go gate compares it against the bound card and
the recipe-declared expectations. The comparison is never automatic, never a
provider write, and never resolves a decision — it rides along as a sidecar and the
graded exit code is unchanged.

**1. Observe through MCP (board isolation still applies).** After the board guard,
read the linked card with `trello_get_card(cardId, fields="idBoard,idList,name")`,
validate `idBoard` against the configured `board_id`, and resolve the card's list
name with `trello_get_lists(boardId: <board_id>)`. Report only what you actually
read — never the state you expected to find.

**2. Write the observation payload outside the repository** (for example to
`$(mktemp)`), so no workspace file changes and the worktree gate stays untouched:

```json
{
  "provider_id": "trello-mcp-workflow",
  "scope": "<board_id>",
  "item_id": "<24-hex card id>",
  "observed_at": "<RFC3339 UTC stamp of this read>",
  "event": "<the event you observed, e.g. delivery>",
  "properties": { "list": "<list name you read>" }
}
```

The payload is a closed shape: `provider_id`, `scope`, `item_id`, `observed_at`,
`event`, `properties` are the only accepted keys, unknown keys are rejected, and
values must be the types shown. `observed_at` is the moment of the read, not a
remembered or inferred time. The payload is read under a fixed size budget
(1048576 bytes): a larger file is reported as `observation-invalid`, never parsed.

**3. Compare with the gate**, naming the event you are asking about
(`--reconcile-event`). Use the stamped/verified gate binary path (`$WORKTREE_GATE_BIN`
or the CLI cache path the hooks use). The recipe declares the supported lifecycle
mapping by default — `delivery` → `default_list` (In Progress), `review` →
`review_list` (Review), `merge` → `done_list` (Done), or → `published_list` when the
project configured that optional list — so a synced project reconciles without
per-project configuration; the project's `[config.reconcile]` block, when present,
overrides it:

```bash
worktree-gate --ledger --checkpoint apply-start --project-root "$PWD" \
  --reconcile "$OBSERVATION" --reconcile-event delivery
```

Reconciliation is read-only: the gate refuses `--reconcile` combined with `--write`
or `--decide` with exit 2 before it touches the store, it never writes the store —
a conflicting grade's conflict snapshot is suppressed instead of recorded and the
suppression is reported on stderr — and it never resolves a decision or changes the
graded exit code.

**4. Read the sidecar** (top-level `reconcile` key of the verdict JSON):

| Outcome | Meaning | Next step |
|---|---|---|
| `agree` | Every declared property matches for the requested event, and the observation reported that event. | Conditional expectation met. Still not proof of delivery. |
| `unconfigured` | The manifest, the recipe's `[config.reconcile]` mapping, or a config value it selects is missing, or the recipe is `enabled = false`. | Re-run `ai-specs sync` so recipe defaults propagate; read `detail`. A readable, explicitly disabled recipe grants no authority. |
| `unmapped-event` | No expectation is declared for the requested event. | Pending / unreconciled: declare the mapping or drop the request. |
| `event-mismatch` | The observation reports a different (or no) event than the one requested. | Never treat the requested event as observed; re-read or re-request. |
| `observation-invalid` | The payload is missing, malformed, oversized, or carries unknown keys. | Fix the acquisition; nothing was compared. |
| `unbound-identity`, `identity-mismatch` | No bound card, or the observation belongs to another provider/scope/item. | Fix the binding or the observation; wrong scope is never compared. |
| `stale-observation`, `future-observed-at`, `invalid-observed-at`, `invalid-window`, `invalid-clock` | The observation is older than the declared window, dated in the future, unparsable, or the freshness window is undeclared. | Re-read; bind a positive `max_age_seconds` no larger than `9223372036`. There is no freshness default. |
| `missing-property`, `property-mismatch` | A declared property was not reported or holds another value. | Report the discrepancy and present it as a pending explicit decision. |

Only `agree` is agreement. A selected event is never evidence that the event
happened, a local property match is not a merge/release verification, and a
non-agreeing outcome is information to report — it does not block unrelated work and
it creates no provider write.

**Pending decisions (every non-agree outcome).** Agreement is a conditional match
of the declared expectations for the requested event, not proof that the event was
delivered. Every non-agreeing outcome above is a *pending explicit decision*: the
agent presents it and the human resolves it. Nothing is resolved by the agent, by a
provider write, or by treating silence as consent. Only the **disputed action** (for
example claiming the card is merged/released, or recording local state as done) is
suspended; unrelated work in the session continues normally.

For one non-agree outcome, the agent presents, in one message:

- The `outcome` and what it means (the table above).
- The event asked for (`requested_event`) against the event actually observed
  (`observed_event`); an unobserved event is never the requested one.
- The bound identity this comparison was about: `provider_id`, `scope`, `item_id`.
  An empty `item_id` means nothing was bound — never borrow another card's id.
- `observed_at` and its age against the declared `max_age_seconds`; an age the
  agent cannot compute is reported as unknown, not as fresh.
- The `findings` list verbatim when it carries entries (`property`, `expected`,
  `observed`, `status`), or the bounded `detail` string for the wiring outcomes
  (`unconfigured`, `unmapped-event`, `observation-invalid`) that carry none.

Then it asks **exactly one** `ask_user` question offering exactly these
resolutions:

| Resolution | What it does |
|---|---|
| **Fix remote state and re-observe** | Correct the card in Trello, then re-read and re-run the `--reconcile` comparison. The agent never edits the card on its own initiative. |
| **Record observed state as truth** | The human decides what the observed state means and the agent performs the explicit ledger write (`--write` / `--decide`) that records it. Never inferred, never automatic. |
| **Leave pending** | No resolution now: report the item as unreconciled and continue unrelated work. It is re-raised on the next explicit reconciliation, not silently dropped. |

Hard rules:

- **No inferred consent.** An unanswered, dismissed, or defaulted prompt is not a
  yes. `record observed state as truth` requires an explicit human selection; the
  agent never selects it for the human, and it never re-asks with a leading option.
- **No provider mutation before an explicit human choice.** Until the human
  chooses, the agent performs no Trello create/update/move/comment/label call and
  no provider-adjacent write. Running the comparison is not consent.
- **Unrelated work continues.** A pending decision suspends only the disputed
  action; it never blocks the session, other tasks, or the graded exit code.
- **Headless or unavailable UI leaves the decision pending.** When no interactive
  prompt is available, the agent does not guess a resolution: it reports the
  outcome as pending/unreconciled with the same fields above and leaves it for a
  session that can ask.
- **Agreement is conditional expectation match, not delivery proof.** `agree` means
  only that the declared properties matched for the requested event; it is not
  evidence of a merge, a release, or a delivery.
- **Closed items after a merge are locally corroborated.** D17 keeps a closed row
  out of the primary slot, so a merge comparison has no bound card by default. It
  binds one only when the observation's item id matches exactly one locally stored
  closed row for the current identity: that corroborated row is compared read-only,
  never reopened, and never selected for new work. An id with no local match, or
  with more than one, stays `unbound-identity` and is presented as a pending
  decision — never guessed and never trusted from the provider alone.

Reading the mapping is bounded acquisition: the manifest must be a plain file, and
the standard parser runs under a deadline with capped output. A missing parser, a
hung parser, an oversized result, a non-TOML manifest, and an invalid mapping all
reach the sidecar as `unconfigured` with a bounded `detail` — never as raw parser
output.

**Declared mapping.** The recipe declares the lifecycle mapping and its
conservative list defaults. During `ai-specs sync`, the mapping and the config
values it references are stamped into `[recipes.trello-mcp-workflow.config]`
when absent; explicit project values remain overrides and are never replaced.
A project that has not been synced after this recipe version can temporarily
report `unconfigured`; re-run sync rather than authoring a reconcile block by
hand:

```toml
[recipes.trello-mcp-workflow.config]
default_list = "In Progress"
review_list = "Review"
done_list = "Done"
# Optional; set it only when the board has a Published-style list. Omit it and the
# merge event keeps reconciling against done_list.
# published_list = "Published"

[recipes.trello-mcp-workflow.config.reconcile]
scope_field = "board_id"
max_age_seconds = 900

[[recipes.trello-mcp-workflow.config.reconcile.expectations]]
event = "delivery"
property = "list"
config_field = "default_list"

[[recipes.trello-mcp-workflow.config.reconcile.expectations]]
event = "review"
property = "list"
config_field = "review_list"

[[recipes.trello-mcp-workflow.config.reconcile.expectations]]
event = "merge"
property = "list"
config_field = "done_list"
config_field_when_set = "published_list"
```

Each expectation means "when the caller asks about `event`, the property `property`
must show the value configured in `config_field`". `config_field_when_set` is an
optional conditional target: the merge expectation compares `published_list` when
the project configured that list, and `done_list` otherwise. The condition is
declared by the recipe, so a project still configures list names only — it never
hand-authors which property maps to which list, and no Published list is invented.
`max_age_seconds` must be positive and at most `9223372036`; anything else is
`unconfigured`, never a clamped default. When there is no open card, a merge
comparison binds a closed row only through the locally corroborated id described
above; the gate never guesses a closed row.

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
