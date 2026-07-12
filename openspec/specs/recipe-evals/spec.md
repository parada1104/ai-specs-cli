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

Live evals SHALL require explicit `EVALS_LIVE=1` plus `claude` on PATH and an
API key. Without that gate, live modules MUST skip rather than fail the default
dry smoke tier.

#### Scenario: Dry smoke passes offline

- GIVEN `EVALS_LIVE` is unset
- WHEN `tests/evals/run.sh` runs
- THEN harness smoke tests pass without network or billed LLM calls

### Requirement: Scenario fixture format

Each scenario SHALL provide `scenario.toml` (metadata + assertion globs) and a
prompt file. The harness SHALL materialize a minimal `ai-specs` project, invoke
`claude -p` when live, then assert forbidden paths unchanged and required
artifacts present.

#### Scenario: AC3 plan stops before apply

- GIVEN the `ac3_plan_stops_before_apply` scenario
- WHEN a live run completes successfully
- THEN `openspec/changes/*/tasks.md` exists
- AND paths under `src/`, `lib/`, `catalog/` were not modified

### Requirement: N-of-M pass criteria

Live scenarios SHALL support `EVALS_TRIALS` with N-of-M thresholds to absorb
LLM non-determinism (default single trial for local dev; 3-of-3 recommended
pre-release).

### Requirement: First client scope

v1 SHALL target `plan-build-flow` AC3–AC7 behavioral gap. Multi-runtime matrix
and catalog-packaged eval recipe are out of scope.

## Acceptance Criteria (test map)

| AC | Eval module / scenario | Req |
|----|------------------------|-----|
| AC1 | `eval_harness_smoke.test_scenario_fixture_loads` | fixture format |
| AC2 | `eval_harness_smoke.test_materialize_plan_build_flow_fixture` | materialize |
| AC3 | `eval_plan_build_flow_live` + `ac3_plan_stops_before_apply` | live gate + AC3 |
| AC4–AC7 | reserved scenario dirs (future prompts) | first client scope |
