# Trello MCP Workflow

Automated Trello board integration for ai-specs SDD workflows.

## Installation

```bash
ai-specs recipe add trello-mcp-workflow
ai-specs recipe init trello-mcp-workflow
```

This installs:

- **Skill**: `trello-mcp-workflow` — runtime skill with 4 capabilities (session-bootstrap, card-linking, state-sync, progress-comment).
- **Templates**: `card-feature.md`, `card-bug.md`, `card-spike.md`, `card-epic.md`, `card-handoff.md` — Trello card description templates.
- **Command**: `trello-workflow` — quick-reference command file for agents.

`ai-specs recipe add` only declares the recipe in `ai-specs/ai-specs.toml` and writes placeholder config.
`ai-specs sync` materializes the bundled assets.
`ai-specs recipe init trello-mcp-workflow` prints a read-only setup brief so the
project can confirm `board_id`, list mappings, and MCP readiness before sync.

## Configuration

Add configuration under `[recipes.trello-mcp-workflow.config]` in `ai-specs/ai-specs.toml`:

| Field | Required | Default | Description |
|---|---|---|---|
| `board_id` | Yes | — | Trello board ID for the project. |
| `default_list` | No | `In Progress` | List name where new cards are created. |
| `epic_list` | No | `Epic` | List name where epic-type cards are placed. |

### Example

```toml
[recipes.trello-mcp-workflow]
enabled = true
version = "1.0.0"

[recipes.trello-mcp-workflow.config]
board_id = "69ec0a2099ea20956e371d62"
```
