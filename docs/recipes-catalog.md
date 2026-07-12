# Recipe catalog

The recipes `ai-specs` ships in its catalog, what each one is for, and the
config it expects in `ai-specs/ai-specs.toml`. Enable a recipe with
`[recipes.<id>] enabled = true` (pin `version`), set any config under
`[recipes.<id>.config]`, then run `ai-specs sync` to materialize its skills,
commands, templates, and docs.

- For the **schema** of `recipe.toml` and `[config]` fields, see
  [`docs/recipe-schema.md`](recipe-schema.md).
- For the **manifest** side (`[recipes.<id>]`, `[recipes.<id>.config]`,
  `[[bindings]]`), see [`docs/ai-specs-toml.md`](ai-specs-toml.md).
- For the **capability model** (how recipes compose), see
  [`docs/capabilities.md`](capabilities.md).

Run `ai-specs recipe list` to see which catalog recipes are installed vs.
available in a project. The `test-*` entries are test fixtures, not real
recipes — ignore them.

## Two tiers

Recipes come in two tiers (see [`docs/capabilities.md`](capabilities.md)):

- **Foundational** — a reusable, tool-agnostic *pattern*. Speaks in terms of
  capabilities, never a vendor. Configurable.
- **Specific** — a concrete integration that *provides* a capability for a named
  tool/service.

A project assembles its harness by picking foundational recipes plus a concrete
provider per capability; swapping the provider (Trello → Jira, GitHub → GitLab)
never touches the foundational layer.

## At a glance

