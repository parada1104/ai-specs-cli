# trello-workflow

Quick reference for the Trello MCP Workflow skill capabilities.

## Capabilities

| Capability | When to Invoke |
|---|---|
| `trello-session-bootstrap` | Session start; detect active card and recommend next task. |
| `trello-card-linking` | On OpenSpec change creation; link or create a Trello card. |
| `trello-state-sync` | On SDD phase transitions; move card and update labels. |
| `trello-progress-comment` | After apply/verify phases; post structured progress update. |

## MCP Tools

| Tool | Purpose |
|---|---|
| `trello_get_active_board_info` | Retrieve board structure. |
| `trello_get_lists` | Resolve list names to IDs. |
| `trello_get_cards_by_list_id` | Fetch cards in a list. |
| `trello_get_card` | Get card details. |
| `trello_add_card_to_list` | Create a new card. |
| `trello_add_comment` | Post a comment. |
| `trello_move_card` | Move card to another list. |
| `trello_update_card_details` | Update labels and card fields. |

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