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
custom runtimes from scratch. It does not currently provision a new agent
runtime over frameworks like LangChain, nor does it define a universal agent
execution model. It assumes an existing coding tool/runtime and focuses on the
per-project harness around it.

Inspired by [`charliesbot/chai`](https://github.com/charliesbot/chai) (global
fan-out, merge-safe MCP) but **per-project** so different repos can have
different agents, skills, and MCP servers — and the configuration is
committable and shareable with a team.

In short: `ai-specs` is closer to a per-project operating layer for supported
coding tools than to an agent-building SDK.

## What's included (MVP v1)

| Feature | Status | Description |
|---------|--------|-------------|
| **Per-project manifest** | ✅ | `ai-specs/ai-specs.toml` as single source of truth |
| **Multi-agent fan-out** | ✅ | Claude, Cursor, OpenCode, Codex, Copilot, Gemini |
| **MCP server distribution** | ✅ | Merge-safe MCP config per agent (JSON, TOML) |
| **Skill management** | ✅ | Local, bundled, and vendored skills with autodiscovery |
| **AGENTS.md runtime brief** | ✅ | Concise operational context generated from `ai-specs.toml` |
| **Skill registry artifact** | ✅ | Auto-generated `ai-specs/.skill-registry.md` with skill index and Auto-invoke mappings |
| **Project initialization** | ✅ | `ai-specs init` scaffolds structure idempotently |
| **Dependency vendoring** | ✅ | `ai-specs add-dep` + `ai-specs sync` clones external skills |
| **Subrepo sync** | ✅ | Mirror derived artifacts to `project.subrepos` |
| **Read-only diagnostics** | ✅ | `ai-specs doctor` validates project health |
| **Context precedence** | ✅ | Skill documenting canonical resolution order |
| **Testing foundation** | ✅ | Default validation commands for SDD cycles |
| **SDD integration** | ✅ | Optional OpenSpec onboarding via `ai-specs sdd` |
| **Bundled skills** | ✅ | `skill-creator` + `skill-sync` + `skills-as-rules` command |
| **Recipes** | ✅ | Named, versioned bundles of skills, commands, templates, and MCP presets |
| **Lock-based updates** | ✅ | SHA-256 baseline tracking for safe skill updates |

## What's NOT included yet

These features are **explicitly deferred** to post-MVP (EPICs 2–7). They are
**not bugs** — they are roadmap items not yet implemented:

| Feature | Planned EPIC | Note |
|---------|-------------|------|
| **Memory / persistence layer** | EPIC 2 | No `[memory]` manifest section yet; no opencode-mem adapter |
| **Context Router** | EPIC 8 | No `ai-specs context plan` command; no deterministic scoring |
| **Packs / recipes** | — | Now implemented as `[recipes.<id>]` in V1 manifest |
| **Handoff automation** | EPIC 3 | No bundled `/handoff` command; no `docs/ai-memory/` structure |
| **Multi-device sync** | EPIC 6 | No sync beyond git; no cloud persistence |
| **Tracker adapters** | EPIC 7 | No Trello/Jira/GitHub Issues integration |
| **Semantic search** | Post-MVP | No embeddings or local vector search for skills/docs |
| **Coverage / linter / type-check** | Post-MVP | Testing foundation exists; stronger tooling not configured |
| **Build arbitrary agent runtimes** | Out of scope today | No LangChain-style "create a new agent from zero" harness generation |

