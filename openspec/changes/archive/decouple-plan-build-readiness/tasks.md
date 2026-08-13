# Tasks: Decouple plan-build readiness

Depth: full
Explore: skipped — affected paths and the single two-layer approach are documented.

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 250–350 authored lines; below the 800-line review budget |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | Single PR |
| Delivery strategy | ask-on-risk |
| Chain strategy | pending |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: pending
400-line budget risk: Low

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | Wording plus store-invariance coverage | PR 1 | `./tests/run.sh` | `EVALS_RUNTIMES=opencode EVALS_SCENARIOS=ac_delivery_contract_artifact_store ./tests/evals/run-live.sh` (runtime availability required) | Revert listed docs/tests/eval changes; gate and guardian stay untouched |

## Phase 1: RED — Contract Tests

- [x] 1.1 Extend `tests/test_plan_build_flow_recipe.py` with failing assertions for persistence-preference wording, file-backed readiness, one placeholder, and vocabulary exemptions; run `./tests/run.sh` and record the expected RED.
- [x] 1.2 Add store-context cases to `tests/test_plan_build_gate_hook.py` and `tests/test_premerge_guardian.py` for `openspec|engram|both`, including memory-only tier/verify blockers; run `./tests/run.sh` and record RED/characterization results.
- [x] 1.3 Update `tests/evals/scenarios/plan-build-flow/ac_delivery_contract_artifact_store/scenario.toml` to require both store preference and readiness-invariant language; prove the scenario contract with `./tests/evals/run.sh`.

## Phase 2: GREEN — Contract Implementation

- [x] 2.1 Update `catalog/recipes/plan-build-flow/recipe.toml`, its `skills/plan-build-flow/SKILL.md`, `README.md`, and `docs/recipes-catalog.md` so store is persistence-only and readiness is canonical file-backed; keep version, enum, placeholder, and forbidden-vocabulary rules; run `./tests/run.sh`.
- [x] 2.2 Complete the parametrized invariance tests in `tests/test_plan_build_gate_hook.py` and `tests/test_premerge_guardian.py`; do not modify `catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh` or `lib/_internal/premerge_guardian.py`; run `./tests/run.sh` with GREEN evidence.

## Phase 3: REFACTOR — Verification

- [x] 3.1 Align wording and assertions across `openspec/changes/decouple-plan-build-readiness/specs/plan-build-flow/spec.md`, recipe docs, and eval fixtures; run `./tests/run.sh` and inspect `git diff --check`.
- [x] 3.2 Run `./tests/validate.sh`; verify the hook and guardian files are byte-unchanged and all seven delta scenarios are represented by tests or the eval fixture.

## Phase 4: Archive Handoff

- [x] 4.1 During archive, merge the delta into `openspec/specs/plan-build-flow/spec.md`; prove parity with `./tests/validate.sh` before the pre-merge guardian.
