# ai-specs

One declarative manifest per project — fan out to Claude, Cursor, OpenCode, Codex, Copilot, and Gemini.

`ai-specs` is a per-project standard for managing AI agent configuration: skills,
MCP servers, and per-agent instruction files. Each project owns its manifest at
`ai-specs/ai-specs.toml`; the global `ai-specs` CLI distributes that manifest
into every enabled agent's native format.

Inspired by [`charliesbot/chai`](https://github.com/charliesbot/chai) (global
fan-out, merge-safe MCP) but **per-project** so different repos can have
different agents, skills, and MCP servers — and the configuration is
committable and shareable with a team.

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
ai-specs sync               # vendor deps + regen AGENTS.md + fan out root + declared subrepos
```

That's it — `.claude/`, `.cursor/`, `.opencode/`, `CLAUDE.md`, `.mcp.json`, etc.
are now generated from your manifest. Re-run `ai-specs sync` whenever the
manifest changes. If `project.subrepos` is declared, the root sync also mirrors
local derived artifacts into each subrepo so agents work from either location.
OpenCode receives project-local skills in `.opencode/skills/` and slash commands
in `.opencode/commands/`.

## Manifest V1 contract (`ai-specs/ai-specs.toml`)

`ai-specs/ai-specs.toml` in the project root is the ONLY V1 source of truth.
This change documents the runtime contract the CLI already consumes today — no
new schema engine, no precedence rules, no `doctor` behavior, and no `[memory]`
section are introduced here.

Canonical V1 surface:

- `[project]`
- `[agents]`
- `[[deps]]`
- `[mcp.<name>]`

No other manifest sections are part of the canonical V1 surface for this change.

Conservative compatibility rules:

- Missing `[agents]`, `[[deps]]`, and `[mcp]` remain valid and normalize to stable defaults.
- `project.subrepos` remains validated by the existing root target resolver.
- MCP `env` is the canonical field name.
- MCP `environment` is still accepted as a tolerated input alias and normalizes to `env`.

Field classification in V1:

| Surface | Fields | Status |
|---------|--------|--------|
| `[project]` | `name` | optional, default `""` |
| `[project]` | `subrepos` | optional, default `[]`, validated as root-relative target paths |
| `[agents]` | `enabled` | optional, default `[]` |
| `[[deps]]` | `id`, `source` | only required minimum fields |
| `[[deps]]` | `path`, `scope`, `auto_invoke`, `license`, `vendor_attribution` | optional passthrough fields consumed by vendoring/rendering |
| `[mcp.<name>]` | `command` | optional |
| `[mcp.<name>]` | `args` | optional, default `[]` |
| `[mcp.<name>]` | `env` | optional canonical field, default `{}` |
| `[mcp.<name>]` | `environment` | tolerated input alias of `env` |
| `[mcp.<name>]` | `timeout` | optional |
| `[mcp.<name>]` | `enabled` | tolerated passthrough field |

Out of scope for this V1 contract (explicitly deferred to future changes):

- precedence / merge policy beyond the currently implemented runtime behavior
- `ai-specs doctor`
- `[memory]`
- introducing new manifest sections

## Context precedence

The canonical conflict-resolution rule lives in [`docs/ai/context-precedence.md`](docs/ai/context-precedence.md).
README only points to that document so the precedence policy has one source of truth.

## Testing foundation

The canonical MVP testing policy lives in [`docs/ai/testing-foundation.md`](docs/ai/testing-foundation.md).
Use `./tests/validate.sh` as the default final verification command until stronger tooling is configured.

## What gets created in your project

```
my-project/
├── AGENTS.md                       ← generated artifact (do not edit; managed by skill-sync)
├── .gitignore                      ← appended with an ai-specs block (gitignores agent files)
└── ai-specs/
    ├── ai-specs.toml               ← YOUR manifest (edit this)
    ├── .gitignore                  ← derived; lists vendored skill dirs
    ├── skills/
        ├── skill-creator/          ← bundled (committable; customize freely)
        ├── skill-sync/             ← bundled (committable; customize freely)
        ├── <your-local-skill>/     ← creada con `/skills-as-rules` (committed)
        └── <vendored-skill>/       ← cloned from [[deps]] (gitignored)
    └── commands/
        └── <your-local-command>.md ← fanned out to native agent command dirs
