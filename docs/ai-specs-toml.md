# ai-specs.toml Reference

`ai-specs/ai-specs.toml` in the project root is the only V1 source of truth.
`ai-specs sync` reads this manifest, vendors configured dependencies, regenerates
`AGENTS.md`, and fans out agent-specific files for the root project and declared
subrepos.

This page documents the contract supported today. It does not describe future
recipe v2 bindings, tracker adapters, or a standalone `[memory]` section.

## Supported Sections

| Section | Required | Purpose |
|---------|----------|---------|
| `[project]` | optional | Project identity and root sync targets. |
| `[agents]` | optional | Selects generated agent outputs. |
| `[[deps]]` | optional repeated table | Vendors external skills into `ai-specs/skills/<id>/`. |
| `[mcp.<name>]` | optional repeated table | Declares MCP servers rendered to enabled agents. |
| `[recipes.<id>]` | optional repeated table | Enables catalog recipes by exact version pin. |
| `[sdd]` | optional | Enables OpenSpec-oriented spec-driven development support. |

Missing `[agents]`, `[[deps]]`, `[mcp.<name>]`, `[recipes.<id>]`, and `[sdd]`
remain valid. The CLI normalizes missing optional sections to stable defaults.

## `[project]`

`[project]` names the project and optionally declares subrepo targets that receive
mirrored generated artifacts.

| Field | Type | Status | Default |
|-------|------|--------|---------|
| `project.name` | string | optional | `""` |
| `project.subrepos` | array of strings | optional | `[]` |

`project.subrepos` entries must be valid root-relative paths. They do not create
independent manifests; the root manifest remains the source of truth.

```toml
[project]
name = "my-project"
subrepos = ["packages/web", "packages/api"]
```

## `[agents]`

`[agents]` controls which agent-specific files are generated.

| Field | Type | Status | Default |
|-------|------|--------|---------|
| `agents.enabled` | array of strings | optional | `[]` |

Supported values: `claude`, `cursor`, `opencode`, `codex`, `copilot`, `gemini`.

```toml
[agents]
enabled = ["claude", "cursor", "opencode", "codex"]
```

Generated outputs include native MCP files where applicable, command folders, and
agent instruction files such as `CLAUDE.md`, `AGENTS.md`, or `opencode.json`.

## `[[deps]]`

Each `[[deps]]` entry vendors an external skill repository into
`ai-specs/skills/<id>/` during `ai-specs sync`.

| Field | Type | Status | Default |
|-------|------|--------|---------|
| `deps.id` | string | required | none |
| `deps.source` | string | required | none |
| `deps.path` | string | optional | repository root |
| `deps.scope` | array of strings | optional | `["root"]` |
| `deps.auto_invoke` | array of strings | optional | `[]` |
| `deps.license` | string | optional | upstream/frontmatter value |
| `deps.vendor_attribution` | string | optional | omitted |

```toml
[[deps]]
id = "tdd"
source = "https://github.com/obra/superpowers"
path = "skills/test-driven-development"
scope = ["root"]
auto_invoke = ["Writing or modifying tests"]
license = "MIT"
vendor_attribution = "obra"
```

Only `id` and `source` are required by the V1 contract.

## `[mcp.<name>]`

Each `[mcp.<name>]` declares one MCP server. `sync-agent` renders the server to
each enabled agent's native config format.

| Field | Type | Status | Default |
|-------|------|--------|---------|
| `mcp.<name>.command` | string or array of strings | optional | omitted |
| `mcp.<name>.args` | array | optional | `[]` |
| `mcp.<name>.env` | array of strings or table | optional canonical field | `{}` |
| `mcp.<name>.environment` | array of strings or table | tolerated alias | normalized to `env` |
| `mcp.<name>.timeout` | integer | optional | omitted |
| `mcp.<name>.enabled` | boolean | tolerated passthrough | omitted |
| `mcp.<name>.type` | string | optional for remote/http servers | omitted |
| `mcp.<name>.url` | string | optional for remote/http servers | omitted |
| `mcp.<name>.headers` | table | optional for remote/http servers | omitted |

`env = ["VAR"]` references variables from the process environment. The CLI turns
that list into a map where the key is the variable name and the value is an
agent-specific environment reference.

`env = { VAR = "literal" }` writes static literal values. Use this when the value
is configuration, not a secret that should come from the user's environment.

