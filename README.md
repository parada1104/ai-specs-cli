# specs-ai

One declarative manifest per project — fan-out to Claude, Cursor, OpenCode, Codex, Copilot, and Gemini.

`specs-ai` is a per-project standard for managing AI agent configuration: skills,
MCP servers, and agent-specific instruction files. Each project owns its
manifest at `ai-specs/ai-specs.toml`; the CLI distributes that manifest to every
enabled agent in their native format.

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
specs-ai init-project                    # scaffolds ai-specs/ + AGENTS.md + .gitignore
# edit ai-specs/ai-specs.toml — add your MCP servers, [[deps]]
./ai-specs/cli/ai-specs sync             # vendor deps + distribute to agents
```

That's it — `.claude/`, `.cursor/`, `.opencode/`, `CLAUDE.md`, `.mcp.json`, etc.
are now generated from your manifest.

## What gets created in your project

```
my-project/
├── AGENTS.md                      ← root agent instructions (template, edit freely)
├── .gitignore                     ← appended with ai-specs block
└── ai-specs/
    ├── ai-specs.toml              ← YOUR manifest (edit this)
    ├── cli/                       ← payload (do not edit; refresh via specs-ai upgrade)
    │   ├── ai-specs               ← per-project entrypoint
    │   ├── init.sh
    │   ├── sync-agent.sh
    │   ├── add-skill.sh
    │   ├── add-dep.sh
    │   └── lib/
    │       ├── toml-read.py
    │       ├── mcp-render.py
    │       ├── deps-render.py
    │       └── platform.sh
    └── skills/
        ├── skill-creator/         ← payload (do not edit)
        ├── skill-sync/            ← payload (do not edit)
        ├── <your-local-skill>/    ← skills you create (autodiscovered)
        └── <vendored-skill>/      ← from [[deps]]
```

## CLI

### Global commands (after install)

| Command | Description |
|---------|-------------|
| `specs-ai init-project [path]` | Bootstrap `ai-specs/` in target dir (default: cwd) |
| `specs-ai upgrade [path]` | Refresh `cli/` + bundled skills from latest payload |
| `specs-ai version` | Print CLI version |
| `specs-ai help` | Show help |

### Per-project commands (after `init-project`)

| Command | Description |
|---------|-------------|
| `./ai-specs/cli/ai-specs init` | Vendor `[[deps]]`, regenerate `AGENTS.md` |
| `./ai-specs/cli/ai-specs sync-agent --all` | Distribute to every enabled agent |
| `./ai-specs/cli/ai-specs sync-agent --claude --cursor` | Distribute to selected agents |
| `./ai-specs/cli/ai-specs add-skill <name>` | Scaffold a local skill |
| `./ai-specs/cli/ai-specs add-dep <git-url>` | Add a vendored skill (mutates manifest) |
| `./ai-specs/cli/ai-specs sync` | `init` + `sync-agent --all` |

All accept `--dry-run` (where applicable) and `--help`.

## How MCP distribution works

`[mcp.*]` entries in `ai-specs.toml` are rendered into each agent's config file
using a **merge-safe** strategy: `specs-ai` owns the MCP key (e.g.
`mcpServers`), every other top-level key is preserved.

| Agent    | Target file              | Key            | Format |
|----------|--------------------------|----------------|--------|
| Claude   | `.mcp.json`              | `mcpServers`   | JSON   |
| Cursor   | `.cursor/mcp.json`       | `mcpServers`   | JSON   |
| OpenCode | `opencode.json`          | `mcp`          | JSON   |
| Codex    | `.codex/config.toml`     | `mcp_servers`  | TOML   |
| Gemini   | `.gemini/settings.json`  | `mcpServers`   | JSON   |
| Copilot  | (no MCP support)         | —              | —      |

## Adding skills

### Local skill (lives in your project)

```bash
./ai-specs/cli/ai-specs add-skill mi-skill \
    --description "What it does" \
    --trigger "When the AI should load it"
```

Scaffolds `ai-specs/skills/mi-skill/SKILL.md` from the template. **Local skills
are autodiscovered** from the filesystem — they are NOT listed in
`ai-specs.toml`.

### Vendored skill (cloned from a Git repo)

```bash
./ai-specs/cli/ai-specs add-dep https://github.com/foo/superskill \
    --auto-invoke "When doing X" \
    --license MIT
```

Appends a `[[deps]]` block to `ai-specs.toml` and re-runs `init` to clone the
skill into `ai-specs/skills/<id>/`.

## Updating the CLI in an existing project

When `specs-ai` itself improves, refresh the payload bundled in your project:

```bash
specs-ai upgrade               # in cwd
specs-ai upgrade ~/code/foo    # specific path
```

This overwrites `ai-specs/cli/` and the bundled `skill-creator` /
`skill-sync` skills. Your `ai-specs.toml`, local skills, and vendored deps are
preserved.

## Layout (this repo)

```
specs-ai-cli/
├── bin/specs-ai                ← global entrypoint
├── lib/
│   ├── init-project.sh         ← scaffolds ai-specs/ in a target dir
│   ├── upgrade.sh              ← refreshes payload in an existing project
│   └── version.sh
├── payload/                    ← what gets copied INTO each project's ai-specs/
│   ├── cli/                    ← per-project CLI (init / sync-agent / add-skill / add-dep)
│   └── skills/
│       ├── skill-creator/
│       └── skill-sync/
├── templates/
│   ├── ai-specs.toml.tmpl
│   ├── AGENTS.md.tmpl
│   └── gitignore.tmpl
├── install.sh
└── VERSION
```

## License

MIT
