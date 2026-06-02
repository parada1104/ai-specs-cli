# ai-specs

Per-project harness engineering for existing coding-agent tools.

`ai-specs` manages coding-agent configuration — skills, MCP servers, derived
instructions, recipes, and workflow artifacts — for tools like Claude, Cursor, OpenCode,
Codex, Copilot, and Gemini. Each project owns a manifest at
`ai-specs/ai-specs.toml`; the `ai-specs` CLI distributes it into every enabled
tool's native format.

**Agent orchestration** (multi-phase planning, multi-model sub-agents, profiles) is handled
by [gentle-ai](https://github.com/Gentleman-Programming/gentle-ai). `ai-specs` focuses
on the **spec layer** and **tool integrations** (recipes) — the fan-out across repos and
harnesses.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/parada1104/ai-specs-cli/main/install.sh | bash
```

Requires: `bash`, `git`, `python3` (3.11+ for `tomllib`).

## Quick start

```bash
cd my-project
ai-specs init                        # scaffold ai-specs/ + AGENTS.md + .gitignore
# edit ai-specs/ai-specs.toml — set [agents].enabled, add [[deps]], add [mcp.*]
ai-specs sync                        # vendor deps + regen AGENTS.md + fan out per agent
```

Your agent configs are now generated from the manifest. Re-run `ai-specs sync`
whenever the manifest changes.

## CLI

| Command | Description |
|---------|-------------|
| `ai-specs init [path]` | Bootstrap `ai-specs/` (idempotent) |
| `ai-specs sync [path]` | Vendor deps, regen AGENTS.md, fan out |
| `ai-specs sync-agent [path] [--all\|--<agent>]` | Fan out per-agent configs only |
| `ai-specs doctor [path]` | Read-only health check |
| `ai-specs refresh-bundled [path]` | Update bundled skills/commands from the CLI |
| `ai-specs add-dep <git-url> [path]` | Register a vendored skill |
| `ai-specs recipe list [path]` | List available recipes |
| `ai-specs recipe add <id> [path]` | Add a recipe declaration |
| `ai-specs recipe init <id> [path]` | View recipe initialization brief |
| `ai-specs upgrade [--dry-run] [--force]` | Upgrade global installation |
| `ai-specs version` | Print version |

Every subcommand accepts an optional `[path]` (defaults to `cwd`) and `--help`.

## Key concepts

### Manifest (`ai-specs/ai-specs.toml`)

Single source of truth for the project's AI harness. Declares enabled agents,
MCP servers, skill dependencies, and recipes. See
[`docs/ai-specs-toml.md`](docs/ai-specs-toml.md) for the full reference.

### Agents

The `[agents].enabled` list controls which tools receive config. Supported:
`claude`, `cursor`, `opencode`, `codex`, `copilot`, `gemini`. Each gets MCP
configs, skills, and commands in its native format.

### MCP servers

Declare `[mcp.<name>]` blocks in the manifest. `ai-specs sync` distributes them
per agent, handling format differences (env var interpolation, JSON schemas).
See [`docs/mcp-distribution.md`](docs/mcp-distribution.md).

### Skills

Three tiers with automatic precedence (local > recipe > dependency):

| Source | Location | Committed |
|--------|----------|-----------|
| Local | `ai-specs/skills/<name>/` | Yes |
| Recipe | `.recipe/<recipe-id>/skills/<name>/` | No (gitignored) |
| Dependency | `.deps/<dep-id>/skills/<name>/` | No (gitignored) |

Use the `/skills-as-rules` slash command inside your agent to create local skills
interactively.

### Recipes

Named, versioned bundles of skills, commands, templates, and MCP presets.
Declared in `[recipes.<id>]` and materialized by `ai-specs sync`. See
[`docs/recipe-schema.md`](docs/recipe-schema.md).

### Harness engineering

`ai-specs` treats agent configuration as infrastructure: a single manifest fans out
to every enabled tool. The primitives are skills, MCP servers, recipes, runtime
hooks, and derived instructions — versioned, vendored, and reproducible.

Recipes can also declare **agent-runtime hooks** (`[[provides.hooks]]`): one
portable script that `ai-specs sync` distributes to every enabled harness in its
native format (Claude `PreToolUse`, generated Cursor/OpenCode/Pi adapters). See
[`docs/runtime-hooks.md`](docs/runtime-hooks.md).

### Updating

```bash
ai-specs upgrade                     # fast-forward global install
ai-specs upgrade --dry-run           # preview only
```

After upgrading, run `ai-specs sync` in each project to refresh generated
artifacts.

## Project layout (this repo)

```
bin/ai-specs        ← global entrypoint
lib/                ← sync, init, agent config, renderers
bundled-skills/     ← skills shipped with the CLI
bundled-commands/   ← slash commands shipped with the CLI
catalog/recipes/    ← recipe definitions
templates/          ← scaffolding templates
docs/               ← reference documentation
tests/              ← test suite
```

## Development

Testing foundation exists at `tests/`. Run `./tests/run.sh` to execute the full
Python test suite, or `./tests/validate.sh` for syntax checks followed by the
test suite. The `skill-sync` bundled skill validates and normalises SKILL.md
frontmatter across all project skills.

## License

MIT
