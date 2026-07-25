# Vault Canonical Store recipe

Provides the **`canonical-store`** capability: durable decisions and handoffs in
a configured vault MCP (e.g. Obsidian over a filesystem MCP server). This is the
concrete (tier-2) provider that foundational recipes such as `session-context`
consume when they need a canonical store.

## What it provides

- **Skill `vault-context`** — when and how to read/write canonical decisions and
  handoffs, note shapes, and the rule of "Vault for durable record, operational
  memory for searchable continuity".
- **Obsidian skills (kepano)** — vendored on enable/sync from
  [`kepano/obsidian-skills`](https://github.com/kepano/obsidian-skills):
  `obsidian-markdown`, `obsidian-bases`, `json-canvas`, `obsidian-cli`, `defuddle`.
- **Capability `canonical-store`** — so a project can bind it wherever a
  canonical store is needed.
- **MCP preset `vault-canonical`** — launches
  `@modelcontextprotocol/server-filesystem@2025.7.1` via a small wrapper that
  reads **`CANONICAL_VAULT_PATH` from the environment** (not from a `${VAR}` argv).

## Why a wrapper (not `${CANONICAL_VAULT_PATH}` in args)

Several agent hosts expand `${VAR}` in `.mcp.json` **only if the variable is
already in the host process environment** when the config is parsed. A bare
argv of `"${CANONICAL_VAULT_PATH}"` then reaches the filesystem server as a
literal string (or empty). Workarounds like `"~/${path}"` can appear to work
for home-relative values, but the durable model is:

1. Set `CANONICAL_VAULT_PATH` to an **absolute** scoped vault directory.
2. Pass that var through MCP `env`.
3. Let `vault-fs-mcp.sh` resolve it at exec time and pass one argv to the server.

## Enable

```toml
[recipes.vault-canonical-store]
enabled = true
version = "1.2.0"

[recipes.vault-canonical-store.config]
decisions_folder = "decisiones"
sessions_folder = "sessions"
vault_scope = "nnodes/proyectos/my-project"
```

Prefer **direnv** with harness values in `ai-specs/.env` (and optional app `.env`
at the repo root). `ai-specs configure-recipes` writes a managed block in
project-root `.envrc` that loads both via `dotenv_if_exists`. Quote values that
contain spaces (Obsidian iCloud paths under `Mobile Documents/`):

```bash
# ai-specs/.env — good (fully resolved absolute path)
CANONICAL_VAULT_PATH="/Users/you/Library/Mobile Documents/iCloud~md~obsidian/Documents/my-vault/nnodes/proyectos/my-project"
```

Avoid leaving nested `$OBSIDIAN_VAULT_PATH/...` **unexpanded** if the agent is
launched without direnv (IDE / Dock) — the wrapper rejects paths that still
contain `$`.

Then run `ai-specs sync` and **restart the agent** so it picks up env + MCP.
Sync materializes `ai-specs/recipes/vault-canonical-store/bin/vault-fs-mcp.sh`
and points every enabled harness at:

```text
command: bash
args:    [ai-specs/recipes/vault-canonical-store/bin/vault-fs-mcp.sh]
env:     CANONICAL_VAULT_PATH=${CANONICAL_VAULT_PATH}
```

### Prove the MCP path without a release / without sync

The wrapper can be smoke-tested against the real filesystem MCP (no LLM, no
published CLI). From a checkout that contains this recipe (e.g. the feature
worktree):

```bash
# temp scope with spaces in the path
python3 tests/smoke_vault_mcp_fs.py

# or your absolute project scope
python3 tests/smoke_vault_mcp_fs.py --path "$CANONICAL_VAULT_PATH"
```

Success prints `SMOKE_OK` and shows `Allowed directories: [<your path>]`.

Live host connect (Claude Code / Cursor Agent) is opt-in via
`EVALS_SCENARIOS=ac_mcp_live_scope ./tests/evals/run-live-vault.sh`. Note:
Claude Code 2.1.x fails tools-fetch on the recipe pin `2025.7.1`; modern
filesystem MCP builds also replace argv dirs with client **roots**, so vault
scopes outside the workspace need the host to include that directory (e.g.
Claude `--add-dir "$CANONICAL_VAULT_PATH"`). Obsidian CLI is **not** a
subdirectory sandbox — it can operate on the whole vault.

To sync a consumer project with the **unreleased** recipe, point the CLI home
at the checkout (no GitHub release required):

```bash
AI_SPECS_HOME=/path/to/ai-specs-cli/.worktrees/vault-canonical-reinforce \
  /path/to/ai-specs-cli/.worktrees/vault-canonical-reinforce/bin/ai-specs sync
```

## Config

| Key | Default | Meaning |
|---|---|---|
| `vault_scope` | — | Optional hint for the vault scope/path the agent should stay within. |
| `decisions_folder` | `decisiones` | Folder for decisions and conventions. |
| `sessions_folder` | `sessions` | Folder for session summaries and handoffs. |

## Capability

| Capability | Meaning |
|---|---|
| `canonical-store` | Durable decisions/handoffs store. |

Other providers (Notion, a git-tracked docs folder, etc.) can be future sibling
recipes that also provide `canonical-store`; a project picks one via
`[[bindings]]`.
