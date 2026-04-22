# specs-ai

One declarative manifest per project — fan out to Claude, Cursor, OpenCode, Codex, Copilot, and Gemini.

`specs-ai` is a per-project standard for managing AI agent configuration: skills,
MCP servers, and per-agent instruction files. Each project owns its manifest at
`ai-specs/ai-specs.toml`; the global `specs-ai` CLI distributes that manifest
into every enabled agent's native format.

Inspired by [`charliesbot/chai`](https://github.com/charliesbot/chai) (global
fan-out, merge-safe MCP) but **per-project** so different repos can have
different agents, skills, and MCP servers — and the configuration is
committable and shareable with a team.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/nnodes/specs-ai-cli/main/install.sh | bash
```

Or from a clone:

```bash
git clone https://github.com/nnodes/specs-ai-cli ~/.specs-ai
bash ~/.specs-ai/install.sh
```

This clones the repo to `~/.specs-ai` and symlinks `bin/specs-ai` into
`~/.local/bin`. Override with `SPECS_AI_HOME` and `INSTALL_BIN`.

Requirements: `bash`, `git`, `python3` (3.11+ for `tomllib`).

## Quick start

```bash
cd my-project
specs-ai init               # scaffolds ai-specs/ + AGENTS.md + .gitignore (idempotent)
# edit ai-specs/ai-specs.toml — set [agents].enabled, add [[deps]], add [mcp.*]
specs-ai sync               # vendor deps + regen AGENTS.md + fan out per agent
```

That's it — `.claude/`, `.cursor/`, `.opencode/`, `CLAUDE.md`, `.mcp.json`, etc.
are now generated from your manifest. Re-run `specs-ai sync` whenever the
manifest changes.

## What gets created in your project

```
my-project/
├── AGENTS.md                       ← root agent instructions (template; edit freely)
├── .gitignore                      ← appended with an ai-specs block (gitignores agent files)
└── ai-specs/
    ├── ai-specs.toml               ← YOUR manifest (edit this)
    ├── .gitignore                  ← derived; lists vendored skill dirs
    └── skills/
        ├── skill-creator/          ← bundled (committable; customize freely)
        ├── skill-sync/             ← bundled (committable; customize freely)
        ├── <your-local-skill>/     ← scaffolded by `specs-ai add-skill` (committed)
        └── <vendored-skill>/       ← cloned from [[deps]] (gitignored)