> If you need any of these now, open an issue or vendor a skill via `[[deps]]`
> that implements the desired behavior locally.

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
# optional policy skills: see catalog/README.md in this repo (vendor via [[deps]])
ai-specs sync               # vendor deps + regen AGENTS.md + fan out root + declared subrepos
```

That's it — `.claude/`, `.cursor/`, `.opencode/`, `CLAUDE.md`, `.mcp.json`, etc.
are now generated from your manifest. Re-run `ai-specs sync` whenever the
manifest changes. If `project.subrepos` is declared, the root sync also mirrors
local derived artifacts into each subrepo so agents work from either location.
OpenCode receives project-local skills in `.opencode/skills/` and slash commands
in `.opencode/commands/`.

## Spec-driven development (`ai-specs sdd`)

Projects can declare optional **`[sdd]`** in `ai-specs/ai-specs.toml` and use **`ai-specs sdd`**
to verify the OpenSpec CLI (**`@fission-ai/openspec`** on npm, command `openspec`), align
`openspec/` (via `openspec init` when safe), merge non-destructive defaults into
`openspec/config.yaml`, and run **`ai-specs refresh-bundled --preset openspec`** so catalog
skills such as `openspec-sdd-conventions` and `testing-foundation` land under
`ai-specs/skills/` with the usual `.ai-specs.lock` behavior. That preset **copies from this
repo’s catalog and bundled commands**; it does **not** add `[[deps]]` rows — use
[`catalog/README.md`](catalog/README.md) if you want vendored clones via `ai-specs sync`.

Requirements when mutating (not for `sdd --dry-run`): **Node.js ≥ 20.19** and `openspec` on
`PATH`, unless you pass **`--install-provider-cli`** (runs `npm install -g @fission-ai/openspec@latest`;
requires `npm`).

```bash
ai-specs sdd --help
ai-specs sdd enable [path]
ai-specs sdd enable --artifact-store filesystem
ai-specs sdd enable --install-provider-cli   # explicit opt-in global install
ai-specs sdd status [path]
ai-specs sdd disable [path]
```

### `artifact_store` values

| Value | `openspec/` on disk | `doctor` when `[sdd].enabled` |
|-------|---------------------|------------------------------|
| `filesystem` | Required | ERROR if missing / unreadable config |
| `hybrid` | Required (same as filesystem) | May WARN about optional “memory” heuristics |
| `memory` | Optional (experimental) | WARN if tree missing; OpenSpec stays file-first in v1 |

### Minimal `[sdd]` example

```toml
[sdd]
enabled = true
provider = "openspec"
artifact_store = "filesystem"
```

With this block present, `ai-specs doctor` will validate that `openspec/config.yaml`
is readable, and `ai-specs sdd enable` will scaffold `openspec/` with a valid
base configuration for the `spec-driven` schema. See [`docs/ai/sdd.md`](docs/ai/sdd.md)
for the full SDD provider contract and generated command reference.

## Manifest and recipe references

`ai-specs/ai-specs.toml` remains the root source of truth, but the detailed
contract now lives in dedicated references:

- [`docs/ai-specs-toml.md`](docs/ai-specs-toml.md) — canonical manifest reference
- [`docs/recipe-schema.md`](docs/recipe-schema.md) — canonical recipe and recipe V2 reference
- [`docs/ai/sdd.md`](docs/ai/sdd.md) — SDD provider contract and OpenSpec workflow

Use the README for overview and navigation. Use the dedicated references for:

- supported manifest sections and defaults
- `[[bindings]]` and `[recipes.<id>.config]`
- recipe schema details such as `[[capabilities]]`, `[[hooks]]`, `[init]`, and recipe `[sdd]`
- explicit compatibility boundaries and deferred items

At a high level, recipes are named, versioned bundles of skills, commands,
templates, docs, and MCP presets materialized by `ai-specs sync`. The sync
pipeline validates version pins, copies declared primitives, resolves
capability bindings, and merges recipe MCP defaults with project manifest
precedence.

## Recommended skills (catalog)

Optional policy skills ship in this repo under [`catalog/skills/`](catalog/skills/). Vendor them
with `[[deps]]` and `path` (see [`catalog/README.md`](catalog/README.md)) — same flow as any
external skill.

## Context precedence

After vendoring `context-precedence` from the catalog, the rule lives in
[`ai-specs/skills/context-precedence/SKILL.md`](ai-specs/skills/context-precedence/SKILL.md).
The skill is listed in `ai-specs/.skill-registry.md` once the skill exists and you run `ai-specs sync`.

## Testing foundation

[`ai-specs/skills/testing-foundation/SKILL.md`](ai-specs/skills/testing-foundation/SKILL.md) is
available from the catalog the same way. Use `./tests/validate.sh` as the default final verification command until stronger tooling is configured.

OpenSpec `config.yaml` shape and apply-time commit conventions:
[`ai-specs/skills/openspec-sdd-conventions/SKILL.md`](ai-specs/skills/openspec-sdd-conventions/SKILL.md).

## What gets created in your project

```
my-project/
├── AGENTS.md                       ← runtime brief generated from manifest (do not edit by hand)
├── .gitignore                      ← appended with an ai-specs block (gitignores agent files)
├── .recipe/                        ← recipe-bundled skills (gitignored; restored by sync)
│   └── <recipe-id>/
│       ├── skills/<skill-id>/
│       └── overrides/              ← optional runtime overrides per recipe
├── .deps/                          ← vendored dependency skills (gitignored; restored by sync)
│   └── <dep-id>/
│       └── skills/<skill-id>/
└── ai-specs/
    ├── ai-specs.toml               ← YOUR manifest (edit this)
    ├── .gitignore                  ← derived; ignores resolved-skills dir
    ├── .skill-registry.md          ← generated skill index + Auto-invoke mappings (do not edit by hand)
    ├── .resolved-skills/           ← flattened resolved skill tree (gitignored; used by agents)
    ├── skills/
    │   ├── skill-creator/          ← bundled on init (contract)
    │   ├── skill-sync/             ← bundled on init (contract)
    │   └── <your-local-skill>/     ← created with `/skills-as-rules` (committed)
    └── commands/
        └── <your-local-command>.md ← fanned out to native agent command dirs
