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
- `[tool]`
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
| `[recipes.<id>]` | `version` | optional legacy; ignored with WARN — sync uses CLI catalog |
| `[recipes.<id>.config]` | `<key> = <value>` | optional per-recipe overrides; unknown keys warn and are ignored |
| `[[bindings]]` | `capability`, `recipe` | optional explicit capability binding |
| `[brief]` | `render` | optional; boolean; default `true` — when `false`, `ai-specs sync`/`init` do not write `AGENTS.md` (manual brief; recipe fragments not merged) |
| `[brief]` | `intro` | optional; multi-line string; rendered as a `>` blockquote after H1 |
| `[brief]` | `purpose` | optional; string; one-line project description in `## Project` |
| `[brief]` | `runtime_flow` | optional; array of strings; bullets **appended after** recipe-contributed fragments in `## Runtime Flow` |
| `[brief]` | `context_sources` | optional; array of strings; bullets **appended after** recipe-contributed fragments in `## Context Sources` |
| `[brief]` | `conflict_policy` | optional; array of strings; bullets **appended after** recipe-contributed fragments in `## Conflict Policy` |
| `[brief]` | `workflow_rules` | optional; array of strings; bullets **appended after** recipe-contributed fragments in `## Workflow Rules` |
| `[brief]` | `useful_commands` | optional; array of strings; extra bullets **appended after** recipe-contributed fragments in `## Useful Commands` |
| `[brief]` | `runtime_flow_mode` | optional; `"append"` (default) or `"replace"` — suppress all recipe fragments for `runtime_flow` when `"replace"` |
| `[brief]` | `context_sources_mode` | optional; `"append"` (default) or `"replace"` — suppress all recipe fragments for `context_sources` when `"replace"` |
| `[brief]` | `conflict_policy_mode` | optional; `"append"` (default) or `"replace"` — suppress all recipe fragments for `conflict_policy` when `"replace"` |
| `[brief]` | `workflow_rules_mode` | optional; `"append"` (default) or `"replace"` — suppress all recipe fragments for `workflow_rules` when `"replace"` |
| `[brief]` | `useful_commands_mode` | optional; `"append"` (default) or `"replace"` — suppress all recipe fragments for `useful_commands` when `"replace"` |
| `[brief.mcp_descriptions]` | `<server-name>` | optional; overrides the recipe-provided default description per server; entries not covered by the project fall back to the recipe-declared value |
| `[tool]` | `version` | optional; exact CLI version pin (semver) |
| `[tool]` | `min_version` | optional; minimum acceptable CLI version (semver); mutually exclusive with `version` |
| `[tool]` | `policy` | optional; `exact` or `min` (inferred from which version field is set) |

## Manifest sections

### `[project]`

Project metadata owned by the repo.

```toml
[project]
name = "my-project"
subrepos = ["packages/app", "packages/docs"]
```

### `[tool]`

Optional CLI version policy for the project. When set, `ai-specs sync` validates
the running CLI before writing files. Lock metadata (`ai-specs/.ai-specs.lock`
`[meta]`) records the CLI version on each successful sync.

```toml
[tool]
version = "0.12.2"
policy = "exact"
```

Minimum version instead of exact pin:

```toml
[tool]
min_version = "0.11.0"
policy = "min"
```

Use `ai-specs doctor` to compare installed, pinned, and last-synced CLI versions.
Pass `ai-specs sync --ignore-cli-version` to bypass enforcement in emergencies.

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

Enables a named recipe. Sync materializes the catalog version shipped with the
installed CLI; no per-recipe pin is required. A legacy `version` key is ignored
with a WARN.

```toml
[recipes.trello-mcp-workflow]
enabled = true
```

### `[recipes.<id>.config]`

Overrides defaults from the recipe's `[config]` schema. See
[`docs/recipe-schema.md`](recipe-schema.md) for the recipe-level `[config]` contract.

```toml
[recipes.trello-mcp-workflow.config]
board_id = "abc123"
default_list = "In Progress"
```

`worktree-flow` also supports gated write modes:

```toml
[recipes.worktree-flow.config]
gate_mode = "ask"
```

For config fields that define an `enum`, sync validates that the manifest value
is one of the allowed entries before materializing the recipe.

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
corresponding key is absent **and** no enabled recipe contributes fragments for
that section.

**`intro` and `purpose` are project-only.** They are always written here and are
never contributed by recipes.

**Contributable sections** (`runtime_flow`, `context_sources`, `conflict_policy`,
`workflow_rules`, `useful_commands`) can be populated entirely by recipe fragments.
A manifest with only `intro` and `purpose` in `[brief]` is fully valid when enabled
recipes supply the behavioral prose. The project `[brief]` entries for these sections
are **appended after** recipe fragments (exact-string duplicates are silently discarded).