```

### Three skill categories

| Category   | Lives in            | Listed in toml? | Committed? | Created by             |
|------------|---------------------|-----------------|------------|------------------------|
| Local      | `ai-specs/skills/<name>/`   | No (autodiscovered) | Yes        | `/skills-as-rules` |
| Bundled    | `ai-specs/skills/{skill-creator,skill-sync}/` | No | Yes (own-and-customize) | `ai-specs init` (one-time copy) |
| Vendored   | `ai-specs/skills/<dep-id>/` | Yes (`[[deps]]`)    | No (gitignored) | `ai-specs add-dep <url>` → cloned by sync |

## CLI

| Command | Description |
|---------|-------------|
| `ai-specs init [path] [--name N] [--force]` | Bootstrap `ai-specs/` (idempotent; never touches your `ai-specs.toml`). `--force` re-copies bundled skills/commands & regenerates AGENTS.md |
| `ai-specs sync [path]` | Resolve `[root, ...project.subrepos]`, refresh bundled, vendor `[[deps]]` once, regen AGENTS.md auto-invoke, then fan out local derived artifacts per target + per agent |
| `ai-specs sync-agent [path] [--all|--<agent>]` | Fan out per-agent only for the current target (no vendoring/regen) |
| `ai-specs refresh-bundled [path]` | Update bundled skills/commands from the CLI — keeps your edits, drops `.new` sidecars for files you customized |
| `ai-specs add-dep <git-url> [path]` | Register a vendored skill in `[[deps]]` and `sync` |
| `ai-specs version` | Print CLI version |
| `ai-specs help` | Show help |

Every subcommand accepts an optional `[path]` (defaults to `cwd`) and `--help`.

> **`ai-specs.toml` is never overwritten.** It's your source of truth: enabled agents, `[[deps]]`, `[mcp.*]`. Mutated only by `add-dep` or by you.

## How MCP distribution works

`[mcp.*]` entries in `ai-specs.toml` are rendered into each agent's native
config via a **merge-safe** strategy: `ai-specs` owns the MCP key (e.g.
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
`skill-sync` whenever you `ai-specs sync` or run `/skills-as-rules`.

## Root + subrepo sync

`project.subrepos` is now an active sync input. When declared, `ai-specs sync`
resolves targets as `[root, ...project.subrepos]`, validates each path before
writing anything, vendors external skills ONCE in the root workspace, and then
mirrors a local derived artifact set into every target:

- `AGENTS.md`
- `ai-specs/.gitignore`
- `ai-specs/skills/**`
- `ai-specs/commands/**`
- per-agent configs/symlinks such as `CLAUDE.md`, `.mcp.json`, `.cursor/mcp.json`, `opencode.json`

Versioning policy: these subrepo files are derived outputs from the latest root
sync run. Do not hand-edit them, and do not add a subrepo `ai-specs.toml` in V1.

Failure semantics: `.gitmodules` is advisory-only in V1. If a declared subrepo
path is invalid or a target cannot be updated, `sync` stops on the FIRST failure
with explicit target reporting. Previous writes are not rolled back.

## Adding skills

### Local skill (lives in your project)

```bash
/skills-as-rules
```

This is the official workflow for local skills. The slash command runs inside
your agent, asks one convention at a time, uses `skill-creator` to author the
skill, and then runs `skill-sync` so `AGENTS.md` stays aligned.

Result: a committed local skill at `ai-specs/skills/<name>/SKILL.md`.
**Local skills are autodiscovered** — they are NOT listed in `ai-specs.toml`.

### Vendored skill (cloned from a Git repo)

```bash
ai-specs add-dep https://github.com/foo/superskill \
    --trigger "When doing X" \
    --license MIT
```

Appends a `[[deps]]` block to `ai-specs.toml` and runs `ai-specs sync`, which
clones the skill into `ai-specs/skills/<id>/`. Vendored skills are
**gitignored** — they're restored on every clone via `ai-specs sync`.

## Updating bundled skills & commands

You own `skill-creator`, `skill-sync`, and `skills-as-rules` — customize them
freely. When the CLI ships new versions, `ai-specs sync` (or the standalone
`ai-specs refresh-bundled`) reconciles them against your edits using a
SHA-256 baseline at `ai-specs/.ai-specs.lock` (committable).

| Your file vs baseline | Upstream changed? | What happens |
|-----------------------|-------------------|--------------|
| Untouched | Yes | **Auto-updated** to the new CLI version |
| Untouched | No  | Nothing (silent) |
| Customized | Yes | CLI version saved as `<name>.new` alongside yours — diff & merge by hand |
| Customized | No  | Your edits stand |
| Deleted by you | — | Respected (stops tracking) |

The lock file records what the CLI shipped to your project last time. Commit
it so teammates stay on the same baseline.

## Updating the CLI

### Existing installation

If you already installed `ai-specs`, the fastest update path is:

```bash
cd ~/.ai-specs && git pull
```

Then refresh any project that should receive the newest bundled skills,
commands, and generated artifacts:

```bash
cd <your-project>
ai-specs sync
```

### Safe re-install / upgrade

Re-running the installer is also safe; it updates the existing clone and
recreates the symlink:

```bash
curl -fsSL https://raw.githubusercontent.com/parada1104/ai-specs-cli/main/install.sh | bash
```

```bash
cd ~/.ai-specs && git pull       # one global install, one update
```

The CLI lives only at `~/.ai-specs`. Projects don't carry a copy of the CLI —
they only carry their manifest, local skills, and the bundled `skill-creator` /
`skill-sync` skills (which they own and may customize).

## Layout (this repo)

```
ai-specs-cli/
├── bin/ai-specs                ← global entrypoint (dispatcher)
├── lib/
│   ├── init.sh                 ← bootstrap a project
│   ├── sync.sh                 ← resolve targets + root refresh + target fan-out
│   ├── sync-agent.sh           ← render one target from the root manifest
│   ├── add-dep.sh              ← register vendored skill
│   ├── version.sh
│   └── _internal/
│       ├── toml-read.py        ← read sections of ai-specs.toml
│       ├── vendor-skills.py    ← clones [[deps]] → ai-specs/skills/<id>/
│       ├── gitignore-render.py ← [[deps]] → ai-specs/.gitignore
│       ├── agents-md-render.py ← skills/ → AGENTS.md
│       ├── mcp-render.py       ← [mcp.*] → per-agent format (merge-safe)
│       └── platform.sh         ← per-agent paths/keys
├── bundled-skills/             ← copied INTO each project on `init`
│   ├── skill-creator/          ← scaffolds new skills (template-driven)
│   └── skill-sync/             ← discovers SKILL.md, regenerates AGENTS.md table
├── bundled-commands/           ← copied INTO ai-specs/commands/ on `init`
│   └── skills-as-rules.md      ← interactive skill authoring slash command
├── templates/
│   ├── ai-specs.toml.tmpl
│   └── gitignore-root.tmpl
├── install.sh
├── VERSION
└── LICENSE
```

## License

MIT
