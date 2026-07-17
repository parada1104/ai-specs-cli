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

## Requirements

- A supported runtime on `PATH`: `claude`, `opencode`, `pi`, or `omp`
- Select with `EVALS_RUNTIME` / `EVALS_RUNTIMES` (otherwise prefer order)
- Model routing (hard rule):
  - `claude` → Claude Code **subscription** via the `claude` CLI (`opus`)
  - `opencode` / `pi` / `omp` → **API for Cursor** only (`cursorapi/...`)
    — never `anthropic/*` and never an Anthropic API key on those runtimes
- Optional override: `EVALS_MODEL=cursorapi/...` (OpenCode-family) or
  `EVALS_MODEL_OPENCODE` / `EVALS_MODEL_PI` / `EVALS_MODEL_OMP` / `EVALS_MODEL_CLAUDE`
- Defaults: `opus` (claude) · `cursorapi/composer-2.5` (opencode/pi/omp)
  Alternates: `cursorapi/composer-2.5-fast`, `cursorapi/grok-4.5`
- Optional: `EVALS_MAX_TURNS`, `EVALS_TRIALS` (default trials=1; use 3 for N-of-M)

## Layout

```
tests/evals/
  lib/              # harness + project fixtures
  scenarios/        # per-recipe scenario folders
  eval_*.py         # unittest modules (dry + live)
  run.sh            # dry discover (all eval_*.py)
  run-live.sh       # LIVE plan-build-flow only
  run-live-vcs.sh   # LIVE vcs-pr-flow siblings only
```

## Scenario contract

- Prompts are **natural user requests** (no `/plan`, `/build`, or "haz un plan")
- `scenario.toml` may set `mode = "plan" | "build"`
- Plan-mode runs must not modify production paths listed in
  `forbidden_path_globs`
- Fixtures seed a tiny app and copy the recipe skill into the runtime discovery
  path (`.claude/skills`, `.opencode/skills`, `.pi/skills`, …)

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
EVALS_RUNTIMES=opencode,pi,omp,claude \
  EVALS_SCENARIOS=git-pr-flow/ac_protected_head_no_delete,git-pr-flow/ac_feature_head_cleanup \
  ./tests/evals/run-live-vcs.sh

# alternate cursorapi model for non-claude runtimes
EVALS_MODEL=cursorapi/grok-4.5 EVALS_RUNTIMES=opencode ./tests/evals/run-live-vcs.sh
```
