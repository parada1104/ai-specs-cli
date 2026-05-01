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
- `[sdd]` (optional)

Recipe-specific schema details live in [`docs/recipe-schema.md`](recipe-schema.md).
SDD provider behavior lives in [`docs/ai/sdd.md`](ai/sdd.md).

Omission of `[sdd]` remains valid for projects not using SDD.

## Compatibility rules

Conservative compatibility rules in V1:

- Missing `[agents]`, `[[deps]]`, and `[mcp]` remain valid and normalize to stable defaults.
- `project.subrepos` remains validated by the existing root target resolver.
- MCP `env` is the canonical field name.
- MCP `environment` is still accepted as a tolerated input alias and normalizes to `env`.
- `env = ["VAR"]` is treated as an env-reference form and normalizes to `{ VAR = "$VAR" }`.
- `env = { VAR = "literal" }` is preserved as a literal mapping.
- A manifest without `[recipes.*]`, `[recipes.<id>.config]`, or `[[bindings]]` remains valid.

## Field classification

| Surface | Fields | Status |
|---------|--------|--------|
| `[project]` | `name` | optional, default `""` |
| `[project]` | `subrepos` | optional, default `[]`, validated as root-relative target paths |
| `[agents]` | `enabled` | optional, default `[]` |
| `[[deps]]` | `id`, `source` | only required minimum fields |
| `[[deps]]` | `path`, `scope`, `auto_invoke`, `license`, `vendor_attribution` | optional passthrough fields consumed by vendoring/rendering |
| `[mcp.<name>]` | `command` | optional |
| `[mcp.<name>]` | `args` | optional, default `[]` |
| `[mcp.<name>]` | `env` | optional canonical field, default `{}` |
| `[mcp.<name>]` | `environment` | tolerated input alias of `env` |
| `[mcp.<name>]` | `timeout` | optional |
| `[mcp.<name>]` | `enabled` | tolerated passthrough field |
| `[recipes.<id>]` | `enabled` | required; boolean - must be `true` to materialize |
| `[recipes.<id>]` | `version` | required; exact string matching `recipe.toml` version |
| `[recipes.<id>.config]` | `<key> = <value>` | optional per-recipe overrides; unknown keys warn and are ignored |
| `[[bindings]]` | `capability`, `recipe` | optional explicit capability binding |
| `[sdd]` | `enabled`, `provider`, `artifact_store` | optional; `provider` = `openspec` in v1 |

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

Overrides defaults from the recipe's `[config]` schema.

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

### `[sdd]`

Optional OpenSpec onboarding block.

```toml
[sdd]
enabled = true
provider = "openspec"
artifact_store = "filesystem"
```

Use [`docs/ai/sdd.md`](ai/sdd.md) for provider workflow, generated commands,
and `artifact_store` semantics.

## Example manifest

```toml
[project]
name = "my-project"

[agents]
enabled = ["claude", "cursor", "opencode"]

[[deps]]
id = "context-precedence"
source = "https://github.com/example/skills"

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

[sdd]
enabled = true
provider = "openspec"
artifact_store = "filesystem"
```

## Out of scope

Out of scope for this V1 contract (explicitly deferred to future changes):

- precedence / merge policy beyond the currently implemented runtime behavior
- `[memory]` (distinct from `[sdd].artifact_store = memory`)

## See also

- [`templates/ai-specs.toml.tmpl`](../templates/ai-specs.toml.tmpl)
- [`docs/recipe-schema.md`](recipe-schema.md)
- [`docs/ai/sdd.md`](ai/sdd.md)