```

### Three skill categories

| Category   | Lives in            | Listed in toml? | Committed? | Created by             |
|------------|---------------------|-----------------|------------|------------------------|
| Local      | `ai-specs/skills/<name>/`   | No (autodiscovered) | Yes        | `specs-ai add-skill <name>` |
| Bundled    | `ai-specs/skills/{skill-creator,skill-sync}/` | No | Yes (own-and-customize) | `specs-ai init` (one-time copy) |
| Vendored   | `ai-specs/skills/<dep-id>/` | Yes (`[[deps]]`)    | No (gitignored) | `specs-ai add-dep <url>` → cloned by sync |

## CLI

| Command | Description |
|---------|-------------|
| `specs-ai init [path] [--name N] [--force]` | Bootstrap `ai-specs/` (idempotent; `--force` rewrites templates and bundled skills) |
| `specs-ai sync [path]` | Vendor `[[deps]]`, refresh AGENTS.md auto-invoke table, fan out per agent |
| `specs-ai sync-agent [path] [--all|--<agent>]` | Fan out per-agent only (no vendoring/regen) |
| `specs-ai add-skill <name> [path]` | Scaffold a local skill |
| `specs-ai add-dep <git-url> [path]` | Register a vendored skill in `[[deps]]` and `sync` |
| `specs-ai version` | Print CLI version |
| `specs-ai help` | Show help |

Every subcommand accepts an optional `[path]` (defaults to `cwd`) and `--help`.

## How MCP distribution works

`[mcp.*]` entries in `ai-specs.toml` are rendered into each agent's native
config via a **merge-safe** strategy: `specs-ai` owns the MCP key (e.g.
`mcpServers`), and every other top-level key is preserved.

| Agent    | Target file              | Key            | Format | Notes |
|----------|--------------------------|----------------|--------|-------|
| Claude   | `.mcp.json`              | `mcpServers`   | JSON   | per-project |
| Cursor   | `.cursor/mcp.json`       | `mcpServers`   | JSON   | merge preserves other keys |
| OpenCode | `opencode.json`          | `mcp`          | JSON   | translated to OpenCode native schema (`type:"local"`, `command:[…]`, `environment:{…}`, `{env:VAR}`) |
| Codex    | `.codex/config.toml`     | `mcp_servers`  | TOML   | rewrites `[mcp_servers.*]` blocks only |
| Gemini   | `.gemini/settings.json`  | `mcpServers`   | JSON   | |
| Copilot  | (no MCP support)         | —              | —      | reads AGENTS.md only |

## How skills are surfaced to each agent

| Agent    | Reads AGENTS.md natively? | Native skill auto-invoke? | What sync-agent generates |
|----------|---------------------------|---------------------------|---------------------------|
| Claude   | No (needs `CLAUDE.md`)    | Yes (`.claude/skills/<name>/SKILL.md`) | `CLAUDE.md` symlink + `.claude/skills` symlink → `ai-specs/skills` + `.mcp.json` |
| Cursor   | Yes                       | No (skills via AGENTS.md text)         | `.cursor/mcp.json` |
| OpenCode | Yes                       | No                                     | `opencode.json` |
| Codex    | Yes                       | No                                     | `.codex/config.toml` |
| Copilot  | No (`.github/copilot-instructions.md`) | No                          | `.github/copilot-instructions.md` symlink |
| Gemini   | No (needs `GEMINI.md`)    | Yes (`.gemini/skills/<name>/SKILL.md`) | `GEMINI.md` symlink + `.gemini/skills` symlink + `.gemini/settings.json` |

The `Auto-invoke` table in `AGENTS.md` is regenerated automatically by
`skill-sync` whenever you `specs-ai sync` or `specs-ai add-skill`.

## Adding skills

### Local skill (lives in your project)

```bash
specs-ai add-skill mi-skill \
    --description "What it does" \
    --trigger "When the AI should load it"
```

Scaffolds `ai-specs/skills/mi-skill/SKILL.md` from `skill-creator`'s template.
**Local skills are autodiscovered** — they are NOT listed in `ai-specs.toml`.
Commit them with the rest of the repo.

### Vendored skill (cloned from a Git repo)

```bash
specs-ai add-dep https://github.com/foo/superskill \
    --trigger "When doing X" \
    --license MIT
```

Appends a `[[deps]]` block to `ai-specs.toml` and runs `specs-ai sync`, which
clones the skill into `ai-specs/skills/<id>/`. Vendored skills are
**gitignored** — they're restored on every clone via `specs-ai sync`.

## Updating the CLI

```bash
cd ~/.specs-ai && git pull       # one global install, one update
```

The CLI lives only at `~/.specs-ai`. Projects don't carry a copy of the CLI —
they only carry their manifest, local skills, and the bundled `skill-creator` /
`skill-sync` skills (which they own and may customize).

## Layout (this repo)

```
specs-ai-cli/
├── bin/specs-ai                ← global entrypoint (dispatcher)
├── lib/
│   ├── init.sh                 ← bootstrap a project
│   ├── sync.sh                 ← vendor + regen + fan out
│   ├── sync-agent.sh           ← per-agent fan-out
│   ├── add-skill.sh            ← scaffold local skill
│   ├── add-dep.sh              ← register vendored skill
│   ├── version.sh
│   └── _internal/
│       ├── toml-read.py        ← read sections of ai-specs.toml
│       ├── deps-render.py      ← [[deps]] → vendor.manifest.toml
│       ├── gitignore-render.py ← [[deps]] → ai-specs/.gitignore
│       ├── mcp-render.py       ← [mcp.*] → per-agent format (merge-safe)
│       └── platform.sh         ← per-agent paths/keys
├── bundled-skills/             ← copied INTO each project on `init`
│   ├── skill-creator/          ← scaffolds new skills (template-driven)
│   └── skill-sync/             ← discovers SKILL.md, regenerates AGENTS.md table
├── templates/
│   ├── ai-specs.toml.tmpl
│   ├── AGENTS.md.tmpl
│   └── gitignore-root.tmpl
├── install.sh
├── VERSION
└── LICENSE
```

## License

MIT
