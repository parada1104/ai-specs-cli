# Tasks: ai-specs-owned topology context and conservative gate refresh

Depth: full; strict TDD; test command: `./tests/run.sh`.

## Review Workload Forecast

| Field | Value |
|---|---|
| Estimated authored changes | 650–800 lines, excluding generated snapshots |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | WU1 → WU2, one PR (`size:exception`) |
| Delivery strategy | exception-ok |
| Chain strategy | size-exception |

Approval: maintainer-approved `size:exception`; one PR; independent WU1/WU2 rollback.

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: size-exception
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|---|---|---|---|---|---|
| WU1 | Request context, fan-out, and canonical planning propagation | PR 1 (exception) | `python3 -m unittest tests.test_repo_topology tests.test_target_resolve tests.test_worktree_root_propagation tests.test_sync_pipeline tests.test_worktree_flow_recipe tests.test_plan_build_flow_recipe tests.test_premerge_guardian` | Temp superrepo + initialized submodule; exercise generated `/worktree-new`, explicit-error path, and central artifact writes | Revert WU1 paths and root/fan-out behavior only |
| WU2 | Gate baseline classification, refresh, backup, and doctor alignment | PR 1 (exception) | `python3 -m unittest tests.test_override_ownership tests.test_recipe_materialize tests.test_project_cache tests.test_doctor tests.test_plan_build_gate_hook` | Temp project with executable gate and lock/cache; run ordinary sync and `--refresh-gates` with provider absent/disabled | Revert gate provenance/refresh paths; preserve unrelated topology work |

## Phase 1: WU1 RED — topology and propagation

- [x] 1.1 RED: extend `tests/test_repo_topology.py` and `tests/test_worktree_root_propagation.py` for subrepo cwd ownership, superrepo-without-subrepo hard error before `git worktree add`, and detached/uninitialized/ambiguous fail-safe behavior.
- [x] 1.2 RED: extend `tests/test_worktree_flow_recipe.py` for explicit/inferred mismatch, path/name validation, absolute subrepo worktree destination, and longest-prefix inference.
- [x] 1.3 RED: extend `tests/test_target_resolve.py` and `tests/test_sync_pipeline.py` for declared-only fan-out, empty `project.subrepos`, first incompatible target stop, shared planning root, and stable `monorepo-apps`.

## Phase 2: WU1 GREEN/REFACTOR — request context and roots

- [x] 2.1 Implement `resolve_request_context()` in `lib/_internal/util.py`, reusing proven Git facts and validated `.gitmodules` without automatic fan-out.
- [x] 2.2 Propagate context through `lib/_internal/target-resolve.py`, `lib/sync.sh`, and `lib/sync-agent.sh`; preserve `project.subrepos` and emit `planning_root`, `topology`, and `declared_only`.
- [x] 2.3 Wire `project_root`/planning root through `lib/_internal/recipe-materialize.py`, `lib/_internal/agents-render.py`, and required `--root` handling in `lib/_internal/premerge_guardian.py`; keep absent/disabled orchestration inline and unchanged.
- [x] 2.4 Update `catalog/recipes/{worktree-flow,plan-build-flow}/**` and `openspec/specs/{worktree-flow,plan-build-flow}/spec.md`; document no executable `/worktree-new` helper and run WU1 tests, then refactor under green.

## Phase 3: WU2 RED — gate provenance threat cases

- [x] 3.1 RED: extend `tests/test_override_ownership.py`/`tests/test_recipe_materialize.py` for executable gates: matching baseline refreshes, byte mismatch preserves with warning, and missing provenance preserves without seeding.
- [x] 3.2 RED: add explicit-refresh tests for exact immutable cache backup, repeated content-hash collision safety, failed backup/lock atomicity, and absent/disabled provider parity.
- [x] 3.3 RED: extend `tests/test_doctor.py` and `tests/test_plan_build_gate_hook.py` for customized-gate warnings, quiet matching baseline, and unchanged no-Gentle behavior.

## Phase 4: WU2 GREEN/REFACTOR — refresh and documentation

- [x] 4.1 Implement gate classification/baseline and `--refresh-gates` in `lib/_internal/recipe-materialize.py` and `lib/_internal/lock.py`; ordinary sync never refreshes customized gates.
- [x] 4.2 Add `backups_root()` in `lib/_internal/project-cache.py` and atomic backup/gate/lock rollback; wire provenance diagnostics through `lib/doctor.sh` and `_internal/doctor.py`.
- [x] 4.3 Update `catalog/recipes/worktree-flow`, override-ownership docs, and `openspec/specs/override-ownership/spec.md`; run focused GREEN/refactor tests and `./tests/validate.sh`, recording RED/GREEN evidence.
