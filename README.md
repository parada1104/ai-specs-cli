# ai-specs

Per-project harness engineering for existing coding-agent tools.

`ai-specs` is a per-project standard for managing coding-agent configuration:
skills, MCP servers, derived instructions, and workflow artifacts for tools
such as Claude, Cursor, OpenCode, Codex, Copilot, and Gemini. Each project owns
its manifest at `ai-specs/ai-specs.toml`; the global `ai-specs` CLI distributes
that manifest into every enabled tool's native format.

The goal is to give a repo its own operational harness: project-local agent
rules, vendored skills, MCP presets, recipes, and SDD workflow wiring that can
be committed, reviewed, and reproduced by a team.

`ai-specs` is **not** a general framework for building arbitrary agents or
custom runtimes from scratch. It assumes an existing coding tool/runtime and
focuses on the per-project harness around it.

In short: `ai-specs` is closer to a per-project operating layer for supported
coding tools than to an agent-building SDK.

## What's included (MVP v1)

| Feature | Status | Description |
|---------|--------|-------------|
| **Per-project manifest** | ✅ | `ai-specs/ai-specs.toml` as single source of truth |
| **Multi-agent fan-out** | ✅ | Claude, Cursor, OpenCode, Codex, Copilot, Gemini |
| **MCP server distribution** | ✅ | Merge-safe MCP config per agent |
| **Skill management** | ✅ | Local, bundled, and vendored skills with autodiscovery |
| **AGENTS.md runtime brief** | ✅ | Concise operational context generated from manifest |
| **Skill metadata validation** | ✅ | `skill-sync` validates `metadata.scope` and `metadata.auto_invoke` |
| **Project initialization** | ✅ | `ai-specs init` scaffolds structure idempotently |
| **Dependency vendoring** | ✅ | `ai-specs add-dep` + `ai-specs sync` clones external skills |
| **Subrepo sync** | ✅ | Mirror derived artifacts to `project.subrepos` |
| **Read-only diagnostics** | ✅ | `ai-specs doctor` validates project health |
| **SDD integration** | ✅ | Optional OpenSpec onboarding via `ai-specs sdd` |
| **Bundled skills** | ✅ | `skill-creator` + `skill-sync` + `skills-as-rules` command |
| **Recipes** | ✅ | Named, versioned bundles of skills, commands, templates, and MCP presets |
| **Lock-based updates** | ✅ | SHA-256 baseline tracking for safe skill updates |

## What's NOT included yet

These features are **explicitly deferred** to post-MVP. They are **not bugs** —
they are roadmap items not yet implemented:

