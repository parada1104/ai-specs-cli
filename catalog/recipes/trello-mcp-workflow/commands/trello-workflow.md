# trello-workflow

Quick reference for the Trello MCP Workflow skill capabilities.

> Requires a reachable Trello MCP. If the server is not configured for this
> session, every capability below is unavailable: report that the tracker is not
> set up for this project and stop — do not offer to configure it, and do not
> hold any work back on it.

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