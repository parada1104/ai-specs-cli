# Recipe behavior evals (slow tier)

Runtime behavior verification for catalog recipes using headless agent
invocation. **Not** part of `./tests/run.sh` — uses `eval_*.py` naming so
`unittest discover -p 'test_*.py'` never loads this directory.

## When to run

- Nightly or pre-release (not per-PR): needs billed LLM calls, inherent flakiness.
- Local dry (no LLM): `./tests/evals/run.sh`
- Live is **per capability / client** — never mix modules in one run:
  - plan-build: `./tests/evals/run-live.sh`
  - vcs-pr-flow: `./tests/evals/run-live-vcs.sh`
  - vault-canonical-store: `./tests/evals/run-live-vault.sh`
  - worktree-flow: `./tests/evals/run-live-worktree.sh`

## Requirements

- A supported runtime on `PATH`: `claude`, `cursor-agent`, `opencode`, `pi`, or `omp`
- Select with `EVALS_RUNTIME` / `EVALS_RUNTIMES` (otherwise prefer order)
- Model routing (hard rule):
  - `claude` → Claude Code **subscription** via the `claude` CLI (`opus`)
  - `cursor-agent` → Cursor Agent **subscription** via `cursor-agent` / `agent`
    (`composer-2.5`). Skills land in `.cursor/skills`. Do **not** use the
    `cursor` IDE shim as the runtime binary.
  - `opencode` / `pi` / `omp` → **API for Cursor** only (`cursorapi/...`)
    — never `anthropic/*` and never an Anthropic API key on those runtimes
- Optional override:
  - OpenCode-family: `EVALS_MODEL=cursorapi/...` or `EVALS_MODEL_OPENCODE` /
    `EVALS_MODEL_PI` / `EVALS_MODEL_OMP`
  - Cursor Agent: `EVALS_MODEL=composer-2.5` or `EVALS_MODEL_CURSOR_AGENT`
    (hyphen → underscore). `cursorapi/*` is rejected here.
  - Claude: `EVALS_MODEL_CLAUDE` / bare Claude Code model ids
- Defaults: `opus` (claude) · `composer-2.5` (cursor-agent) ·
  `cursorapi/composer-2.5` (opencode/pi/omp)
- Optional: `EVALS_MAX_TURNS`, `EVALS_TRIALS` (default trials=1; use 3 for N-of-M)

## Layout

```
tests/evals/
  lib/              # harness + project fixtures
  scenarios/        # per-recipe scenario folders
  eval_*.py         # unittest modules (dry + live)
  run.sh            # dry discover (all eval_*.py)
  run-live.sh            # LIVE plan-build-flow only
  run-live-vcs.sh        # LIVE vcs-pr-flow siblings only
  run-live-worktree.sh   # LIVE worktree-flow only
```

## Scenario contract

- Prompts are **natural user requests** (no `/plan`, `/build`, or "haz un plan")
- `scenario.toml` may set `mode = "plan" | "build"`
- Plan-mode runs must not modify production paths listed in
  `forbidden_path_globs`
- Fixtures seed a tiny app and copy the recipe skill into the runtime discovery
  path (`.claude/skills`, `.cursor/skills`, `.opencode/skills`, `.pi/skills`, …)

## Clients

### `plan-build-flow`

Live: `./tests/evals/run-live.sh` → `eval_plan_build_flow_live.py`

| Scenario | Mode | Asserts |
|----------|------|---------|
| AC3 `ac3_plan_stops_before_apply` | plan | tasks + specs; no `src/` edits |
| AC4 `ac4_build_after_auth` | build | implements seeded plan (`signup.py`) |
| AC5 `ac5_archive_before_merge` | build | archives change folder; active gone |
| AC7 `ac7_light_gitignore_file_store` | build | writes `.gitignore` (file store) |

```bash
EVALS_RUNTIMES=opencode,claude EVALS_SCENARIOS=ac3_plan_stops_before_apply \
  ./tests/evals/run-live.sh
```

### `vcs-pr-flow` siblings (`git-pr-flow`, `gitlab-mr-flow`, `bitbucket-pr-flow`)

Live: `./tests/evals/run-live-vcs.sh` → `eval_vcs_pr_flow_live.py`  
Agents write `ai-specs/eval-notes/merge-plan.md` (no real remote merges).

| Scenario | git | gitlab | bitbucket | Asserts |
|----------|-----|--------|-----------|---------|
| `ac_protected_head_no_delete` | yes | yes | yes | classify protected/protegido + provider merge CLI; no delete-source |
| `ac_feature_head_cleanup` | yes | yes | yes | delete-source flag + worktree/local cleanup |
| `ac_release_head_preferred` | yes | yes | yes | recommends `release/v*` head |
| `ac_delete_branch_on_merge_warn` | yes | — | — | warns + documents `gh api` PATCH; no auto-apply |

Select with `recipe/scenario` tokens (or bare scenario id for all providers that
define it):

```bash
EVALS_RUNTIMES=claude,cursor-agent \
  EVALS_SCENARIOS=git-pr-flow/ac_protected_head_no_delete,git-pr-flow/ac_feature_head_cleanup \
  ./tests/evals/run-live-vcs.sh

# Cursor Agent subscription (composer)
EVALS_RUNTIMES=cursor-agent EVALS_MODEL=composer-2.5 ./tests/evals/run-live-vcs.sh

# alternate cursorapi model for OpenCode-family runtimes
EVALS_MODEL=cursorapi/grok-4.5 EVALS_RUNTIMES=opencode ./tests/evals/run-live-vcs.sh
```

### `vault-canonical-store`

Live: `./tests/evals/run-live-vault.sh` → `eval_vault_canonical_live.py`

**Guidance scenarios** write notes under `ai-specs/eval-notes/` (skill behavior).

