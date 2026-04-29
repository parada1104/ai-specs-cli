# ai-specs.toml Reference

`ai-specs/ai-specs.toml` in the project root is the only V1 source of truth.
This reference documents the mature manifest surface supported today: project
metadata, vendored skill dependencies, and MCP server distribution.

Agent selection is included only as context because MCP output depends on
`[agents].enabled`. Broader workflow features are not part of this manifest
reference yet.

## Supported Sections

| Section | Required | Purpose |
|---------|----------|---------|
| `[project]` | optional | Project identity and root sync targets. |
| `[agents]` | optional context | Selects which agent outputs receive generated config. |
| `[[deps]]` | optional repeated table | Vendors external skills into `ai-specs/skills/<id>/`. |
| `[mcp.<name>]` | optional repeated table | Declares MCP servers rendered to enabled agents. |

Missing `[agents]`, `[[deps]]`, and `[mcp.<name>]` remain valid. The CLI
normalizes missing optional sections to stable defaults.

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

## `[agents]` Context

`[agents]` controls which agent-specific files are generated. It is documented
here because MCP rendering depends on enabled agents.

| Field | Type | Status | Default |
|-------|------|--------|---------|
| `agents.enabled` | array of strings | optional | `[]` |

Supported values: `claude`, `cursor`, `opencode`, `codex`, `copilot`, `gemini`.

```toml
[agents]
enabled = ["claude", "cursor", "opencode", "codex"]
```

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

## Complete Example

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

[mcp.trello]
command = "npx"
args = ["-y", "@modelcontextprotocol/server-trello"]
env = ["TRELLO_API_KEY", "TRELLO_TOKEN"]
timeout = 30000

[mcp.demo]
command = "npx"
args = ["-y", "@example/mcp-server"]
env = { MODE = "local" }
```

## Not In This Reference

This page intentionally does not document experimental manifest surface. In
particular, it does not recommend configuring:

- A standalone `[memory]` section.
- Bundle declarations or capability bindings.
- SDD/OpenSpec settings as part of the stable TOML reference.
- Tracker adapters such as Trello/Jira/GitHub Issues as manifest-native sections.
