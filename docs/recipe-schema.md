# recipe.toml Schema

A recipe is a named, versioned bundle of AI agent primitives that `ai-specs sync` can materialize into a project.

## Directory layout

```
catalog/recipes/<id>/
├── recipe.toml
├── skills/         # optional — bundled skills
├── commands/       # optional — slash commands
├── templates/      # optional — file templates
└── docs/           # optional — documentation files
```

## `[recipe]` table

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Recipe identifier; matches directory name |
| `name` | string | yes | Human-readable name |
| `description` | string | yes | Short description |
| `version` | string | yes | Exact version string |
| `author` | string | no | Author or organization |
| `license` | string | no | SPDX license identifier |

## `[provides]` table

### `skills`

Array of objects:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Skill identifier |
| `source` | string | yes | `"bundled"` or `"dep"` |
| `url` | string | yes for `dep` | Git URL to clone |
| `path` | string | no | Subdirectory inside the repo |

### `commands`

Array of objects:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Command identifier |
| `path` | string | yes | Relative path to `.md` file inside recipe directory |

### `mcp`

Array of tables with MCP server configuration:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | MCP preset identifier |
| ... | any | no | Any MCP fields (`command`, `args`, `env`, etc.) |

### `templates`

Array of tables:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `source` | string | yes | Relative path inside recipe directory |
| `target` | string | yes | Relative path inside project root |
| `condition` | string | no | `"not_exists"` (default) — skip if target already exists |

### `docs`

Array of tables:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `source` | string | yes | Relative path inside recipe directory |
| `target` | string | yes | Relative path inside project root |

## Manifest declaration

In `ai-specs/ai-specs.toml`:

```toml
[recipes.<id>]
enabled = true
version = "1.0.0"
```

- `enabled` (boolean, required): must be `true` for the recipe to materialize
- `version` (string, required): exact version that must match `recipe.toml`

## Version pinning

If the manifest pin does not match the catalog `recipe.toml` version, `ai-specs sync` fails with an explicit error.

## Conflict detection

If two enabled recipes declare the same primitive ID (skill, command, or MCP), sync fails with an explicit error naming both recipes and the conflicting ID.

---

# recipe.toml V2 Additions

V2 tables are **strictly optional**. A V1 recipe requires zero changes to continue working.

## `[[capabilities]]` table

Declares capabilities that this recipe provides. Capabilities are abstract identifiers that other recipes or the manifest can bind to.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Capability identifier; must be non-empty kebab-case |

Example:

```toml
[[capabilities]]
id = "tracker"

[[capabilities]]
id = "canonical-memory"
```

Duplicate capability IDs within the same recipe cause a validation error.

## `[config]` schema declaration

Defines configuration fields that the recipe expects. Values can be overridden per-project in the manifest.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `required` | boolean | yes | Whether the field must be provided |
| `type` | string | no | Hint for the expected type (`string`, `integer`, `boolean`) |
| `default` | any | no | Default value when not overridden |

Example:

```toml
[config.timeout]
required = false
type = "integer"
default = 30

[config.board_id]
required = true
type = "string"
```

Missing `required` or invalid types cause a validation error.

## `[[hooks]]` lifecycle events

Hooks run after all primitives are materialized.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `event` | string | yes | Lifecycle event name (`on-sync`) |
| `action` | string | yes | Action to execute (`validate-config`) |

Example:

```toml
[[hooks]]
event = "on-sync"
action = "validate-config"
```

Unknown actions emit a warning and are skipped; sync continues.

---

# Manifest V2 Additions

## `[[bindings]]` table

Explicitly binds a capability to a recipe. Resolves ambiguity when multiple enabled recipes declare the same capability.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `capability` | string | yes | Capability ID to bind |
| `recipe` | string | yes | Recipe ID that provides the capability |

Example:

```toml
[[bindings]]
capability = "tracker"
recipe = "recipe-a"
```

Duplicate explicit bindings for the same capability cause sync to fail.

## `[recipes.<id>.config]` override syntax

Overrides recipe config defaults per-project.

Example:

```toml
[recipes.my-recipe]
enabled = true
version = "1.0.0"

[recipes.my-recipe.config]
timeout = 60
board_id = "abc123"
```

## Auto-binding behavior

If a capability is declared by **exactly one** enabled recipe and has no explicit binding, it is automatically bound to that recipe.

If **multiple** enabled recipes declare the same capability and there is no explicit binding, sync emits a **warning** and the capability remains unbound. Sync does **not** fail solely due to ambiguity.

## Config merge rules

During sync, the final config for each recipe is computed as:

1. Start with defaults from `recipe.toml` `[config]` schema (`default` values).
2. Overlay values from `ai-specs.toml` `[recipes.<id>.config]`.
3. If any field with `required = true` is still missing, sync fails with an explicit error naming the field.

Unknown config keys in the manifest emit a warning and are ignored.

## Backward compatibility

V2 tables (`[[capabilities]]`, `[[hooks]]`, `[config]`, `[[bindings]]`, `[recipes.<id>.config]`) are all optional:

- A V1 `recipe.toml` without V2 tables parses and materializes exactly as before.
- A V1 `ai-specs.toml` without `[[bindings]]` or recipe `config` proceeds normally.
- Existing primitive conflict detection (skills, commands, MCP) is unchanged.