| Recipe | Tier | Focus | Provides capability | Installs MCP | Key config |
|--------|------|-------|---------------------|--------------|------------|
| [`session-context`](#session-context) | Foundational | Session-start focus resolution + conflict policy | `session-bootstrap`, `conflict-policy` | — | — (consumes `memory`, `tracker`, `canonical-store`) |
| [`tdd-flow`](#tdd-flow) | Foundational | Red-green-refactor with a configurable test command | `test-runner` | — | `test_command` |
| [`plan-build-flow`](#plan-build-flow) | Foundational | Ambient skill-only plan/build workflow (no slash commands) | `plan-build-flow` | — | — |
| [`worktree-flow`](#worktree-flow) | Foundational | Isolated `.worktrees/` + safe post-merge cleanup | `worktree-isolation`, `worktree-cleanup` | — | `worktrees_dir`, `integration_branch`, `auto_remove_merged`, `WORKTREE_GATE_PROTECTED` |
| [`git-pr-flow`](#git-pr-flow) | Specific | Branch → PR → approval-gated merge (GitHub) | `vcs-pr-flow` | — | `base_branch` |
| [`gitlab-mr-flow`](#gitlab-mr-flow) | Specific | Branch → MR → approval-gated merge (GitLab) | `vcs-pr-flow` | — | `base_branch` |
| [`bitbucket-pr-flow`](#bitbucket-pr-flow) | Specific | Branch → PR → approval-gated merge (Bitbucket) | `vcs-pr-flow` | — | `base_branch` |
| [`trello-mcp-workflow`](#trello-mcp-workflow) | Specific | Trello board integration (tracker) | `tracker` (+ 4 trello-* capabilities) | `trello` | **`board_id` (required)**, `default_list`, `epic_list` |
| [`vault-canonical-store`](#vault-canonical-store) | Specific | Durable decisions/handoffs in a vault MCP | `canonical-store` | `vault-canonical` | `vault_scope`, `decisions_folder`, `sessions_folder` |

---


## CLI prerequisites

Recipes that shell out to external CLIs declare them via `[[deps.cli]]` in
`recipe.toml`. `ai-specs doctor` emits WARN (required) / INFO (optional) when a
binary is missing; the config wizard shows the same guidance. Install is always
manual.

| Recipe | Binary | Purpose | Required |
|--------|--------|---------|----------|
| `git-pr-flow` | `gh` | GitHub PR create/merge | yes |
| `gitlab-mr-flow` | `glab`, `jq` | GitLab MR flow + JSON parsing | yes |
| `bitbucket-pr-flow` | `bb` | Bitbucket PR create/merge | yes |
| `trello-mcp-workflow` | `npx` | Trello MCP server runtime | yes |
| `vault-canonical-store` | `npx` | Filesystem MCP server runtime | yes |
| `worktree-flow` | `git` | Worktree add/remove/cleanup | yes |
| `tdd-flow` | — | Test command is config-driven | — |


## session-context

**Tool-agnostic session-start discipline.** Resolves the active focus from the
`memory` capability first, then the runtime brief (`AGENTS.md`), and only
cross-checks the `tracker` when gaps or contradictions remain. Bundles the
canonical conflict-resolution policy used when context sources disagree. Catches
referential ambiguity ("siguiente card", "continuar", "apply" without a named
change) and asks one concrete question instead of guessing.

- **Provides:** skills `session-bootstrap`, `context-precedence`; capabilities
  `session-bootstrap`, `conflict-policy`.
- **Consumes (by convention):** `memory`, `tracker`, `canonical-store` — bind
  concrete providers in the manifest.
- **Config:** none.
- **Full README:** [`catalog/recipes/session-context/README.md`](../catalog/recipes/session-context/README.md)

```toml
[recipes.session-context]
enabled = true
version = "2.0.0"
```

## tdd-flow

**Red-green-refactor discipline for any project.** Write a failing test (RED),
make it pass minimally (GREEN), refactor — and record RED/GREEN evidence before
merge. The test command is *configuration*: the recipe never hardcodes how your
project runs tests. If `test_command` is unset, the skill asks rather than
guessing.

- **Provides:** skill `tdd-flow`, command `/tdd`; capability `test-runner`.
- **Config:**

  | Key | Type | Required | Default | Description |
  |-----|------|----------|---------|-------------|
  | `test_command` | string | no | _(none)_ | Exact command run for every RED/GREEN step. Project-specific — no default. |

- **Full README:** [`catalog/recipes/tdd-flow/README.md`](../catalog/recipes/tdd-flow/README.md)

```toml
[recipes.tdd-flow]
enabled = true
version = "1.0.0"

[recipes.tdd-flow.config]
test_command = "./tests/run.sh"
```

## plan-build-flow

**Ambient skill-only change workflow.** The bundled skill auto-invokes on
substantial requests to produce reviewable planning artifacts, waits for human
authorization, then implements, validates, and closes the change — without
`/plan` or `/build` commands. Implementation defers to an isolated-worktree
workflow when one is enabled, without hard-depending on it.

- **Provides:** skill `plan-build-flow`; capability `plan-build-flow`.
- **Config:** none — change slug and artifact store resolve per session.
- **Full README:** [`catalog/recipes/plan-build-flow/README.md`](../catalog/recipes/plan-build-flow/README.md)

```toml
[recipes.plan-build-flow]
enabled = true
version = "1.0.0"
```

## worktree-flow

**Isolated git worktrees under `.worktrees/` with safe post-merge cleanup.**
File-writing change work runs in a dedicated worktree; pure exploration stays
outside one. The bundled cleanup script removes only merged + clean worktrees
(detecting regular, squash, and rebase merges by patch-id), preserves dirty and
unmerged ones, and never touches the main worktree.

- **Provides:** skill `worktree-flow`, commands `/worktree-new`,
  `/worktree-clean`, script `bin/worktree-cleanup.sh`, and a `worktree-gate`
  runtime hook (`[[provides.hooks]]`) that blocks writes to the main worktree on
  a protected branch and supports `gate_mode` dispatch (`always` / `ask` /
  `off`) — see [`docs/runtime-hooks.md`](runtime-hooks.md);
  capabilities `worktree-isolation`, `worktree-cleanup`.
- **Config:**

  | Key | Type | Default | Description |
  |-----|------|---------|-------------|
  | `worktrees_dir` | string | `.worktrees` | Directory holding per-change worktrees. |
  | `integration_branch` | string | `main` | Branch worktrees are created from and merged into. |
  | `auto_remove_merged` | boolean | `true` | Whether merged worktrees are eligible for cleanup. |
  | `gate_mode` | string | `always` | Main-worktree gate mode. `always` keeps the current block, `ask` blocks with a bypass hint, and `off` disables the gate. |
  | `WORKTREE_GATE_PROTECTED` | string | `main development` | Space-separated branch names where the `worktree-gate` hook blocks Edit/Write in the main worktree. Passed to the rendered hook as the `WORKTREE_GATE_PROTECTED` env var. |

- **Full README:** [`catalog/recipes/worktree-flow/README.md`](../catalog/recipes/worktree-flow/README.md)

```toml
[recipes.worktree-flow]
enabled = true
version = "1.2.0"

[recipes.worktree-flow.config]
integration_branch = "development"
gate_mode = "always"
```

## git-pr-flow

**GitHub branch → PR → merge flow.** Pushes the branch, opens a PR against the
configured base, and merges *only* after explicit user approval. Uses the `gh` CLI.
Sibling recipes cover GitLab and Bitbucket; select the host through `[[bindings]]`.

- **Provides:** skill `git-merge-workflow`, command `/pr-create`; capability
  `vcs-pr-flow`.
- **Config:**

  | Key | Type | Required | Default | Description |
  |-----|------|----------|---------|-------------|
  | `base_branch` | string | no | `main` | Base branch the PR targets. |

- **Full README:** [`catalog/recipes/git-pr-flow/README.md`](../catalog/recipes/git-pr-flow/README.md)

```toml
[recipes.git-pr-flow]
enabled = true
version = "1.2.0"

[recipes.git-pr-flow.config]
base_branch = "development"

[[bindings]]
capability = "vcs-pr-flow"
recipe = "git-pr-flow"
```

## gitlab-mr-flow

**GitLab branch → MR → merge flow.** Uses the `glab` CLI. Sibling of
[`git-pr-flow`](#git-pr-flow) and [`bitbucket-pr-flow`](#bitbucket-pr-flow).
Installs no MCP server.

- **Provides:** skill `gitlab-merge-workflow`, command `/mr-create`; capability
  `vcs-pr-flow`.
- **Config:**

  | Key | Type | Required | Default | Description |
  |-----|------|----------|---------|-------------|
  | `base_branch` | string | no | `development` | Base branch the MR targets. |

- **Full README:** [`catalog/recipes/gitlab-mr-flow/README.md`](../catalog/recipes/gitlab-mr-flow/README.md)

```toml
[recipes.gitlab-mr-flow]
enabled = true
version = "1.1.0"

[recipes.gitlab-mr-flow.config]
base_branch = "development"

[[bindings]]
capability = "vcs-pr-flow"
recipe = "gitlab-mr-flow"
```

## bitbucket-pr-flow

**Bitbucket branch → PR → merge flow.** Uses the `bb` CLI. Sibling of
[`git-pr-flow`](#git-pr-flow) and [`gitlab-mr-flow`](#gitlab-mr-flow).
Installs no MCP server.

- **Provides:** skill `bitbucket-merge-workflow`, command `/bb-pr-create`; capability
  `vcs-pr-flow`.
- **Config:**

  | Key | Type | Required | Default | Description |
  |-----|------|----------|---------|-------------|
  | `base_branch` | string | no | `development` | Base branch the PR targets. |

- **Full README:** [`catalog/recipes/bitbucket-pr-flow/README.md`](../catalog/recipes/bitbucket-pr-flow/README.md)

```toml
[recipes.bitbucket-pr-flow]
enabled = true
version = "1.0.0"

[recipes.bitbucket-pr-flow.config]
base_branch = "development"

[[bindings]]
capability = "vcs-pr-flow"
recipe = "bitbucket-pr-flow"
```

## trello-mcp-workflow

**Concrete Trello provider for the `tracker` capability.** Board-scoped
integration: card linking, state sync, and progress comments, plus card
templates (feature, bug, spike, epic, handoff, decision). Enforces board
isolation — forbids cross-board tools and requires card validation against the
configured board. Ships an MCP preset for `@delorenj/mcp-server-trello` (needs
`TRELLO_API_KEY` / `TRELLO_TOKEN` in the environment) and a read-only
`ai-specs recipe init` brief to confirm setup before sync.

- **Provides:** skill `trello-mcp-workflow`, command `/trello-workflow`, 6 card
  templates, an MCP preset; capability `tracker` (+ `trello-session-bootstrap`,
  `trello-card-linking`, `trello-state-sync`, `trello-progress-comment`).
- **Config:**

  | Key | Type | Required | Default | Description |
  |-----|------|----------|---------|-------------|
  | `board_id` | string | **yes** | — | Trello board ID. Validated against `^[0-9a-fA-F]{24}$`. |
  | `default_list` | string | no | `In Progress` | List where new cards are created. |
  | `epic_list` | string | no | `Epic` | List where epic-type cards are placed. |

- **Board isolation** (declared in `recipe.toml`, not overridden per project):
  `forbidden_tools` = `trello_get_my_cards`, `trello_list_boards`;
  `restricted_tools` = `trello_set_active_board`;
  `card_validation_required` = `true` (cards must belong to the configured
  `board_id`).

- **Setup:** `ai-specs recipe add trello-mcp-workflow` then
  `ai-specs recipe init trello-mcp-workflow` (read-only brief), then set
  `board_id` and run `ai-specs sync`.
- **Full README:** [`catalog/recipes/trello-mcp-workflow/README.md`](../catalog/recipes/trello-mcp-workflow/README.md)

```toml
[recipes.trello-mcp-workflow]
enabled = true
version = "1.2.0"

[recipes.trello-mcp-workflow.config]
board_id = "69ec097f13e2d38ecd89a557"
```

## vault-canonical-store

**Concrete provider for the `canonical-store` capability.** Durable decisions
and handoffs in a configured vault MCP (e.g. Obsidian over a filesystem MCP
server). Defines when to read/write canonical context, plus decision- and
handoff-note shapes. The rule: Vault holds the deliberate, human-auditable
record; operational memory holds searchable continuity. Ships an MCP preset for
`@modelcontextprotocol/server-filesystem` (pinned `2025.7.1`) scoped via
`CANONICAL_VAULT_PATH` in the environment (typically set in `.envrc`).

- **Provides:** skill `vault-context`; capability `canonical-store`; MCP preset
  `vault-canonical`.
- **Config:**

  | Key | Type | Default | Description |
  |-----|------|---------|-------------|
  | `vault_scope` | string | — | Optional hint for the vault scope/path to stay within. |
  | `decisions_folder` | string | `decisiones` | Folder for decisions and conventions. |
  | `sessions_folder` | string | `sessions` | Folder for session summaries and handoffs. |

- **Full README:** [`catalog/recipes/vault-canonical-store/README.md`](../catalog/recipes/vault-canonical-store/README.md)

```toml
[recipes.vault-canonical-store]
enabled = true
version = "1.1.0"

[recipes.vault-canonical-store.config]
decisions_folder = "decisiones"
sessions_folder = "sessions"
```
