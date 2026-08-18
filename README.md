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
ai-specs sync --adopt-brief           # explicitly hand an existing AGENTS.md to ai-specs
```

Your agent configs are now generated from the manifest. Re-run `ai-specs sync`
whenever the manifest changes.


## Interactive hub (`ai-specs` with no subcommand)

Running bare `ai-specs` (or `ai-specs hub [path]`) opens the project hub instead of printing help.
`ai-specs help` remains available and unchanged.

Behavior is a 4-state matrix of **initialized × TTY**:

| State | Condition | Behavior | Exit |
|-------|-----------|----------|------|
| Interactive hub | manifest present + TTY | Status panel + command menu | 0 on Quit |
| Non-interactive status | manifest present + no TTY | Plain-text status + command list (no deps) | 0 |
| Offer init | no manifest + TTY | Confirm → run `ai-specs init` → hub | 0 if declined |
| Uninitialized error | no manifest + no TTY | Stderr guidance to run init | 2 |

Menu actions: Sync, Doctor, Agents, Skills, Recipes, Rules audit, Upgrade, Version, Help, Init wizard, Quit.

Hub UX notes:
- **Status** (interactive panel + non-interactive text) includes a `version` row from the CLI `VERSION` file (`unknown` when missing).
- **Recipes** Add/Remove use pickers over catalog/manifest recipes (no free-text id prompts). Configure stays nested under Recipes.
- **Skills** opens an interactive categorized submenu (Bundled / Local·vendored / Recipes·catalog / Registered deps) with list + inspect.
- **`ai-specs skills list`** labels CLI-shipped skills under **Bundled skills**, not Local.

Missing interactive deps (`rich` + `questionary`) yield exit **3** with install guidance. Non-interactive status needs **no** third-party packages (CI-safe).

## CLI

| Command | Description |
|---------|-------------|
| `ai-specs` / `ai-specs hub [path]` | Interactive status + command menu (see below) |
| `ai-specs configure-recipes [path]` | Configure recipe config fields, check CLI deps, offer `.envrc.example` |
| `ai-specs init [path]` | Bootstrap `ai-specs/` (idempotent) |
| `ai-specs sync [path]` | Vendor deps, regen AGENTS.md, fan out (compact output by default) |
| `ai-specs sync [path] [--ignore-cli-version]` | Sync with optional CLI pin bypass |
| `ai-specs sync [path] [-v\|--verbose]` | Sync with full per-step detail instead of the compact summary |
| `ai-specs sync-agent [path] [--all\|--<agent>] [-v\|--verbose]` | Fan out per-agent configs only; `-v` forwards through fan-out |
| `ai-specs doctor [path]` | Read-only health check |
| `ai-specs rules-audit [path]` | Read-only legacy rules inventory (JSON) |
| `ai-specs refresh-bundled [path]` | Update bundled skills/commands from the CLI |
| `ai-specs skills add <git-url> [path]` | Register a vendored skill (`[[deps]]`) and sync |
| `ai-specs skills list [path]` | List bundled, registered, local, and catalog skills |
| `ai-specs skills remove <id> [path]` | Remove a vendored skill from the manifest |
| `ai-specs add-dep <git-url> [path]` | Alias for `skills add` (backward-compatible) |
| `ai-specs recipe list [path]` | List available recipes |
| `ai-specs recipe add <id> [path]` | Add a recipe declaration |
| `ai-specs recipe init <id> [path]` | View recipe initialization brief |
| `ai-specs upgrade [--dry-run] [--force] [-v]` | Upgrade global installation |
| `ai-specs version` | Print version |

Every subcommand accepts an optional `[path]` (defaults to `cwd`) and `--help`.

`sync` and `sync-agent` print one compact `syncing <label>` line per step by
default; warnings, notices, and errors always pass through untouched. Pass
`-v`/`--verbose` to restore full per-step detail. A failing step always prints
its complete, unfiltered stdout+stderr regardless of mode, so compaction never
hides a diagnosis. Note: compact/verbose capture stdout and stderr separately
per step and replay them sequentially, so exact interleaving between the two
streams is not preserved in the summary view — acceptable for a status
overview, not a replacement for verbose output when debugging ordering.

## Key concepts

### Manifest (`ai-specs/ai-specs.toml`)

Single source of truth for the project's AI harness. Declares enabled agents,
MCP servers, skill dependencies, recipes, and optionally a CLI version pin
(`[tool]`). See [`docs/ai-specs-toml.md`](docs/ai-specs-toml.md) for the full
reference. See [`CHANGELOG.md`](CHANGELOG.md) for migration notes between CLI
versions.

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
interactively. For batch migration from legacy Cursor rules, run `/rules-audit`.

### Rules migration audit

`ai-specs rules-audit [path]` scans legacy rule sources read-only and emits JSON
inventory to stdout. The `/rules-audit` slash command consumes that JSON and
writes an advisory plan to `ai-specs/plans/rules-migration-<YYYY-MM-DD>.md`.

Classification buckets (suggestions only):

`keep_in_brief` · `enable_recipe` · `use_catalog_dep` · `create_local_skill` ·
`merge_into_skill` · `already_in_atl` · `deprecate_rule_file`

### Recipes

Named bundles of skills, commands, templates, and MCP presets from the installed
CLI catalog. Declared in `[recipes.<id>]` with `enabled = true` (no version pin)
and materialized by `ai-specs sync`. See the
[recipe catalog](docs/recipes-catalog.md) for what each shipped recipe does and
the config it expects, and [`docs/recipe-schema.md`](docs/recipe-schema.md) for
the `recipe.toml` schema.

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
ai-specs upgrade -v                  # show the full git output
```

The upgrade prints one line per step, then summarizes the versions you crossed.
When a release requires a post-upgrade action, it prints that under
**Action required** — read it, because nothing else will tell you.

After upgrading, run `ai-specs sync` in each project to refresh generated
artifacts.

### Install footprint

`~/.ai-specs` is a partial clone with a sparse checkout: `openspec/`, `tests/`,
`.github/` and `tmp/` are not materialized, since nothing reads them at runtime.
The full commit history is kept — `ai-specs upgrade` needs it to verify your
install has not diverged, which a shallow clone would break.

To restore every file (for example to run the test suite from the install):

```bash
git -C ~/.ai-specs sparse-checkout disable
```

Narrowing is best effort. On a git without sparse-checkout support, or if
anything fails, you keep an ordinary full checkout and the upgrade still
succeeds.

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