`environment` is accepted as a tolerated input alias for `env`; do not use it as the canonical name.

### Local MCP Example

```toml
[mcp.trello]
command = "npx"
args = ["-y", "@modelcontextprotocol/server-trello"]
env = ["TRELLO_API_KEY", "TRELLO_TOKEN"]
timeout = 30000
```

### Static Env Example

```toml
[mcp.demo]
command = "npx"
args = ["-y", "@example/mcp-server"]
env = { MODE = "local" }
```

### Remote MCP Example

```toml
[mcp.remote-docs]
type = "http"
url = "https://example.com/mcp"
headers = { Authorization = "${env:EXAMPLE_TOKEN}" }
timeout = 30000
```

### MCP Render By Agent

| Agent | Output file | Owned key | Env key in output | Environment reference rendering |
|-------|-------------|-----------|-------------------|---------------------------------|
| Claude Code | `.mcp.json` | `mcpServers` | `env` | `$VAR` becomes `${VAR}` |
| Cursor | `.cursor/mcp.json` | `mcpServers` | `env` | `$VAR` becomes `${VAR}` |
| OpenCode | `opencode.json` | `mcp` | `environment` | `$VAR` becomes `{env:VAR}` |
| Codex | `.codex/config.toml` | `[mcp_servers.<name>]` | `env` | `$VAR` becomes `${VAR}` |

OpenCode uses a native schema: local servers render as `type = "local"` with a
single command array, while HTTP/SSE/remote servers render as `type = "remote"`
with `url` and optional headers. OpenCode remote headers use `{env:VAR}` syntax.

Claude Code, Cursor, and Codex use the generic server shape after environment
references are converted to their supported syntax.

## `[recipes.<id>]`

Recipes are named catalog bundles of skills, commands, templates, docs, and MCP
presets. A recipe entry enables a catalog recipe by exact version pin.

| Field | Type | Status | Default |
|-------|------|--------|---------|
| `recipes.<id>.enabled` | boolean | required | none |
| `recipes.<id>.version` | string | required | none |

```toml
[recipes.runtime-memory-openmemory]
enabled = true
version = "1.0.0"
```

The version must match `catalog/recipes/<id>/recipe.toml`. `enabled = false` does
not materialize the recipe.

## `[sdd]`

`[sdd]` enables OpenSpec-oriented spec-driven development support. Omit this
section for projects that do not use SDD.

| Field | Type | Status | Default |
|-------|------|--------|---------|
| `sdd.enabled` | boolean | optional when `[sdd]` is absent | none |
| `sdd.provider` | string | optional when `[sdd]` is absent | none |
| `sdd.artifact_store` | string enum | optional when `[sdd]` is absent | none |

Allowed values: `filesystem`, `hybrid`, `memory`.

```toml
[sdd]
enabled = true
provider = "openspec"
artifact_store = "filesystem"
```

In V1, `provider = "openspec"` is the supported provider. `artifact_store =
"memory"` is experimental; OpenSpec remains file-first.

## Complete Minimal Example

```toml
[project]
name = "my-project"

[agents]
enabled = ["claude", "cursor", "opencode"]
```

## Complete Example With Deps, Recipes, MCP, And SDD

```toml
[project]
name = "my-project"
subrepos = ["packages/web"]

[agents]
enabled = ["claude", "cursor", "opencode", "codex"]

[[deps]]
id = "tdd"
source = "https://github.com/obra/superpowers"
path = "skills/test-driven-development"
scope = ["root"]

[recipes.runtime-memory-openmemory]
enabled = true
version = "1.0.0"

[mcp.trello]
command = "npx"
args = ["-y", "@modelcontextprotocol/server-trello"]
env = ["TRELLO_API_KEY", "TRELLO_TOKEN"]
timeout = 30000

[mcp.demo]
command = "npx"
args = ["-y", "@example/mcp-server"]
env = { MODE = "local" }

[sdd]
enabled = true
provider = "openspec"
artifact_store = "filesystem"
```

## Explicitly Deferred

The V1 manifest does not include:

- A standalone `[memory]` section.
- Recipe v2 capability bindings or hooks.
- Tracker adapters such as Trello/Jira/GitHub Issues as manifest-native sections.
- Precedence or merge policy beyond the currently implemented runtime behavior.
