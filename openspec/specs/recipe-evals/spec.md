# Recipe Evals Specification

## Purpose

Repo-internal slow-tier harness that verifies **runtime behavior** of catalog
recipes by running headless agent invocations against scenario fixtures and
asserting filesystem/git outcomes. Complements deterministic materialization
tests in `tests/test_*_recipe.py`.

## Requirements

### Requirement: Separate discovery tier

Eval modules SHALL live under `tests/evals/` with filenames matching `eval_*.py`
so `./tests/run.sh` (`test_*.py` glob) never loads them. A dedicated
`tests/evals/run.sh` SHALL discover only `eval_*.py`.

#### Scenario: Default unit suite excludes evals

- GIVEN `./tests/run.sh` is executed
- WHEN unittest discovery runs
- THEN no module under `tests/evals/` is imported

### Requirement: Opt-in live execution

Live evals SHALL require explicit `EVALS_LIVE=1` plus a supported runtime on
PATH (`claude`, `opencode`, `pi`, `omp`). Runtime SHALL be selected via
`EVALS_RUNTIME` or auto-detect. Without the live gate, live modules MUST skip
rather than fail the dry smoke tier.

Default models (overridable with `EVALS_MODEL`):

- `claude` → `opus`
- `opencode` / `pi` / `omp` → `opencode-go/glm-5.2`

#### Scenario: Dry smoke passes offline

- GIVEN `EVALS_LIVE` is unset
- WHEN `tests/evals/run.sh` runs
- THEN harness smoke tests pass without network or billed LLM calls

### Requirement: Scenario fixture format

Each scenario SHALL provide `scenario.toml` (metadata + assertion globs +
optional `mode`) and a prompt file. The harness SHALL materialize a minimal
`ai-specs` project, seed an application fixture when needed, copy the recipe
skill into the runtime discovery path, invoke the selected runtime (plan mode
preferred for planning scenarios), then assert forbidden paths unchanged and
required artifacts present.

Prompts MUST be natural user requests and MUST NOT coach `/plan`, `/build`, or
equivalent meta-ceremony verbs.

#### Scenario: AC3 ambient plan stops before apply

- GIVEN the `ac3_plan_stops_before_apply` scenario with a natural implement
  request
- WHEN a live plan-mode run completes successfully
- THEN `openspec/changes/*/tasks.md` exists
- AND at least one `openspec/changes/*/specs/**/*.md` exists
- AND paths under `src/`, `lib/`, `catalog/` were not modified

### Requirement: Runtime skill discovery setup

The fixture helper SHALL install the materialized recipe skill into the
runtime-specific skills directory so ambient `auto_invoke` can fire.

### Requirement: N-of-M pass criteria

Live scenarios SHALL support `EVALS_TRIALS` with N-of-M thresholds to absorb
LLM non-determinism (default single trial for local dev; 3-of-3 recommended
pre-release).

### Requirement: First client scope

v2 targets `plan-build-flow` ambient AC3+ with multi-runtime support.
LLM-as-judge (AC6) and catalog-packaged eval recipe remain out of scope.

### Requirement: Runtime hooks wired for live scenarios

The live-eval harness SHALL wire a recipe's `[[provides.hooks]]` runtime hooks
into the fixture's native runtime channel before invoking the agent, so
scenarios exercise runtime hooks end-to-end (not only the advisory skill/brief
layer). Wiring SHALL reuse `hooks-render.py` (the same renderer `sync-agent.sh`
uses), fed by the resolved-hooks JSON from `recipe-materialize`, mapping the
eval runtime id to the platform agent id (`cursor-agent → cursor`; others
identity).

#### Scenario: Hook wired for a hook-capable runtime

- GIVEN a recipe declaring a `pre-tool-use` blocking hook
- AND the fixture is materialized for the `claude` runtime
- WHEN the harness wires runtime hooks
- THEN the fixture `.claude/settings.json` MUST contain a `PreToolUse` entry
  invoking that hook's materialized script

#### Scenario: No file-write hook for cursor

- GIVEN the same recipe
- AND the fixture is prepared for the `cursor-agent` runtime
- WHEN the harness wires runtime hooks
- THEN no file-write (`Edit|Write|MultiEdit|NotebookEdit`) hook is wired for
  cursor (the runtime exposes no pre-file-write event)

### Requirement: Hook-dependent scenario scoping

A scenario MAY declare `requires_hook = true`. The runner SHALL skip such a
scenario on any runtime that cannot receive a file-write hook (currently
`cursor-agent`), so a gate scenario asserts only where the gate can actually be
enforced.

#### Scenario: Gate scenario skipped on cursor-agent

- GIVEN a scenario with `requires_hook = true`
- AND the selected runtime is `cursor-agent`
- WHEN the runner evaluates that scenario/runtime pair
- THEN it MUST be skipped (not counted as pass or fail)

#### Scenario: Gate scenario runs on claude

- GIVEN a scenario with `requires_hook = true`
- AND the selected runtime is `claude`
- WHEN the runner evaluates that scenario/runtime pair
- THEN it MUST run with the hook wired and assert the scenario's globs

## Acceptance Criteria (test map)

| AC | Eval module / scenario | Req |
|----|------------------------|-----|
| AC1 | `eval_harness_smoke.test_scenario_fixture_loads` | fixture format |
| AC2 | `eval_harness_smoke.test_materialize_plan_build_flow_fixture` | materialize |
| AC3 | `eval_plan_build_flow_live` + `ac3_plan_stops_before_apply` | live ambient plan |
| AC4–AC5 | stub scenario dirs | build / archive follow-ups |
| AC6 | deferred | transcript judge |
| AC7 | reserved | future |
| AC8 | `test_eval_hook_wiring` (wiring); `ac8_approval_verb_without_folder` (`requires_hook`) | runtime hooks wired + scoped |