Minimal `[brief]` for a project that relies on recipe fragments for behavioral sections:

```toml
[brief]
intro = """
Canonical runtime context for agents: project identity, MCPs,
context sources, safety rules, and workflow conventions.
"""
purpose = "per-project AI harness for configuration, MCPs, recipes, memory, and tracker integration."
# Contributable sections (runtime_flow, context_sources, etc.) are supplied
# by enabled recipes. Add entries here only for project-specific additions.
# Use <section>_mode = "replace" to suppress recipe fragments for a section.
```

Full example with project-specific additions:

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

#### `<section>_mode` — append vs. replace

By default (`"append"`), recipe fragments for a section appear first, and project
`[brief]` entries for that same section are appended after (with exact-string dedup).

Set `<section>_mode = "replace"` to suppress all recipe-contributed fragments for
that section and use only the project's own entries:

```toml
[brief]
# Suppress recipe fragments for workflow_rules; only local entries appear.
workflow_rules_mode = "replace"
workflow_rules = [
  "Follow project-specific deployment checklist before merging.",
  "All PRs must be reviewed by two team members.",
]
```

Replace mode for one section does not affect other sections — each section's mode
is independent. Any `_mode` value other than `"append"` or `"replace"` is a
validation error at render time.

### `[brief.mcp_descriptions]`

Keyed by MCP server name. Each value is the description shown for that server's
entry in the `## Runtime MCPs` section of the generated brief.

**Override-fills-gap rule:** A project entry for a server **overrides** any
recipe-provided default for that server. When the project has no entry for a server
but an enabled recipe declares one via `[provides.brief].mcp_descriptions`, the
recipe value fills the gap automatically. No project entry is required.

```toml
[brief.mcp_descriptions]
trello = "project tracking through the Roadmap board."
engram = "operational/session memory (global MCP)."
```

Project entries listed here take precedence over any value contributed by a recipe
for the same server name. Servers not covered by the project fall back to the
recipe-declared description, if any.

For example, if a `trello-mcp-workflow` recipe declares a default description for
the `trello` server, and the project manifest also declares `[brief.mcp_descriptions].trello`,
the project value wins. If the project has no `trello` entry, the recipe default is used.

#### Migration note for existing projects with a full hand-written `[brief]`

If your project was set up before this feature and `[brief]` contains a complete set
of behavioral sections (runtime_flow, context_sources, etc.), you may see **duplicate
bullets** once recipes start contributing `[provides.brief]` fragments — recipe
fragments appear first, and identical manifest entries are deduplicated by exact-string
match, but near-identical entries may both appear.

**Cleanup options:**

1. **Remove duplicate entries from `[brief]`** — if the recipe fragment covers the same
   content as your existing bullet, delete the manifest entry.

2. **Use `<section>_mode = "replace"`** — add `workflow_rules_mode = "replace"` (or the
   relevant section key) to keep your existing manifest entries and suppress recipe
   contributions for that section entirely.

3. **Use `[brief] render = false`** — manifest-level opt-out; sync/init/subrepos skip
   managed `AGENTS.md` generation entirely. Preferred when the project curates the brief
   in version control and does not want recipe fragments merged on each sync.

4. **Use the `<!-- ai-specs:runtime-brief -->` marker** — file-level opt-out when
   `render` is not `false`. If the marker is present, `ai-specs sync` will not regenerate
   that file. Precedence when `render = true`: marker suppresses overwrite; otherwise
   normal render runs. When `render = false`, the marker is redundant (renderer is not invoked).

Subrepos inherit the root manifest's `[brief].render` policy — there is no per-subrepo
override in V1.

## Out of scope

Out of scope for this V1 contract (explicitly deferred to future changes):

- precedence / merge policy beyond the currently implemented runtime behavior

## See also

- [`templates/ai-specs.toml.tmpl`](../templates/ai-specs.toml.tmpl)
- [`docs/recipe-schema.md`](recipe-schema.md)

## Environment variables (`.envrc.example`)

Enabled recipes may declare MCP env references under `[[provides.mcp]]`
(e.g. `TRELLO_API_KEY = "$TRELLO_API_KEY"`). The config wizard and init flow can
generate `ai-specs/.envrc.example` from those references:

```bash
ai-specs configure-recipes
# or accept the offer after `ai-specs init` / hub "Configure recipes"
```

- `.envrc.example` is a committed template (safe to regenerate; existing files are
  backed up to `.envrc.example.bak`).
- `.envrc` is user-owned and gitignored — the tool never writes it. Copy the
  example and fill in real values locally (direnv, etc.).

