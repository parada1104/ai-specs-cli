# trello-workflow

Quick reference for the Trello MCP Workflow skill capabilities.

## Capabilities

| Capability | When to Invoke |
|---|---|
| `trello-session-bootstrap` | Session start; detect active card and recommend next task. |
| `trello-card-linking` | On OpenSpec change creation; link or create a Trello card and record it in the change's `## Tracker` section (`proposal.md` / `tasks.md`). |
| `trello-state-sync` | On SDD phase transitions; move card and update labels. |
| `trello-progress-comment` | After apply/verify phases; post structured progress update. |

## MCP Tools

| Tool | Purpose | Board Isolation |
|---|---|---|
| `trello_get_active_board_info` | Retrieve board structure. | Board guard verification. |
| `trello_get_lists` | Resolve list names to IDs. | Pass `boardId`. |
| `trello_get_cards_by_list_id` | Fetch cards in a list. | Pass `boardId`. |
| `trello_get_card` | Get card details. | Validate `idBoard` first. |
| `trello_add_card_to_list` | Create a new card. | Pass `boardId`. |
| `trello_add_comment` | Post a comment. | Validate `idBoard` first. |
| `trello_move_card` | Move card to another list. | Pass `boardId`. |
| `trello_update_card_details` | Update labels and card fields. | Pass `boardId`. |

## Forbidden and Restricted Tools

The following tools have board isolation restrictions enforced at the skill level:

| Tool | Restriction | Rationale |
|---|---|---|
| `trello_get_my_cards` | **Forbidden** | Returns cards across all boards; leaks scope. |
| `trello_list_boards` | **Forbidden** | Enumerates all accessible boards; leaks scope. |
| `trello_set_active_board` | **Restricted** — bootstrap only | Required once per session; later calls bypass guard. |

See the Board Isolation section in `skills/trello-mcp-workflow/SKILL.md` for full details.

## Card link section

Every active change must carry a `## Tracker` section in `proposal.md` (fallback `tasks.md`) with non-empty `card_id` (+ `url`). Exemption: `openspec/changes/<slug>/tracker.none`. See the skill's **Card link section (`## Tracker`)** for the canonical shape.

`## Tracker` is **artifact sugar only**: it is presentation, never authority, and it **never opens or binds** a ledger row. Rows are opened, linked, closed, and exempted only by explicit writes.

## Provider-backed lifecycle seam

A Trello card is created or linked through MCP, but the ledger item is bound locally. After the provider card exists, run the exact `bind` write — one locked transaction that opens-if-absent and links, with `kind=bind`, the card's `item_id`, `url`, `native_type` (`card`), a provider-neutral `state`, and the opaque `provider` snapshot (the observed list):

```bash
worktree-gate --ledger --checkpoint apply-start --ledger-mode <mode> --project-root <root> \
  --write '{"kind":"bind","item_id":"<24-hex>","url":"https://trello.com/c/...","native_type":"card","state":"in-progress","provider":{"list":"<list name>"}}'
```

A retried `bind` reports `unchanged` and a closed row is never reopened (D17). The write needs no `openspec/`/SDD/ODD artifact and makes no provider call; state lives in `<git-common-dir>/ai-specs/ledger/state.json`.

At a delivery, review, or merge lifecycle transition the agent/provider adapter reads the card through MCP (`trello_get_card`), produces the closed observation payload, and calls `worktree-gate --ledger --reconcile <observation> --reconcile-event <event>`. The closed payload has exactly `provider_id`, `scope`, `item_id`, `observed_at`, `event`, and `properties`. **Only `agree` is provider-backed compliance**; every other outcome is a pending decision. The Go ledger stays **provider-neutral** and performs **no provider write** — provider calls live only in the adapter.

**Ask path (`ledger_mode = ask`).** At cycle start the gate can return a `needs-item` verdict (decision `ask`). The agent's options are to **create or link** the provider card and then bind it locally with the write above, or to record the human's explicit decline once with `--decide ... --kind opt-out`. The decline is **lifecycle-scoped** and is **not repeated** at every checkpoint; it is never inferred and creates no item. The `ask` prompt is advisory: without a terminal the gate reports the pending `needs-item` state, records no opt-out, and proceeds without blocking.

## Tracker lifecycle checkpoints

The tracker ledger lifecycle is Plan Build-independent and needs no `openspec/` tree. Grade it directly with the tracker gate's shell host:

```bash
GATE=ai-specs/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh
bash "$GATE" --root "$PWD" --checkpoint pre-merge <slug>
bash "$GATE" --root "$PWD" --checkpoint archive-close <slug>
```

`--root` is required; the slug is optional. `--stage pre-merge|pre-archive` stays as a compatibility alias for `--checkpoint pre-merge|archive-close`. **`archive-close` is tracker item closure, not an OpenSpec archive** — it never infers a close from archive state. A provider recipe only maps native state through `[config.reconcile]`; it does not enable, configure, or implement the ledger.

The Tracker host is **advisory**: it reports a `block` / `ask` / `needs-item` verdict on stderr and exits `0`, so a non-zero Tracker verdict never blocks a merge, a close, or unrelated source work. Only the Plan Build `work-start` checkpoint may still block, separately.

## Phase Mappings

### Phase → List

| SDD Phase | Trello List |
|---|---|
| proposal | Backlog |
| specs | Design |
| design | Design |
| tasks | Ready |
| apply | In Progress |
| verify | In Review |
| archive | Done |

### Phase → Label

| SDD Phase | Label |
|---|---|
| proposal | `sdd:proposal` |
| specs | `sdd:specs` |
| design | `sdd:design` |
| tasks | `sdd:tasks` |
| apply | `sdd:apply` |
| verify | `sdd:verify` |
| archive | `sdd:archive` |

## Card Templates

| Type | Template | Use Case |
|---|---|---|
| feature | `card-feature.md` | New capabilities, commands, skills. |
| bug | `card-bug.md` | Regressions or fixes with reproduction steps. |
| spike | `card-spike.md` | Research with go/no-go conclusion. |
| epic | `card-epic.md` | Grouping cards; no direct code. |
| handoff | `card-handoff.md` | Session continuity between agents. |