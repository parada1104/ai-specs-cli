# ai-specs.toml Reference

`ai-specs/ai-specs.toml` in the project root is the ONLY V1 source of truth.

This document is the canonical manifest reference for the current `ai-specs`
runtime. It documents only fields and behaviors that are already implemented by
the current runtime, sync pipeline, and current specs.

## Canonical V1 surface

The manifest surface supported today is:

- `[project]`
- `[agents]`
- `[[deps]]`
- `[mcp.<name>]`
- `[recipes.<id>]`
- `[recipes.<id>.config]`
- `[[bindings]]`
- `[brief]`
- `[brief.mcp_descriptions]`
Recipe-specific schema details live in [`docs/recipe-schema.md`](recipe-schema.md).

## Compatibility rules

Conservative compatibility rules in V1:

- Missing `[agents]`, `[[deps]]`, and `[mcp]` remain valid and normalize to stable defaults.
- `project.subrepos` remains validated by the existing root target resolver.
- MCP `env` is the canonical field name.
- MCP `environment` is still accepted as a tolerated input alias and normalizes to `env`.
- `env = ["VAR"]` is treated as an env-reference form and normalizes to `{ VAR = "$VAR" }`.
- `env = { VAR = "literal" }` is preserved as a literal mapping.
- Env reference values accept both `$VARIABLE_NAME` (canonical) and `${VARIABLE_NAME}` (tolerated fallback). Both produce the same rendered output per agent: `{env:VAR}` for OpenCode and `${VAR}` for Claude/Cursor. Variable names must follow shell convention (`[A-Z_][A-Z0-9_]*`); other strings pass through as literals.
- A manifest without `[recipes.*]`, `[recipes.<id>.config]`, or `[[bindings]]` remains valid.

## Field classification

| Surface | Fields | Status |
|---------|--------|--------|
| `[project]` | `name` | optional, default `""` |
| `[project]` | `subrepos` | optional, default `[]`, validated as root-relative target paths |
| `[agents]` | `enabled` | optional, default `[]` |
| `[[deps]]` | `id`, `source` | only required minimum fields |
| `[[deps]]` | `path`, `scope`, `auto_invoke`, `license`, `vendor_attribution`, `version` | optional passthrough fields consumed by vendoring/rendering |
| `[mcp.<name>]` | `command` | optional |
| `[mcp.<name>]` | `args` | optional, default `[]` |
| `[mcp.<name>]` | `env` | optional canonical field, default `{}` |
| `[mcp.<name>]` | `environment` | tolerated input alias of `env` |
| `[mcp.<name>]` | `timeout` | optional |
| `[mcp.<name>]` | `enabled` | tolerated passthrough field |
| `[recipes.<id>]` | `enabled` | required; boolean — must be `true` to materialize |
| `[recipes.<id>]` | `version` | required; exact string matching `recipe.toml` version |
| `[recipes.<id>.config]` | `<key> = <value>` | optional per-recipe overrides; unknown keys warn and are ignored |
| `[[bindings]]` | `capability`, `recipe` | optional explicit capability binding |
| `[brief]` | `intro` | optional; multi-line string; rendered as a `>` blockquote after H1 |
| `[brief]` | `purpose` | optional; string; one-line project description in `## Project` |
| `[brief]` | `runtime_flow` | optional; array of strings; bullet list in `## Runtime Flow` |
| `[brief]` | `context_sources` | optional; array of strings; bullet list in `## Context Sources` |
| `[brief]` | `conflict_policy` | optional; array of strings; bullet list in `## Conflict Policy` |
| `[brief]` | `workflow_rules` | optional; array of strings; bullet list in `## Workflow Rules` |
| `[brief]` | `useful_commands` | optional; array of strings; extra bullet items appended to `## Useful Commands` |
| `[brief.mcp_descriptions]` | `<server-name>` | optional per-server description appended to each MCP entry in `## Runtime MCPs` |

## Manifest sections

### `[project]`

Project metadata owned by the repo.

```toml
[project]
name = "my-project"
subrepos = ["packages/app", "packages/docs"]
```