**MCP path smoke (no sync / no release / no LLM):**

```bash
python3 tests/smoke_vault_mcp_fs.py
python3 tests/smoke_vault_mcp_fs.py --path "$CANONICAL_VAULT_PATH"
```

Asserts `vault-fs-mcp.sh` + `@modelcontextprotocol/server-filesystem@2025.7.1`
starts with `Allowed directories` equal to standalone `CANONICAL_VAULT_PATH`.

**Live MCP connect + scope** (`ac_mcp_live_scope`, claude + cursor-agent):
syncs/registers the vault filesystem MCP, places `MARKER.md` under an in-project
scoped dir with spaces, asks the agent to read it via MCP only, and asserts an
*outside* sibling secret does not leak. Requires `mcp__vault-canonical__*` tool
evidence unless `EVALS_MCP_REQUIRE_TOOL_EVIDENCE=0`.

Host notes (Claude Code 2.1.215, 2026-07):
- The recipe pin `@…/server-filesystem@2025.7.1` **tools-fetched empty schemas** until
  the wrapper started pinning `zod@3` alongside it. The package inherits zod from the
  SDK, which resolves to zod 4, and its `zod-to-json-schema@3` emits schemas without
  `type`/`properties` for zod 4 definitions. Fixed in the wrapper; see the recipe
  README section "Why `zod@3` is pinned".
- `2025.7.29+` connects but replaces argv dirs with MCP **roots**, and a host always
  advertises its cwd. That either denies the vault or widens the scope to cwd + vault,
  so the recipe stays on `2025.7.1`. `EVALS_VAULT_FS_MCP_PACKAGE` still overrides the
  package when a scenario needs to probe roots behavior explicitly.
- Wrapper + `2025.7.1` `AllowedDirectories` remain proven by
  `python3 tests/smoke_vault_mcp_fs.py` (no LLM).

| Scenario | Asserts |
|----------|---------|
| `ac_kepano_skills_present` | Obsidian-native decision draft (wikilinks / decision shape) |
| `ac_mcp_path_with_spaces` | Documents single-argv `CANONICAL_VAULT_PATH` for spaced iCloud paths |
| `ac_vault_context_guidance` | Decision note shape + Vault vs Engram split |
| `ac_mcp_live_scope` | Live MCP read of scoped marker; sibling secret stays out |

```bash
EVALS_RUNTIMES=claude,cursor-agent \
  EVALS_SCENARIOS=ac_vault_context_guidance ./tests/evals/run-live-vault.sh

# expensive MCP connect/scope proof
EVALS_RUNTIMES=claude,cursor-agent \
  EVALS_SCENARIOS=ac_mcp_live_scope ./tests/evals/run-live-vault.sh
```

### `worktree-flow`

Live: `./tests/evals/run-live-worktree.sh` → `eval_worktree_flow_live.py`  
Agents write plans under `ai-specs/eval-notes/` (no real `git worktree` execution).

| Scenario | Mode | Asserts |
|----------|------|---------|
| `ac_submodule_create_uses_subrepo_contract` | build | `git -C` + submodule path + `<subrepo>-<slug>` dest; no `.worktrees/` created |
| `ac_monorepo_apps_no_subrepo_needed` | build | plain repo-root `git worktree add`; no affirmative `-C` / submodule / `.gitmodules` |
| `ac_cleanup_scans_all_submodules` | build | scans all initialized submodules; root-only `worktree list` is not enough |
| `ac_gate_blocked_write_creates_worktree_not_bash_fallback` | build | create worktree (`/worktree-new` / `git worktree add`); no bash write fallback |

```bash
EVALS_RUNTIMES=claude,cursor-agent \
  EVALS_SCENARIOS=ac_submodule_create_uses_subrepo_contract \
  ./tests/evals/run-live-worktree.sh

# Cursor Agent subscription (composer)
EVALS_RUNTIMES=cursor-agent EVALS_MODEL=composer-2.5 ./tests/evals/run-live-worktree.sh
```

### `assisted-configure`

Live: `./tests/evals/run-live-assisted-configure.sh` →
`eval_assisted_configure_live.py`. This is an additive client on the existing
eval system; it is excluded from `./tests/run.sh` because it is named
`eval_*.py` and requires an opt-in runtime.

| Scenario | Recipe evidence | Asserts |
|----------|-----------------|---------|
| `ac_recommend_stops_before_apply` | worktree-flow | inspect/recommend and approval gate leave manifest bytes unchanged |
| `ac_topology_grounded_without_initmd` | worktree-flow + initialized submodule | topology evidence is cited without consumer-specific paths or `init.md` |
| `ac_apply_sync_verify_report` | trello-mcp-workflow | approved MCP config is applied, sync/doctor run, and report fields are surfaced |
| `ac_noop_reapply_preserves_bytes` | plan-build-flow | equivalent re-apply is a byte-identical no-op |
| `ac_blocked_cli_version_pin` | plan-build-flow + unsatisfied pin | preflight blocks before any write |

Run directly from a shell (one runtime or several):

```bash
EVALS_RUNTIMES=claude,cursor-agent \
  EVALS_SCENARIOS=ac_recommend_stops_before_apply \
  ./tests/evals/run-live-assisted-configure.sh
```

The optional Orca/OMP orchestration layer may fan out this same runner with
different `EVALS_RUNTIMES` values and aggregate provenance per runtime. It only
invokes and collects existing runner results: it does not alter scenarios,
prompts, fixtures, assertions, isolation, trials, pass criteria, or verdicts.
Without orchestration, the direct shell command above remains the canonical
execution path and has identical eval semantics. Runtime evidence records
scenario, runtime, model, trial, CLI version, worktree SHA, exit/timed-out, and
the helper report; it is evidence rather than a unit-suite merge gate.
