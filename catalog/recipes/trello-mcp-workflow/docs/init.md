# Trello Recipe Init

Use this brief to prepare project-specific Trello setup before `ai-specs sync`.

Ask or confirm these values:

- `board_id`: the Trello board ID that this project should use.
- `default_list`: the list name for day-to-day implementation work.
- `epic_list`: the list name reserved for epic cards.

Review the project's existing `[mcp.trello]` declaration before proposing changes.
If Trello MCP is missing, propose a reviewable manifest change under `[mcp.trello]`
instead of writing recipe config keys that duplicate MCP ownership.

When the project already declares `[recipes.trello-mcp-workflow.config]`, update the
existing keys instead of adding duplicates.

Template targets under `ai-specs/recipes/trello-mcp-workflow/templates/` are
reviewable defaults for card descriptions. If a target already exists, prefer
update/skip guidance rather than overwrite guidance.
