# recipe.toml Schema

This document is the canonical reference for `recipe.toml` and recipe-level
concepts. For manifest-level bindings and config overrides, see
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
├── docs/           # optional — documentation files
└── hooks/          # optional — runtime hook scripts ([[provides.hooks]])
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

### `[provides.brief]`

A recipe may contribute prose fragments to the agent runtime brief via the optional
`[provides.brief]` table. When enabled, the harness collects fragments from all enabled
recipes (in `enabled` declaration order), deduplicates them, applies `{config.KEY}`
placeholder substitution, and merges the result with the project's own `[brief]` entries
before generating `AGENTS.md`.

When `[provides.brief]` is absent, the recipe contributes no fragments and produces
no change in the renderer's output.

See [`docs/ai-specs-toml.md`](ai-specs-toml.md) for the manifest-side `[brief]` table,
`<section>_mode` append/replace control, and `mcp_descriptions` override-fills-gap.

#### Contributable sections

Only the following sections may appear under `[provides.brief]`:

| Section | Brief heading rendered |
|---------|------------------------|
| `runtime_flow` | `## Runtime Flow` |
| `context_sources` | `## Context Sources` |
| `conflict_policy` | `## Conflict Policy` |
| `workflow_rules` | `## Workflow Rules` |
| `useful_commands` | `## Useful Commands` |
| `mcp_descriptions` | `## Runtime MCPs` (per-server description) |

> **Project-only sections** — `intro` and `purpose` are exclusively for the project
> manifest `[brief]` table. Declaring them in `[provides.brief]` is a validation error:
>
> ```
> [provides.brief].intro: section is project-only; recipes MUST NOT contribute it
> ```

#### Two supported fragment forms

**Form 1 — simple string array** (use when semantic deduplication by key is not needed):

```toml
[provides.brief]
workflow_rules = [
  "Create a dedicated worktree for changes that write artifacts or modify code.",
  "Do not push to `{config.integration_branch}` without a PR.",
]
```

Each string becomes a bullet with `key = null`. Deduplication uses exact text matching.

**Form 2 — array of inline-tables with explicit `key`** (enables stable semantic deduplication
across recipes when multiple recipes might contribute the same concept):

```toml
[[provides.brief.context_sources]]
key  = "trello-source-of-truth"
text = "Trello is the source of truth for work state and dependencies."

[[provides.brief.context_sources]]
key  = "vault-canonical"
text = "Vault is the canonical note-taker for decisions and handoffs."
```

Both `key` and `text` are required for the inline-table form. If either is missing, parsing
raises a validation error naming the missing field.

A single section MUST use one form consistently — mixing string values and inline-table
entries in the same section is a validation error.

#### `{config.KEY}` substitution

Fragment text strings support `{config.KEY}` placeholder substitution. The namespace is
the recipe's own merged config (default values from `[config]` plus any `[recipes.<id>.config]`
overrides from the project manifest).

```toml
[provides.brief]
workflow_rules = [
  "Do not push to `{config.integration_branch}` without a PR.",
  "Run `{config.test_command}` before opening a PR.",
]
```

With `integration_branch = "development"` and `test_command = "./tests/run.sh"` in the
resolved recipe config, the rendered bullets become:

```
- Do not push to `development` without a PR.
- Run `./tests/run.sh` before opening a PR.
```

**Substitution rules:**

- Only `{config.KEY}` placeholders are substituted (the `config.` prefix is required).
- Bare `{KEY}` references (without the `config.` prefix) are left verbatim.
- If a key is referenced but absent from the recipe's merged config, the placeholder is
  preserved verbatim in the output — the render never fails due to a missing key.
- `{{` renders as a literal `{`; `}}` renders as a literal `}`. Use these escapes when
  you need literal curly braces in prose or code examples.

**Escape example:**

```toml
[provides.brief]
workflow_rules = [
  "Run `{config.test_command}` (do not use {{skip}} to bypass hooks).",
]
```

With `test_command = "./tests/run.sh"`, the rendered output is:

```
- Run `./tests/run.sh` (do not use {skip} to bypass hooks).
```

> **Authoring guidance** — always write generic, placeholder-based prose in recipe
> `[provides.brief]` declarations. Use `{config.KEY}` for any project-specific value
> (branches, commands, board IDs). Hard-coding project-specific literals defeats the
> purpose of reusable recipes and makes deduplication unpredictable.

#### `key` field and deduplication

When two enabled recipes declare a fragment with the same `key`, the **first** recipe in
the `enabled` list wins and the later one is silently discarded. This makes recipe ordering
meaningful for semantic deduplication.

When `key` is `null` (simple string array form), deduplication falls back to exact-string
matching: if a fragment with the same resolved text already appears in the output (from a
previous recipe or from the manifest itself), the duplicate is discarded.

#### `mcp_descriptions` section

The `mcp_descriptions` section follows the inline-table form exclusively. Use `key` to
name the MCP server and `text` to provide the description:

```toml
[[provides.brief.mcp_descriptions]]
key  = "trello"
text = "Project tracking through the Trello board."

[[provides.brief.mcp_descriptions]]
key  = "engram"
text = "Operational/session memory (global MCP)."
```

The project manifest `[brief].mcp_descriptions` overrides on a per-server basis. See
[`docs/ai-specs-toml.md`](ai-specs-toml.md) for the precedence rule.

### `hooks` (agent-runtime lifecycle hooks)

Array of tables declaring **agent-runtime** lifecycle hooks (distinct from the
sync-time `[[hooks]]` table below). Each hook is a single portable script that
`ai-specs sync` distributes to every enabled harness in its native format.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Hook identifier; unique within the recipe |
| `event` | string | yes | Abstract event: `pre-tool-use`, `post-tool-use`, `session-start`, `stop` |
| `script` | string | yes | Path inside the recipe directory (no `../`, no absolute paths) |
| `matcher` | string | no | Tool-name pattern (e.g. `Edit\|Write`) |
| `blocking` | boolean | no | Whether the hook can block the action (default `false`) |
| `description` | string | no | Human-readable description |

```toml
[[provides.hooks]]
id          = "worktree-gate"
event       = "pre-tool-use"
script      = "hooks/worktree-gate.sh"
matcher     = "Edit|Write|MultiEdit|NotebookEdit"
blocking    = true
description = "Block writes to the main worktree on a protected branch"
```

Runtime hooks are declared **only** in `recipe.toml`, never in the project
manifest. Tunable values ride the existing `[config.*]` → `[recipes.<id>.config]`
override path and reach the rendered hook as environment variables. See
[`docs/runtime-hooks.md`](runtime-hooks.md) for the normalized script contract,
the abstract→native event map, and per-harness distribution details.

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

# Recipe V2 Additions

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

Defines configuration fields that the recipe expects. Values can be overridden per-project in the manifest under `[recipes.<id>.config]` (see [`docs/ai-specs-toml.md`](ai-specs-toml.md)).

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