```

### Three-tier skill layout

Skills are resolved from three isolated sources with deterministic precedence:

1. **Local** (`ai-specs/skills/<id>/`) — highest precedence; committed project-owned skills
2. **Recipe** (`.recipe/<recipe-id>/skills/<id>/`) — middle precedence; bundled by enabled recipes
3. **Dependency** (`.deps/<dep-id>/skills/<id>/`) — lowest precedence; cloned from `[[deps]]`

When the same skill ID exists in multiple sources, the higher-precedence source
wins automatically. Local skills silently override recipe and dep versions
without error or warning.

| Category   | Lives in                                  | Listed in toml? | Committed? | Created by |
|------------|-------------------------------------------|-----------------|------------|------------|
| Local      | `ai-specs/skills/<name>/`                 | No (autodiscovered) | Yes | `/skills-as-rules` |
| Recipe     | `.recipe/<recipe-id>/skills/<name>/`      | Yes (`[recipes.*]`) | No (gitignored) | `ai-specs sync` |
| Dependency | `.deps/<dep-id>/skills/<name>/`           | Yes (`[[deps]]`)    | No (gitignored) | `ai-specs add-dep <url>` → cloned by sync |

Recipe-bundled and vendored skills are kept outside `ai-specs/skills/` so your
local skill directory stays clean and commit-ready. On every `ai-specs sync`,
the CLI rebuilds `.recipe/` and `.deps/` from the manifest, removes orphaned
directories for disabled recipes or deleted deps, and flattens the resolved
skill tree into `ai-specs/.resolved-skills/` for agent consumption.

### Runtime overrides for recipe skills

Each recipe can provide optional overrides without forking the recipe code:

- `.recipe/<recipe-id>/overrides/config.toml` — merged on top of bundled defaults
- `.recipe/<recipe-id>/overrides/templates/<name>.md` — overrides identically-named bundled templates

Overrides are scoped to their parent recipe and do not leak into other recipes
or dependencies. Missing override files are handled gracefully (no error, no
warning). The `overrides/` directory is gitignored so teams can share the base
recipe while allowing individual customizations.

## CLI

| Command | Description |
|---------|-------------|
| `ai-specs init [path] [--name N] [--force]` | Bootstrap `ai-specs/` (idempotent; never touches your `ai-specs.toml`). `--force` re-copies bundled skills/commands & regenerates AGENTS.md |
| `ai-specs sync [path]` | Resolve `[root, ...project.subrepos]`, refresh bundled, vendor `[[deps]]` once, regen AGENTS.md runtime brief + `ai-specs/.skill-registry.md`, then fan out local derived artifacts per target + per agent |
| `ai-specs sync-agent [path] [--all|--<agent>]` | Fan out per-agent only for the current target (no vendoring/regen) |
| `ai-specs doctor [path]` | Read-only health check for manifest, bundled assets, enabled agents, symlinks, and MCP outputs (does not modify files) |
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
| Cursor   | Yes                       | No (skills via filesystem + registry)  | `.cursor/mcp.json` |
| OpenCode | Yes                       | No                                     | `opencode.json` |
| Codex    | Yes                       | No                                     | `.codex/config.toml` |
| Copilot  | No (`.github/copilot-instructions.md`) | No                          | `.github/copilot-instructions.md` symlink |
| Gemini   | No (needs `GEMINI.md`)    | Yes (`.gemini/skills/<name>/SKILL.md`) | `GEMINI.md` symlink + `.gemini/skills` symlink + `.gemini/settings.json` |

The canonical skill index and Auto-invoke mappings live in
`ai-specs/.skill-registry.md` and are regenerated automatically by `skill-sync`
whenever you `ai-specs sync` or run `/skills-as-rules`.

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

You own **bundled** skills (`skill-creator`, `skill-sync`), **vendored** skills (from `[[deps]]`,
including optional [catalog](catalog/README.md) entries), and `skills-as-rules` — customize them
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
they only carry their manifest, local skills, and the bundled skills shipped
from `bundled-skills/` (which they own and may customize).

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
├── bundled-skills/             ← copied on `init` (contracts only)
│   ├── skill-creator/          ← scaffolds new skills (template-driven)
│   └── skill-sync/             ← discovers SKILL.md, regenerates AGENTS.md table
├── catalog/skills/             ← optional skills; vendor via [[deps]] (see catalog/README.md)
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
