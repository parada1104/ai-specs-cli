# Recipe catalog

The recipes `ai-specs` ships in its catalog, what each one is for, and the
config it expects in `ai-specs/ai-specs.toml`. Enable a recipe with
`[recipes.<id>] enabled = true`, set any config under
`[recipes.<id>.config]`, then run `ai-specs sync` to materialize its skills,
commands, templates, and docs. Catalog `version` in `recipe list` is
informational only — no per-recipe pin is required.

- For the **schema** of `recipe.toml` and `[config]` fields, see
  [`docs/recipe-schema.md`](recipe-schema.md).
- For the **manifest** side (`[recipes.<id>]`, `[recipes.<id>.config]`,
  `[[bindings]]`), see [`docs/ai-specs-toml.md`](ai-specs-toml.md).
- For the **capability model** (how recipes compose), see
  [`docs/capabilities.md`](capabilities.md).

Run `ai-specs recipe list` to see which catalog recipes are installed vs.
available in a project.

## Agent-assisted configuration

For a non-interactive, reviewable setup flow, an agent can use the additive
helper `ai-specs recipe configure <id> [path]`:

1. Inspect with `--inspect --json` and ground the recommendation in the schema,
   existing config, repository topology, MCP state, and CLI dependencies.
2. Show proposed keys, preserved keys, assumptions, and planned verification;
   wait for explicit user approval before applying.
3. Apply approved values with repeatable `--set KEY=VALUE`; add `--sync` to run
   synchronization and doctor verification. `--dry-run` never writes.
4. Report `status`, changed/unchanged/preserved keys, preflight, sync, verify,
   assumptions, drift, and version gaps. Exit 3 rejects input; exit 4 blocks a
   CLI-version preflight before any write. Partial sync failures are not
   reported as complete.

The helper preserves unmentioned manifest keys, comments, and project override
files. It never accepts secret-shaped literals; use `${env:VAR}` references or
redaction. The interactive `ai-specs configure-recipes` wizard remains
unchanged, and `ai-specs recipe init` remains read-only and does not sync.