| Feature | Planned | Note |
|---------|---------|------|
| **Memory / persistence layer** | EPIC 2 | No `[memory]` manifest section yet |
| **Context Router** | EPIC 8 | No `ai-specs context plan` command |
| **Handoff automation** | EPIC 3 | No bundled `/handoff` command |
| **Multi-device sync** | EPIC 6 | No sync beyond git |
| **Tracker adapters** | EPIC 7 | No Trello/Jira/GitHub Issues integration |
| **Semantic search** | Post-MVP | No embeddings or local vector search |
| **Coverage / linter / type-check** | Post-MVP | Testing foundation exists; stronger tooling not configured |
| **Build arbitrary agent runtimes** | Out of scope | No LangChain-style harness generation |

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/parada1104/ai-specs-cli/main/install.sh | bash
```

Or from a clone:

```bash
git clone https://github.com/parada1104/ai-specs-cli ~/.ai-specs
bash ~/.ai-specs/install.sh
```

This clones the repo to `~/.ai-specs` and symlinks `bin/ai-specs` into
`~/.local/bin`. Override with `AI_SPECS_HOME` and `INSTALL_BIN`.

Requirements: `bash`, `git`, `python3` (3.11+ for `tomllib`).

## Quick start

```bash
cd my-project
ai-specs init               # scaffolds ai-specs/ + AGENTS.md + .gitignore (idempotent)
# edit ai-specs/ai-specs.toml — set [agents].enabled, add [[deps]], add [mcp.*]
ai-specs sync               # vendor deps + regen AGENTS.md + fan out root + subrepos
```

That's it — agent configs are now generated from your manifest. Re-run
`ai-specs sync` whenever the manifest changes. If `project.subrepos` is
declared, the root sync mirrors derived artifacts into each subrepo so agents
work from either location.

## CLI

| Command | Description |
|---------|-------------|
| `ai-specs init [path] [--name N] [--force]` | Bootstrap `ai-specs/` (idempotent; never overwrites your `ai-specs.toml`) |
| `ai-specs sync [path]` | Vendor deps, regenerate AGENTS.md, validate skill metadata, fan out per target + per agent |
| `ai-specs sync-agent [path] [--all\|--<agent>]` | Fan out per-agent configs for the current target only |
| `ai-specs doctor [path]` | Read-only health check: manifest, bundled assets, agents, symlinks, MCP outputs |
| `ai-specs refresh-bundled [path]` | Update bundled skills/commands from the CLI; keeps your edits |
| `ai-specs add-dep <git-url> [path]` | Register a vendored skill in `[[deps]]` and sync |
| `ai-specs recipe list [path]` | List available recipes from the catalog |
| `ai-specs recipe add <id> [path]` | Add a recipe declaration to the manifest and sync |
| `ai-specs recipe init <id> [path]` | Print an initialization brief for a recipe that declares `[init]` |
| `ai-specs sdd enable [path]` | Enable SDD: scaffold `openspec/`, merge config, refresh bundled skills |
| `ai-specs sdd disable [path]` | Set `[sdd].enabled = false` (preserves `openspec/`) |
| `ai-specs sdd status [path]` | Show `[sdd]` manifest block, toolchain presence, directory health |
| `ai-specs upgrade [--dry-run] [--force]` | Safely upgrade the global installation to latest `origin/main` |
| `ai-specs version` | Print CLI version |
| `ai-specs help` | Show help |

Every subcommand accepts an optional `[path]` (defaults to `cwd`) and `--help`.

## Spec-driven development (`ai-specs sdd`)

Projects can declare optional `[sdd]` in `ai-specs/ai-specs.toml` and use
`ai-specs sdd` to scaffold and verify the OpenSpec provider. Requirements when
mutating: **Node.js ≥ 20.19** and `openspec` on `PATH`, unless you pass
`--install-provider-cli`.

```bash
ai-specs sdd enable [path]
ai-specs sdd enable --install-provider-cli
ai-specs sdd status [path]
```

Minimal manifest block:

```toml
[sdd]
enabled = true
provider = "openspec"
artifact_store = "filesystem"
```

See [`docs/ai/sdd.md`](docs/ai/sdd.md) for the full provider contract,
generated command reference, and config example.

## Manifest and recipe references

The canonical manifest contract and recipe schema live in dedicated documents:

- [`docs/ai-specs-toml.md`](docs/ai-specs-toml.md) — Canonical manifest reference
- [`docs/recipe-schema.md`](docs/recipe-schema.md) — Canonical recipe and recipe V2 reference
- [`docs/ai/sdd.md`](docs/ai/sdd.md) — SDD provider contract and OpenSpec workflow
- [`docs/mcp-distribution.md`](docs/mcp-distribution.md) — How MCP config is rendered per agent
- [`docs/skills-by-agent.md`](docs/skills-by-agent.md) — How skills are surfaced to each agent
- [`docs/bundled-merge-rules.md`](docs/bundled-merge-rules.md) — Lock-based skill update behavior

## Root + subrepo sync

`project.subrepos` is an active sync input. When declared, `ai-specs sync`
resolves targets as `[root, ...project.subrepos]`, validates each path before
writing, vendors external skills once in the root workspace, and mirrors derived
artifacts into every target. Versioning policy: subrepo files are derived
outputs from the latest root sync run — do not hand-edit them, and do not add a
subrepo `ai-specs.toml` in V1.

## What gets created in your project

```
my-project/
├── AGENTS.md                       ← runtime brief (generated — do not hand-edit)
├── .gitignore                      ← appended with an ai-specs block
├── .recipe/                        ← recipe-bundled skills (gitignored)
│   └── <recipe-id>/
├── .deps/                          ← vendored dependency skills (gitignored)
│   └── <dep-id>/
└── ai-specs/
    ├── ai-specs.toml               ← YOUR manifest (edit this)
    ├── .gitignore                  ← derived
    ├── .internal/resolved-skills/  ← flattened skill tree (gitignored; symlinks point here)
    ├── skills/                     ← local + bundled skills (committed)
    └── commands/                   ← local slash commands
