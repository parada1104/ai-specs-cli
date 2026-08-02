# Apply Progress: plan-build-delivery-contracts

## Structured status consumed

```yaml
schemaName: spec-driven
changeName: 2026-08-02-plan-build-delivery-contracts
artifactStore: openspec
planningHome:
  root: /Users/robert/proyectos/nnodes/ai-specs-cli-plan-build-delivery-contracts/openspec
  changesDir: openspec/changes
changeRoot: openspec/changes/2026-08-02-plan-build-delivery-contracts
artifactPaths:
  proposal: [openspec/changes/2026-08-02-plan-build-delivery-contracts/proposal.md]
  specs: [openspec/changes/2026-08-02-plan-build-delivery-contracts/specs/plan-build-flow/spec.md]
  design: [openspec/changes/2026-08-02-plan-build-delivery-contracts/design.md]
  tasks: [openspec/changes/2026-08-02-plan-build-delivery-contracts/tasks.md]
  applyProgress: [openspec/changes/2026-08-02-plan-build-delivery-contracts/apply-progress.md]
  verifyReport: []
  syncReport: []
artifacts:
  proposal: done
  specs: done
  design: done
  tasks: done
  applyProgress: done
  verifyReport: missing
  syncReport: missing
taskProgress:
  total: 22
  complete: 22
  remaining: 0
  unchecked: []
applyState: all_done
dependencies:
  apply: all_done
  verify: ready
  sync: blocked
  archive: blocked
actionContext:
  mode: repo-local
  workspaceRoot: /Users/robert/proyectos/nnodes/ai-specs-cli-plan-build-delivery-contracts
  allowedEditRoots: [/Users/robert/proyectos/nnodes/ai-specs-cli-plan-build-delivery-contracts]
  warnings: []
nextRecommended: Verify the change, then sync/archive through the parent workflow.
```

## Completed tasks

- P0.1/P0.2: locked scope confirmed and focused unittest commands established; no runtime or gate source changes.
- P1.1–P1.8: RED tests committed in `43503cd`; exact one-field schema, narrow vocabulary excisions, merge/enum behavior, renderer substitution, negative surface, materialization, catalog key, baseline preservation, and hermetic/live eval coverage added.
- P2.1–P2.3: GREEN recipe/docs implementation committed in `ce3ad42`; recipe is version `1.3.0`, declares only `artifact_store_default`, appends one string-form interpolated rule, and documents the contract.
- P3.1–P3.4: triangulation passed with default/override materialization, all enum values through the existing hook, gate preservation, and scope-token guards.
- P4.1/P4.2: shared materialization test helper extracted and README/catalog/version surfaces reconciled; refactor committed in `ecd197e` and negative-surface cleanup in `551bb90`.
- P5.1: `./tests/run.sh` passed 1266 tests in 378.931s (`OK`). The initial 300-second attempt timed out without a test failure; rerun with the required longer timeout completed successfully.
- P5.2: `./tests/validate.sh` passed 1266 tests in 380.531s (`OK`).
- P5.3: final task checkbox and worktree review completed; no generated outputs or runtime internals were edited.

## TDD Cycle Evidence

| Task group | Test File / Command | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| P1.1–P1.8 | `tests/test_plan_build_flow_recipe.py`, renderer/catalog tests, hermetic eval | Unit + E2E | 137 focused baseline tests passed | 8 expected failures against config-less recipe; commit `43503cd` | 107 tests passed after recipe/docs; commit `ce3ad42` | 146 tests passed with defaults, overrides, enum cases, gate/baseline coverage | 121 tests passed after shared helper; commits `ecd197e`, `551bb90` |
| P2–P4 | `tests/evals/run.sh` | Hermetic eval | N/A | Scenario failed before recipe field/rule | Delivery eval passed after implementation | 38 eval tests passed, 10 live tests skipped | Reused existing eval harness |
| P5 | `./tests/run.sh`, `./tests/validate.sh` | Full configured unittest/validation | Focused TDD evidence above | N/A | 1266/1266 passed | N/A | N/A |

## Files changed

- `catalog/recipes/plan-build-flow/recipe.toml`
- `catalog/recipes/plan-build-flow/README.md`
- `docs/recipes-catalog.md`
- `tests/test_plan_build_flow_recipe.py`
- `tests/test_agents_render_brief_fragments.py`
- `tests/test_recipes_catalog.py`
- `tests/evals/eval_plan_build_flow_live.py`
- `tests/evals/scenarios/plan-build-flow/ac_delivery_contract_artifact_store/{scenario.toml,prompt.txt}`
- `openspec/changes/2026-08-02-plan-build-delivery-contracts/tasks.md`
- `openspec/changes/2026-08-02-plan-build-delivery-contracts/apply-progress.md`

## Remaining tasks

None. All P0–P5 tasks are checked in `tasks.md`.

## Deviations

- `plan-build-gate.sh` and all `lib/_internal/*` files remain untouched. Generated outputs remain sync products and were not hand-edited.
- The existing eval harness is reused; the new live scenario is opt-in through `EVALS_LIVE=1` and does not require an external preflight package.

## Live eval note

The opt-in `EVALS_LIVE=1 EVALS_RUNTIMES=opencode EVALS_SCENARIOS=ac_delivery_contract_artifact_store ./tests/evals/run-live.sh` reached the delivery scenario and materialized the brief, but the external `opencode` provider returned `UnknownError: Unexpected server error. Check server logs for details.` (exit 1, no project files changed). The corrected harness version setup passes hermetically; live execution requires a healthy provider and is not a repository failure.
