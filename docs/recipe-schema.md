# recipe.toml Schema

This document is the canonical reference for `recipe.toml` and related recipe
manifest extensions. For the root manifest contract, see
[`docs/ai-specs-toml.md`](ai-specs-toml.md).

A recipe is a named, versioned bundle of AI agent primitives that `ai-specs sync` can materialize into a project.

For normal consumer projects, `catalog/recipes/` is not part of the project workspace. The directory layout below describes recipe authoring inside the CLI's bundled catalog. Consumer projects declare recipes in `ai-specs/ai-specs.toml`, and the CLI resolves them from its own catalog during `recipe add`, `recipe init`, `recipe list`, and `sync`.

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

Only `source` and `target` are part of the supported docs contract. Extra keys
may be tolerated by parsing, but doc materialization currently copies declared
files unconditionally and does not apply template-style conditions.

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
| `type` | string | no | Optional hint for the expected type (`string`, `integer`, `boolean` by convention) |
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

Missing `required` causes a validation error. The current validator treats
`type` as descriptive metadata and does not enforce a closed enum of values.

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

## `[init]` workflow declaration

Declares an optional, agent-facing initialization workflow for project-specific setup. Init is **read-only and reviewable by default**: it prints a setup brief, proposed config targets, MCP guidance, and template/override preview, but it does not mutate files or run `sync`.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `prompt` | string | yes | Relative path to an init prompt file inside the recipe directory |
| `description` | string | no | Human-readable setup summary |
| `needs_manifest` | boolean | no | Whether the workflow expects manifest context in the brief |
| `needs_mcp` | array of strings | no | MCP server IDs relevant to setup/discovery |

Example:

```toml
[init]
prompt = "docs/init.md"
description = "Configure tracker board and list mappings"
needs_manifest = true
needs_mcp = ["trello"]
```

Prompt path rules:

- `prompt` must be relative to the recipe directory.
- Absolute paths and parent traversal outside the recipe directory are invalid.
- The prompt target must exist and must be a file.
- Unknown `[init]` fields are rejected so the contract stays small and explicit.

### `ai-specs recipe init <id> [path]`

The command prints an agent-readable initialization brief for a recipe that declares `[init]`.

The brief includes:

- Recipe identity, install state, and init metadata.
- Prompt content from the recipe.
- Project manifest context when relevant.
- Existing `[recipes.<id>.config]` keys and schema-aligned setup targets.
- MCP discovery for configured servers and recipe-provided presets.
- Template/override target preview with create/update/skip guidance.
- Reviewable next actions for the human or agent.

The command is intentionally separate from `ai-specs sync`:

- It does not add recipe declarations to the manifest.
- It does not write `[recipes.<id>.config]` values.
- It does not copy bundled skills, commands, templates, or docs.
- It does not generate `.recipe-mcp.json`, agent configs, or registries.

Durable setup values that a human approves should be written under `[recipes.<id>.config]` unless another existing manifest section owns the value. For example, a Trello board ID belongs in recipe config, while MCP command/env declarations belong under `[mcp.<name>]`.

MCP discovery output must redact secret-like literal values. Env references such as `$TOKEN` are displayed as references rather than resolved. Init guidance preserves the sync-time rule that project manifest MCP values take precedence over recipe defaults.

Init is idempotent: rerunning it detects existing `[recipes.<id>]`, existing config keys, and existing template/override targets, then proposes updates or skips instead of duplicate declarations, duplicate keys, or silent overwrites.

## `[sdd]` recipe metadata

Recipes may declare optional SDD metadata used by the adaptive SDD contract.

### `threshold`

`[sdd].threshold` is an optional ceremony hint that tells agents the minimum
SDD level expected for work involving the recipe.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `threshold` | string | no | Optional ceremony level: `trivial`, `local_fix`, `behavior_change`, or `domain_change` |

Example:

```toml
[sdd]
threshold = "behavior_change"
```

Invalid threshold values are rejected when the recipe is parsed.

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

## Reference recipe: `trello-mcp-workflow`

`catalog/recipes/trello-mcp-workflow/recipe.toml` is the most complete current
example in this repo. It demonstrates:

- `[recipe]` metadata and version pinning
- `[[capabilities]]` declarations
- `[[hooks]]` declarations for sync-time and runtime-deferred actions
- `[config.<field>]` schema entries
- `[provides]` primitives for bundled skills, commands, templates, and docs

Use it as a reference recipe for V2 structure, but treat this document as the
canonical contract when example details and implementation ergonomics diverge.
