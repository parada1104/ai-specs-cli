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

## Acceptance Criteria (test map)

| AC | Eval module / scenario | Req |
|----|------------------------|-----|
| AC1 | `eval_harness_smoke.test_scenario_fixture_loads` | fixture format |
| AC2 | `eval_harness_smoke.test_materialize_plan_build_flow_fixture` | materialize |
| AC3 | `eval_plan_build_flow_live` + `ac3_plan_stops_before_apply` | live ambient plan |
| AC4–AC5 | stub scenario dirs | build / archive follow-ups |
| AC6 | deferred | transcript judge |
| AC7 | reserved | future |
