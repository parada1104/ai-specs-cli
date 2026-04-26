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