The broader evidence MVP covers `worktree-flow` topology grounding without an
`init.md`, `trello-mcp-workflow` MCP/secrets/init guidance, and
`plan-build-flow` plain config. Runtime evidence lives in the existing
`tests/evals/` system as an additive client; deterministic helper tests remain
the merge gate. An optional Orca/OMP orchestration skill may invoke the
existing live runners across runtimes and aggregate per-runtime results, but it
is not a runtime, runner, scoring service, or prerequisite. Running the shell
runner directly produces the same scenario semantics and verdicts.

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
| [`playwright-ui-flow`](#playwright-ui-flow) | Specific | Playwright UI test/smoke discipline + CLI surface | `ui-browser-testing` | — | `ui_test_command`, `ui_smoke_command`, `playwright_config` |
| [`playwright-mcp`](#playwright-mcp) | Specific | Exploratory browser automation via `@playwright/mcp` (add-on) | — (augments base) | `playwright` | — (override via `[mcp.playwright]`) |
| [`plan-build-flow`](#plan-build-flow) | Foundational | Ambient skill-only plan/build workflow (no slash commands) | `plan-build-flow` | — | `artifact_store_default` |
| [`worktree-flow`](#worktree-flow) | Foundational | Isolated `.worktrees/` + safe post-merge cleanup (standalone / monorepo-apps / monorepo-submodules) | `worktree-isolation`, `worktree-cleanup` | — | `worktrees_dir`, `integration_branch`, `auto_remove_merged`, `repo_topology`, `gate_mode`, `gate_scope`, `WORKTREE_GATE_PROTECTED` |
| [`git-pr-flow`](#git-pr-flow) | Specific | Branch → PR → approval-gated merge (GitHub) | `vcs-pr-flow` | — | `base_branch`, `expected_owner`, `auto_switch_account` |
| [`gitlab-mr-flow`](#gitlab-mr-flow) | Specific | Branch → MR → approval-gated merge (GitLab) | `vcs-pr-flow` | — | `base_branch`, `expected_owner` |
| [`bitbucket-pr-flow`](#bitbucket-pr-flow) | Specific | Branch → PR → approval-gated merge (Bitbucket) | `vcs-pr-flow` | — | `base_branch`, `expected_owner` |
| [`trello-mcp-workflow`](#trello-mcp-workflow) | Specific | Trello board integration (tracker) + card-per-change gate | `tracker` (+ 4 trello-* capabilities) | `trello` | **`board_id` (required)**, `default_list`, `epic_list`, `gate_mode` |
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
| `playwright-ui-flow` | `npx` | Playwright UI test/smoke commands | yes |
| `playwright-mcp` | `npx` | `@playwright/mcp` server runtime | yes |


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

## playwright-ui-flow

**Playwright UI test/smoke discipline + CLI surface.** Canonical
`ui-browser-testing` capability: when to run suites/smokes, CLI-first
precedence, and evidence before merge. Complements `tdd-flow` (does not
replace it). Installs no MCP server — add [`playwright-mcp`](#playwright-mcp)
for exploratory automation.

- **Provides:** skills `ui-browser-testing`, `playwright-cli`, command
  `/ui-smoke`; capability `ui-browser-testing`.
- **Config:**

  | Key | Type | Required | Default | Description |
  |-----|------|----------|---------|-------------|
  | `ui_test_command` | string | no | _(unset)_ | Full UI suite command (e.g. `npx playwright test`). |
  | `ui_smoke_command` | string | no | _(unset)_ | Fast smoke subset (e.g. `npx playwright test --grep @smoke`). |
  | `playwright_config` | string | no | _(unset)_ | Path to `playwright.config.*` when non-standard. |

- **Full README:** [`catalog/recipes/playwright-ui-flow/README.md`](../catalog/recipes/playwright-ui-flow/README.md)

```toml
[recipes.playwright-ui-flow]
enabled = true

[recipes.playwright-ui-flow.config]
ui_test_command = "npx playwright test"
ui_smoke_command = "npx playwright test --grep @smoke"
```

## playwright-mcp

**Exploratory browser automation via `@playwright/mcp`.** Add-on to
[`playwright-ui-flow`](#playwright-ui-flow): hybrid = enable both. Declares no
capability of its own (avoids binding ambiguity). Ships MCP preset `playwright`.

- **Provides:** skill `playwright-mcp`, MCP preset `playwright`.
- **Config:** none in v1 — tune browser/headless via `[mcp.playwright]` in the
  project manifest.
- **Full README:** [`catalog/recipes/playwright-mcp/README.md`](../catalog/recipes/playwright-mcp/README.md)

```toml
[recipes.playwright-ui-flow]
enabled = true

[recipes.playwright-mcp]
enabled = true
```

## plan-build-flow

**Ambient skill-only change workflow.** The bundled skill auto-invokes on
substantial requests to produce reviewable planning artifacts, waits for human
authorization, then implements, validates, and closes the change — without
`/plan` or `/build` commands. Implementation defers to an isolated-worktree
workflow when one is enabled, without hard-depending on it.

It is the sole ceremony/depth classification source (`Light` / `Standard` /
`Full`), replacing the retired ceremony contract.

OpenSpec archive-tail uses the canonical dated destination
`openspec/changes/archive/YYYY-MM-DD-<slug>/` with a valid ISO calendar date.
The exact undated `archive/<slug>/` form remains a legacy fallback only when no
dated candidate exists. The pre-merge guardian inspects only direct children,
rejects invalid or near-match names, and fails closed for multiple dated or
dated-plus-undated candidates.

- **Provides:** skill `plan-build-flow`; capability `plan-build-flow`.
- **Config:**

  | Key | Type | Required | Default | Accepted values | Description |
  |-----|------|----------|---------|-----------------|-------------|
  | `artifact_store_default` | string | no | `openspec` | `openspec`, `engram`, `both` | External-session persistence preference for planning artifacts; may be overridden in the project manifest and is materialized into the brief during sync. Plan-build readiness is always proven by file-backed artifacts, never by the store selection. |

  The generated rule is repository-declared guidance. An external session runtime may
  consume it when asked where planning artifacts should live, but runtime session behavior
  is outside this recipe. Plan-build readiness is always proven by file-backed artifacts
  under the canonical `openspec/changes/<slug>/` tree — `tasks.md`, tier minimum planning
  files, and `verify-report.md` — never by the store selection; Engram MAY mirror artifacts
  but never replaces them.

- **Full README:** [`catalog/recipes/plan-build-flow/README.md`](../catalog/recipes/plan-build-flow/README.md)

```toml
[recipes.plan-build-flow]
enabled = true
version = "1.6.0"

[recipes.plan-build-flow.config]
artifact_store_default = "both"
```

### Cross-repository planning boundaries

For a recognized initialized submodule linked worktree, repository **topology**
derives the containing **superproject** as the **central** planning root. The
central `openspec/changes/` tree is the single planning-artifact location for
that cross-repository change: active plans are looked up there and writes remain
limited to that subtree, never to arbitrary superproject production paths.
**Standalone** and non-submodule worktrees retain nearest-root behavior. An
unresolved relationship is **fail-safe** and does not guess an unrelated parent.
There is **no duplication** and **no orchestration**: the recipe does not copy,
synchronize, or create duplicate repository-local plans, worktrees, branches, or
pull requests.

## worktree-flow

**Isolated git worktrees under `.worktrees/` with safe post-merge cleanup.**
File-writing change work runs in a dedicated worktree; pure exploration stays
outside one. The bundled cleanup script removes only merged + clean worktrees
(detecting regular, squash, and rebase merges by patch-id), preserves dirty and
unmerged ones, and never touches the main worktree.

- **Provides:** skill `worktree-flow`, commands `/worktree-new`,
  `/worktree-clean`, script `bin/worktree-cleanup.sh`, and a `worktree-gate`
  runtime hook (`[[provides.hooks]]`) that blocks writes to the main worktree on
  protected branches, supports `gate_mode` dispatch (`always` / `ask` / `off`),
  and applies the proven-topology `gate_scope` policy — see
  [`docs/runtime-hooks.md`](runtime-hooks.md); capabilities
  `worktree-isolation`, `worktree-cleanup`.
- **Gate implementation:** the gate ships as a single zero-dependency Go
  binary (implementation of record) plus a frozen Bash reference
  (`worktree-gate-legacy.sh`) kept for one minor release as the rollback path.
  `gate_impl` selects the implementation: `auto` (default — prefer the Go
  binary, fall back to Bash), `go` (binary only; fails open when unusable, with
  a `worktree-gate` doctor ERROR), or `bash` (frozen Bash reference; no binary,
  network, or Go toolchain required). `ai-specs sync` materializes a thin
  bash-3.2 launcher at the unchanged hook path, acquires the binary into
  `$AI_SPECS_HOME/cache/bin/worktree-gate/<cli-version>/<goos>-<goarch>/`,
  verifies SHA-256 against the committed `SHA256SUMS` trust root before install,
  and degrades with a warning on any failure — acquisition never fails sync.
  `ai-specs doctor` reports the resolved implementation, version, digest state,
  and silent fallbacks.
- **Gate provenance:** sync records a baseline of the exact bytes the CLI last
  rendered for the generated gate hook. A baseline match means unmodified and
  may be force-updated; a byte mismatch or missing baseline is preserved with a
  warning (no seeding). An explicit refresh (`ai-specs sync --refresh-gates`)
  saves the exact pre-refresh bytes to a cache-only immutable backup before
  replacing a customized gate; `ai-specs doctor` warns on customized gates and
  stays quiet on matching baselines.
- **Topologies:** `standalone`, `monorepo-apps` (naming-only), and
  `monorepo-submodules` (per-submodule `git -C` create + cleanup enumeration
  under a shared superproject `worktrees_dir`).
- **Config:**

  | Key | Type | Default | Description |
  |-----|------|---------|-------------|
  | `worktrees_dir` | string | `.worktrees` | Directory holding per-change worktrees. |
  | `integration_branch` | string | `main` | Branch worktrees are created from and merged into. |
  | `auto_remove_merged` | boolean | `true` | Whether merged worktrees are eligible for cleanup. |
  | `gate_mode` | string | `always` | Main-worktree gate mode. `always` keeps the current block, `ask` blocks with a bypass hint, and `off` disables the gate. |
  | `gate_scope` | string | `auto` | Scope policy: `auto` / `superrepo` / `subrepo`; only proven canonical `<superrepo>/openspec/changes/**` planning paths are excepted. |
  | `gate_impl` | string | `auto` | Gate implementation: `auto` / `go` / `bash` (see "Gate implementation" above). |
  | `WORKTREE_GATE_SCOPE` | string | — | Optional invocation override; invalid values warn and fall back to the stamped scope. |
  | `repo_topology` | string | `auto` | `auto` / `standalone` / `monorepo-apps` / `monorepo-submodules`. Auto detects initialized submodules; never auto-selects `monorepo-apps`. Shared `<worktrees_dir>/<subrepo>-<slug>` layout under submodules; cleanup enumerates per-module. |
  | `WORKTREE_GATE_PROTECTED` | string | `main development` | Space-separated branch names where the `worktree-gate` hook blocks Edit/Write in the main worktree. Passed to the rendered hook as the `WORKTREE_GATE_PROTECTED` env var. |

- **Full README:** [`catalog/recipes/worktree-flow/README.md`](../catalog/recipes/worktree-flow/README.md)

```toml
[recipes.worktree-flow]
enabled = true
version = "1.5.0"

[recipes.worktree-flow.config]
integration_branch = "development"
gate_mode = "always"
gate_scope = "auto"
repo_topology = "auto"
```

## git-pr-flow

**GitHub branch → PR → merge flow.** Pushes the branch, opens a PR against the
configured base, and merges *only* after explicit user approval. Uses the `gh` CLI.
Sibling recipes cover GitLab and Bitbucket; select the host through `[[bindings]]`.
Long-lived heads (`main` / `development` / `staging` / configured base) are not
deleted after merge; prefer `release/vX.Y.Z` into `main`. The skill warns when
repo-wide `delete_branch_on_merge` is enabled.

- **Provides:** skill `git-merge-workflow`, command `/pr-create`; capability
  `vcs-pr-flow`.
- **Config:**

  | Key | Type | Required | Default | Description |
  |-----|------|----------|---------|-------------|
  | `base_branch` | string | no | `main` | Base branch the PR targets. |
  | `expected_owner` | string | no | `""` | Account username this repo expects; activates auth preflight when set. |
  | `auto_switch_account` | boolean | no | `false` | gh only: auto-switch CLI account on mismatch (requires gh ≥ 2.50.0). |

- **Full README:** [`catalog/recipes/git-pr-flow/README.md`](../catalog/recipes/git-pr-flow/README.md)

```toml
[recipes.git-pr-flow]
enabled = true
version = "1.3.0"

[recipes.git-pr-flow.config]
base_branch = "development"
expected_owner = ""
auto_switch_account = false

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
  | `expected_owner` | string | no | `""` | Account username this repo expects; activates auth preflight when set. |
  | `auto_switch_account` | boolean | no | `false` | Reserved for API parity; glab has no auth switch — mismatch blocks with guidance. |

- **Full README:** [`catalog/recipes/gitlab-mr-flow/README.md`](../catalog/recipes/gitlab-mr-flow/README.md)

```toml
[recipes.gitlab-mr-flow]
enabled = true
version = "1.2.0"

[recipes.gitlab-mr-flow.config]
base_branch = "development"
expected_owner = ""
auto_switch_account = false

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
  | `expected_owner` | string | no | `""` | Account username this repo expects; activates auth preflight when set. |
  | `auto_switch_account` | boolean | no | `false` | Reserved for API parity; bb has no auth switch — mismatch blocks with guidance. |

- **Auth note:** Bitbucket CLI verifies authentication with `bb auth status`, the same subcommand as
  `gh` and `glab`. There is no `bb auth show` (bb 1.23.2 answers `unknown command 'show'`).

- **Full README:** [`catalog/recipes/bitbucket-pr-flow/README.md`](../catalog/recipes/bitbucket-pr-flow/README.md)

```toml
[recipes.bitbucket-pr-flow]
enabled = true
version = "1.1.0"

[recipes.bitbucket-pr-flow.config]
base_branch = "development"
expected_owner = ""
auto_switch_account = false

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
`TRELLO_API_KEY` / `TRELLO_TOKEN` in the environment), a read-only
`ai-specs recipe init` brief to confirm setup before sync, and a phased
`tracker-card-gate` (`gate_mode` = `off|warn|always`, default `warn`) that
requires a `## Tracker` link section before production/PR-archive work.

- **Provides:** skill `trello-mcp-workflow`, command `/trello-workflow`, 6 card
  templates, an MCP preset, dual runtime hooks (`tracker-card-gate` +
  `tracker-card-gate-shell`); capability `tracker` (+ `trello-session-bootstrap`,
  `trello-card-linking`, `trello-state-sync`, `trello-progress-comment`).
- **Config:**

  | Key | Type | Required | Default | Description |
  |-----|------|----------|---------|-------------|
  | `board_id` | string | **yes** | — | Trello board ID. Validated against `^[0-9a-fA-F]{24}$`. |
  | `default_list` | string | no | `In Progress` | List where new cards are created. |
  | `epic_list` | string | no | `Epic` | List where epic-type cards are placed. |
  | `gate_mode` | string | no | `warn` | Tracker card gate: `off` / `warn` / `always`. |

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
version = "1.3.0"

[recipes.trello-mcp-workflow.config]
board_id = "69ec097f13e2d38ecd89a557"
gate_mode = "warn"
```

## vault-canonical-store

**Concrete provider for the `canonical-store` capability.** Durable decisions
and handoffs in a configured vault MCP (e.g. Obsidian over a filesystem MCP
server). Defines when to read/write canonical context, plus decision- and
handoff-note shapes. The rule: Vault holds the deliberate, human-auditable
record; operational memory holds searchable continuity. Ships an MCP preset for
`@modelcontextprotocol/server-filesystem` (pinned `2025.7.1`) via
`vault-fs-mcp.sh`, which reads absolute `CANONICAL_VAULT_PATH` from the
environment at exec time (not a bare `${VAR}` MCP arg). Also vendors the
kepano Obsidian skills (`obsidian-markdown`, `obsidian-bases`, `json-canvas`,
`obsidian-cli`, `defuddle`) for LLM wiki / second-brain note formats. Set the
path in `.envrc` (shell expansion); quote when the Obsidian path contains
spaces (e.g. iCloud `Mobile Documents/`).

- **Provides:** skill `vault-context`; kepano dep skills above; capability
  `canonical-store`; MCP preset `vault-canonical`.
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
version = "1.2.0"

[recipes.vault-canonical-store.config]
decisions_folder = "decisiones"
sessions_folder = "sessions"
```