```

### Three-tier skill layout

| Category   | Lives in                                | Listed in toml? | Committed? | Created by |
|------------|-----------------------------------------|-----------------|------------|------------|
| Local      | `ai-specs/skills/<name>/`               | No (autodiscovered) | Yes | `/skills-as-rules` |
| Recipe     | `.recipe/<recipe-id>/skills/<name>/`    | Yes (`[recipes.*]`) | No (gitignored) | `ai-specs sync` |
| Dependency | `.deps/<dep-id>/skills/<name>/`         | Yes (`[[deps]]`)    | No (gitignored) | `ai-specs add-dep` |

When the same skill ID exists in multiple sources, the higher-precedence
source wins automatically. Local skills silently override recipe and dep
versions without error or warning.

## Adding skills

### Local skill

```bash
/skills-as-rules
```

The slash command runs inside your agent, asks one convention at a time, uses
`skill-creator` to author the skill, and runs `skill-sync`. Result: a committed
local skill at `ai-specs/skills/<name>/SKILL.md`.

### Vendored skill

```bash
ai-specs add-dep https://github.com/foo/superskill \
    --trigger "When doing X" \
    --license MIT
```

Appends a `[[deps]]` block to `ai-specs.toml` and syncs.

## Updating

```bash
ai-specs upgrade            # fast-forward global install to latest
ai-specs upgrade --dry-run  # preview without modifying
ai-specs upgrade --force    # skip working-tree safety checks
```

After upgrading, refresh any project:

```bash
cd <your-project>
ai-specs sync
```

Re-running the installer is safe for recovery:

```bash
curl -fsSL https://raw.githubusercontent.com/parada1104/ai-specs-cli/main/install.sh | bash
```

Use `ai-specs upgrade` for routine updates and `install.sh` only for first-time
install or recovery from a broken installation.

## Layout (this repo)

```
ai-specs-cli/
├── bin/ai-specs                ← global entrypoint (dispatcher)
├── lib/
│   ├── init.sh                 ← bootstrap a project
│   ├── sync.sh                 ← resolve targets + refresh + fan-out
│   ├── sync-agent.sh           ← render one target
│   ├── add-dep.sh              ← register vendored skill
│   ├── version.sh
│   └── _internal/
│       ├── toml-read.py        ← read ai-specs.toml sections
│       ├── vendor-skills.py    ← clone [[deps]] → skills/<id>/
│       ├── gitignore-render.py ← [[deps]] → .gitignore
│       ├── agents-md-render.py ← skills/ → AGENTS.md
│       ├── mcp-render.py       ← [mcp.*] → per-agent format
│       └── platform.sh         ← per-agent paths/keys
├── bundled-skills/             ← copied on init (contracts only)
├── bundled-commands/           ← copied on init
├── catalog/
│   ├── skills/                 ← optional vendorable skills
│   └── recipes/                ← recipe definitions
├── templates/
│   ├── ai-specs.toml.tmpl
│   └── gitignore-root.tmpl
├── install.sh
├── VERSION
└── LICENSE
```

## License

MIT
