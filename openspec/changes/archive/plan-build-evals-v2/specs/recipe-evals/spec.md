# Spec delta: recipe-evals (v2)

## MODIFIED Requirements

### Requirement: Opt-in live execution

Live evals SHALL require explicit `EVALS_LIVE=1` plus a supported runtime on
PATH. Supported runtimes: `claude`, `opencode`, `pi`, `omp`. Runtime SHALL be
selected via `EVALS_RUNTIME` or auto-detect. Without the live gate, live
modules MUST skip rather than fail the dry smoke tier.

Default models (overridable with `EVALS_MODEL`):

- `claude` → `opus`
- `opencode` / `pi` / `omp` → `opencode-go/glm-5.2` (alternate
  `opencode-go/deepseek-v4-flash` via env)

#### Scenario: Dry smoke passes offline

- GIVEN `EVALS_LIVE` is unset
- WHEN `tests/evals/run.sh` runs
- THEN harness smoke tests pass without network or billed LLM calls

#### Scenario: Live skips without runtime

- GIVEN `EVALS_LIVE=1` and no supported runtime on PATH
- WHEN live eval modules load
- THEN they skip rather than error

### Requirement: Scenario fixture format

Each scenario SHALL provide `scenario.toml` and a prompt file. The harness
SHALL materialize a minimal `ai-specs` project, seed a small application
fixture when required, copy the recipe skill into the runtime discovery path,
invoke the selected runtime in **plan mode** (or build mode when the scenario
declares it), then assert forbidden paths unchanged and required artifacts
present.

Prompts MUST be natural user requests. They MUST NOT instruct the agent to run
`/plan`, `/build`, or equivalent meta-ceremony verbs.

#### Scenario: AC3 ambient plan stops before apply

- GIVEN the `ac3_plan_stops_before_apply` scenario with a natural
  "necesito implementar…" prompt
- AND the runtime is started in plan mode
- WHEN a live run completes successfully
- THEN `openspec/changes/*/tasks.md` exists
- AND for Standard-tier scenarios, at least one spec delta under
  `openspec/changes/*/specs/` exists
- AND paths under `src/`, `lib/`, `catalog/` were not modified
- AND the prompt file does not contain `/plan` or "haz un plan"

### Requirement: First client scope

v2 SHALL target `plan-build-flow` ambient AC3+ with multi-runtime support for
`claude`, `opencode`, `pi`, and `omp`. Catalog-packaged eval recipe remains
out of scope. LLM-as-judge (AC6) remains deferred.

## ADDED Requirements

### Requirement: Runtime skill discovery setup

The fixture helper SHALL install the materialized recipe skill into the
runtime-specific skills directory so ambient `auto_invoke` can fire.

#### Scenario: Claude discovers plan-build-flow skill

- GIVEN a materialized fixture for runtime `claude`
- WHEN `setup_runtime_skills` completes
- THEN `.claude/skills/plan-build-flow/SKILL.md` exists

#### Scenario: OpenCode discovers plan-build-flow skill

- GIVEN a materialized fixture for runtime `opencode`
- WHEN `setup_runtime_skills` completes
- THEN `.opencode/skills/plan-build-flow/SKILL.md` exists