### `[agents]`

Controls which agent-specific derived files are generated.

```toml
[agents]
enabled = ["claude", "cursor", "opencode"]
```

### `[[deps]]`

Declares vendored external skills.

```toml
[[deps]]
id = "context-precedence"
source = "https://github.com/example/skills"
path = "skills/context-precedence"
scope = ["root"]
version = "1.0.0"
```

### `[mcp.<name>]`

Declares MCP server config that will be rendered into agent-native files.

```toml
[mcp.openmemory]
command = "npx"
args = ["-y", "@openmemory/mcp"]
env = ["OPENMEMORY_API_KEY"]
timeout = 30000
```

### `[recipes.<id>]`

Enables a named recipe and pins its catalog version.

```toml
[recipes.trello-mcp-workflow]
enabled = true
version = "1.0.0"
```

### `[recipes.<id>.config]`

Overrides defaults from the recipe's `[config]` schema. See
[`docs/recipe-schema.md`](recipe-schema.md) for the recipe-level `[config]` contract.

```toml
[recipes.trello-mcp-workflow.config]
board_id = "abc123"
default_list = "In Progress"
```

### `[[bindings]]`

Explicitly binds a capability to one recipe when multiple enabled recipes could
provide it.

```toml
[[bindings]]
capability = "trello-card-linking"
recipe = "trello-mcp-workflow"
```

If exactly one enabled recipe declares a capability and no explicit binding is
present, sync auto-binds it. If multiple enabled recipes declare the same
capability and no binding is present, sync warns and leaves it unbound.

## Example manifest

```toml
[project]
name = "my-project"

[agents]
enabled = ["claude", "cursor", "opencode"]

[[deps]]
id = "context-precedence"
source = "https://github.com/example/skills"
version = "1.0.0"

[mcp.openmemory]
command = "npx"
args = ["-y", "@openmemory/mcp"]
env = ["OPENMEMORY_API_KEY"]

[recipes.trello-mcp-workflow]
enabled = true
version = "1.0.0"

[recipes.trello-mcp-workflow.config]
board_id = "abc123"

[[bindings]]
capability = "trello-card-linking"
recipe = "trello-mcp-workflow"
```

### `[brief]`

Supplies prose content for the generated `AGENTS.md` runtime brief. Structured
values — board ID, integration branch, test command, vault scope — come from
recipe configs; only prose with no structured home goes here.

All keys are optional. Sections are omitted from the rendered brief when the
corresponding key is absent.

```toml
[brief]
intro = """
Canonical runtime context for agents: project identity, MCPs,
context sources, safety rules, and workflow conventions.
"""
purpose = "per-project AI harness for configuration, MCPs, recipes, memory, and tracker integration."
runtime_flow = [
  "A session works on one explicit user request or Trello card.",
  "Artifact phases run in a dedicated worktree when they write files.",
]
context_sources = [
  "Trello is the source of truth for work state and dependencies.",
  "Vault is the canonical note-taker for decisions and handoffs.",
]
conflict_policy = [
  "Current explicit human instruction controls immediate scope unless it conflicts with safety or secrets.",
  "Proposed agent plans are lowest authority until accepted and recorded in Trello, Vault, docs, or code.",
]
workflow_rules = [
  "Do not merge or push to the integration branch without explicit human instruction.",
  "Create a dedicated worktree for changes that write artifacts or modify code.",
]
useful_commands = [
  "Inspect the active Trello card before resuming work.",
]
```

### `[brief.mcp_descriptions]`

Keyed by MCP server name. Each value is a short description appended to that
server's entry in the `## Runtime MCPs` section of the generated brief.

```toml
[brief.mcp_descriptions]
trello = "project tracking through the Roadmap board."
engram = "operational/session memory (global MCP)."
```

## Out of scope

Out of scope for this V1 contract (explicitly deferred to future changes):

- precedence / merge policy beyond the currently implemented runtime behavior

## See also

- [`templates/ai-specs.toml.tmpl`](../templates/ai-specs.toml.tmpl)
- [`docs/recipe-schema.md`](recipe-schema.md)
