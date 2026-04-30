# Trello MCP Workflow

Automated Trello board integration for ai-specs SDD workflows.

## Installation

```bash
ai-specs recipe add trello-mcp-workflow
```

This installs:

- **Skill**: `trello-mcp-workflow` — runtime skill with 4 capabilities (session-bootstrap, card-linking, state-sync, progress-comment).
- **Templates**: `card-feature.md`, `card-bug.md`, `card-spike.md`, `card-epic.md`, `card-handoff.md` — Trello card description templates.
- **Command**: `trello-workflow` — quick-reference command file for agents.

## Configuration

Add to `ai-specs/recipes/trello-mcp-workflow/config.json` or the recipe section in `ai-specs.toml`:

| Field | Required | Default | Description |
|---|---|---|---|
| `board_id` | Yes | — | Trello board ID for the project. |
| `default_list` | No | `In Progress` | List name where new cards are created. |
| `epic_list` | No | `Epic` | List name where epic-type cards are placed. |

### Example

```json
{
  "board_id": "69ec0a2099ea20956e371d62",
  "default_list": "In Progress",
  "epic_list": "Epic"
}
```

## What Gets Installed

| Asset | Target Location |
|---|---|
| Skill | `ai-specs/skills/trello-mcp-workflow/SKILL.md` |
| Feature template | `ai-specs/recipes/trello-mcp-workflow/templates/card-feature.md` |
| Bug template | `ai-specs/recipes/trello-mcp-workflow/templates/card-bug.md` |
| Spike template | `ai-specs/recipes/trello-mcp-workflow/templates/card-spike.md` |
| Epic template | `ai-specs/recipes/trello-mcp-workflow/templates/card-epic.md` |
| Handoff template | `ai-specs/recipes/trello-mcp-workflow/templates/card-handoff.md` |
| Command | `ai-specs/commands/trello-workflow.md` |

Templates are installed only if they do not already exist (condition: `not_exists`).

## Rollback

```bash
ai-specs recipe remove trello-mcp-workflow
```

This removes installed assets. User-created templates are preserved (only recipe-provided defaults are removed).

## Graceful Degradation

All Trello MCP failures emit warnings to stderr and are optionally logged to `.recipe/trello-mcp-workflow/warnings.log`. Trello failures **never block agent progress** — the SDD workflow continues regardless of Trello availability.

If the Trello MCP server is unreachable, the skill skips all Trello capabilities for the remainder of the session and logs a single warning.