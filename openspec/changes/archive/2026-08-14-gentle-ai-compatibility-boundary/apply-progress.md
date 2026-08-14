# Apply Progress: gentle-ai-compatibility-boundary

## Status consumed

```yaml
schemaName: gentle-ai.sdd-status
changeName: gentle-ai-compatibility-boundary
artifactStore: openspec (hybrid session persistence: OpenSpec + Engram)
planningHome: .worktrees/gentle-ai-compatibility-boundary/openspec
changeRoot: openspec/changes/gentle-ai-compatibility-boundary
taskProgress: 0/13 → 13/13 complete
applyState: ready → (this run completes all tasks)
actionContext: repo-local; allowedEditRoots = [worktree]
delivery: exception-ok / size-exception (maintainer-approved single PR)
attempt: sha256:a403466914a617e7147c7f1d5f48f63fa9cc850b49370cb083044af080102ba0
         (acquire returned state=proceed, work-unit full-change-size-exception)
```

## Delivery decision

One PR with maintainer-approved `size:exception`; WU1 and WU2 kept as internal
rollback boundaries but implemented in this single apply transaction per the
delivery decision.

## Completed tasks

All 13 tasks (1.1–4.3) implemented in dependency order with strict TDD
RED → GREEN → REFACTOR evidence. All checkboxes marked `[x]` in tasks.md.

- [x] 1.1 RED: `resolve_request_context` tests (subrepo owner + super planning
      root, superrepo-without-subrepo hard error, fail-safe detached/
      uninitialized/non-git) in `test_repo_topology.py` + real-git integration
      in `test_worktree_root_propagation.py`.
- [x] 1.2 RED: doc-contract tests in `test_worktree_flow_recipe.py`
      (explicit-required superrepo context, owner vs planning root, Markdown
      command not executable helper, absolute destination, longest-prefix).
- [x] 1.3 RED: plan-JSON tests in `test_target_resolve.py` (declared_only,
      fanout_targets, planning_root, topology, stable monorepo-apps) + empty-
      subrepos/.gitmodules non-expansion sync test in `test_sync_pipeline.py`.
- [x] 2.1 `resolve_request_context()` + `RequestContext` + `_superproject_root()`
      in `lib/_internal/util.py` (reuses `resolve_subrepo`/
      `resolve_repo_topology`; `.gitmodules` validated, never auto-fanned-out).
- [x] 2.2 `target-resolve.py` plan emits `planning_root`, `topology`,
      `declared_only`, `fanout_targets`, `worktrees_dir`; `sync.sh`/
      `sync-agent.sh` consume and display the context (declared-only fan-out
      preserved).
- [x] 2.3 resolved-config carries `project_root` + `topology`
      (`recipe-materialize.py`); `agents-render.py` verified to consume them
      (already read `resolved.project_root`); `premerge_guardian.py` `--root`
      now required (never falls back to process cwd).
- [x] 2.4 `catalog/recipes/{worktree-flow,plan-build-flow}/**` docs updated
      (request context, superrepo hard-error, no executable `/worktree-new`
      helper, planning-root propagation to the guardian); main specs
      `openspec/specs/{worktree-flow,plan-build-flow}/spec.md` promoted from
      deltas; WU1 tests run; refactor under green.
- [x] 3.1 RED: gate classification tests (baseline-match refresh + baseline
      record kind=gate/policy=auto, byte-mismatch preserve + warn, missing
      provenance preserve without seeding) replacing the obsolete
      `test_hook_materialization_remains_unconditional`.
- [x] 3.2 RED: explicit-refresh tests (immutable cache backup of exact
      pre-refresh bytes, content-hash collision safety, failed-backup atomic
      abort, absent/disabled provider parity) + `backups_root()`/
      `gate_backup_path()` naming tests.
- [x] 3.3 RED: doctor gate-provenance tests (customized-gate warn, quiet
      matching baseline, missing-provenance warn, no-hook quiet) + plan-build
      gate no-Gentle parity test.
- [x] 4.1 Gate classification/baseline + `--refresh-gates` in
      `recipe-materialize.py` + atomic `write_lock` + `set_gate_baseline` in
      `lock.py`; ordinary sync never refreshes customized gates.
- [x] 4.2 `backups_root()`/`gate_backup_path()` in `project-cache.py`;
      all-or-nothing backup→gate→lock refresh with rollback; doctor
      `_check_gate_provenance` wired through `doctor.sh`/`doctor.py`.
- [x] 4.3 Docs (`catalog/recipes/worktree-flow/README.md`,
      `trello-mcp-workflow/README.md`, `docs/recipes-catalog.md`, `doctor.sh`)
      + `openspec/specs/override-ownership/spec.md` promoted; focused GREEN
      tests + `./tests/validate.sh` green.

## TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 1.1 | `tests/test_repo_topology.py` (RequestContextTests, 8) + `tests/test_worktree_root_propagation.py` (SubmoduleRequestContextIntegrationTests, 4) | Unit+Integration | ✅ 285/285 | ✅ 11/11 AttributeError on `resolve_request_context` | ✅ 12/12 | ✅ 8 distinct cases incl. real-git ownership | ✅ split `_superproject_root` helper |
| 1.2 | `tests/test_worktree_flow_recipe.py` | Unit (doc-contract) | ✅ 285/285 | ✅ 4/4 missing doc content | ✅ 4/4 | ✅ 4 scenarios | ✅ assertions aligned to spec wording |
| 1.3 | `tests/test_target_resolve.py` (5) + `tests/test_sync_pipeline.py` (1) | Unit+Integration | ✅ 285/285 | ✅ 6/6 missing plan fields | ✅ 6/6 | ✅ 6 cases (empty list, .gitmodules, monorepo-apps, shared root) | ✅ single-parse JSON read in sync.sh |
| 2.1–2.3 | same files + `test_recipe_materialize.py` (ResolvedConfigContextTests, 3) + `test_premerge_guardian.py` (1) | Unit | ✅ 285/285 | ✅ 4/4 (project_root/topology absent; cwd fallback) | ✅ 4/4 | ✅ 3 context cases + guardian CLI | ✅ dataclass frozen, helpers extracted |
| 2.4 | `test_worktree_flow_recipe.py` + `test_plan_build_flow_recipe.py` | Unit | ✅ 285/285 | (RED carried from 1.2) | ✅ 50/50 | ✅ spec-merge + docs consistency | ✅ spec merge scripted deterministically |
| 3.1 | `tests/test_override_ownership.py` | Unit | ✅ 285/285 | ✅ 2/2 behavioral (unconditional overwrite) + API errors | ✅ 2/2 | ✅ 3 gate states (match/mismatch/missing) | ✅ shared `_hook_project`/`_hook_lock_entry` helpers |
| 3.2 | `tests/test_override_ownership.py` (4) + `tests/test_project_cache.py` (2) | Unit | ✅ 285/285 | ✅ 6/6 missing `backups_root`/`gate_backup_path`/`refresh_gates` | ✅ 6/6 | ✅ 6 cases incl. collision + atomic abort | ✅ test parent-as-file collision fixture |
| 3.3 | `tests/test_doctor.py` (GateProvenanceDoctorTests, 4) + `tests/test_plan_build_gate_hook.py` (1) | Unit+Integration | ✅ 285/285 | ✅ 5/5 missing `_check_gate_provenance` / parity | ✅ 5/5 | ✅ 4 doctor states + gate parity | ✅ doctor check mirrors classifier |
| 4.1–4.2 | `test_override_ownership.py`, `test_recipe_materialize.py`, `test_lock.py`, `test_trello_mcp_workflow_recipe.py` | Unit | ✅ 285/285 | (RED carried from 3.1–3.2) | ✅ 281/281 | ✅ trello direct-call path + stale-gate path | ✅ atomic lock via tempfile+os.replace |
| 4.3 | `tests/test_sync_pipeline.py` (GateRefreshCliTests E2E, 1) | E2E | ✅ 285/285 | (behavioral RED from 3.1–3.2) | ✅ 1/1 | ✅ full CLI sync → customize → `--refresh-gates` | ✅ sync.sh flag forwarded once |

### Test Summary
- Total tests written for this change: **41** (11 + 4 + 6 + 4 + 3 + 2 + 5 + 1 + 2 + 1 + 2 = 41 new/extended test methods; some RED batches overlap by task).
- Full suite: **1656 tests OK** (116 skipped: Go binary/network-dependent), `./tests/validate.sh` exit 0.
- Layers: Unit (35), Integration (5), E2E CLI (1).
- Approval tests: `test_git_dash_c_create_yields_subrepo_owned_worktree` (real-git ownership contract) — 1.
- Pure functions created: `resolve_request_context`, `_superproject_root`, `_write_gate_backup`, `_refresh_gate`, `backups_root`, `gate_backup_path`, `set_gate_baseline`, `_worktree_flow_config` (target-resolve + materialize) — 8.

## Work Unit Evidence

### WU1 — Request context, fan-out, and canonical planning propagation

| Evidence | Required value |
|---|---|
| Focused test command and exact result | `python3 -m unittest tests.test_repo_topology tests.test_target_resolve tests.test_worktree_root_propagation tests.test_worktree_flow_recipe tests.test_plan_build_flow_recipe tests.test_premerge_guardian` → **Ran 135 tests, OK (skipped=1)**; `tests.test_sync_pipeline` → **Ran 90 tests, OK** (127.5s) |
| Runtime harness command/scenario and exact result | Real-git integration: superproject + initialized submodule; `resolve_request_context(submodule cwd)` → owner=subrepo, planning_root=super; `git -C <subrepo> worktree add` registers in the **subrepo** `git worktree list` and NOT the superproject's (test_git_dash_c_create_yields_subrepo_owned_worktree); superrepo cwd without explicit subrepo raises `SubrepoResolutionError` with **zero** worktrees created (worktree list byte-identical before/after). Full `bin/ai-specs sync` on temp fixtures in `test_sync_pipeline` exercises the plan JSON + resolved-config context. |
| Rollback boundary | Revert: `lib/_internal/util.py`, `lib/_internal/target-resolve.py`, `lib/sync.sh`, `lib/sync-agent.sh`, `lib/_internal/recipe-materialize.py` (project_root/topology only), `lib/_internal/premerge_guardian.py` (`--root` required), `catalog/recipes/worktree-flow/**`, `catalog/recipes/plan-build-flow/**`, `openspec/specs/{worktree-flow,plan-build-flow}/spec.md`, and WU1 test files. WU2 gate/refresh behavior is untouched by reverting these paths. |

### WU2 — Gate baseline classification, refresh, backup, and doctor alignment

| Evidence | Required value |
|---|---|
| Focused test command and exact result | `python3 -m unittest tests.test_override_ownership tests.test_recipe_materialize tests.test_project_cache tests.test_lock tests.test_doctor tests.test_plan_build_gate_hook tests.test_trello_mcp_workflow_recipe tests.test_worktree_flow_recipe tests.test_premerge_guardian` → **Ran 281 tests, OK** (47.5s) |
| Runtime harness command/scenario and exact result | E2E `GateRefreshCliTests`: temp project, `bin/ai-specs init`, `bin/ai-specs sync` (gate materialized + baseline recorded, `AI_SPECS_GATE_OFFLINE=1`), gate customized, ordinary `bin/ai-specs sync` → **preserved** with `user-modified` warning; `bin/ai-specs sync --refresh-gates` → gate restored to CLI-rendered bytes and cache-only immutable backup contains the exact pre-refresh bytes. Absent/disabled provider parity proven via subprocess with `GENTLE_AI_MODE=disabled` vs clean env → identical bytes. |
| Rollback boundary | Revert: `lib/_internal/recipe-materialize.py` (gate classification/refresh only), `lib/_internal/lock.py` (atomic write + `set_gate_baseline`), `lib/_internal/project-cache.py` (`backups_root`/`gate_backup_path`), `lib/_internal/doctor.py` + `lib/doctor.sh` (gate-provenance), `lib/sync.sh` (`--refresh-gates` flag), `catalog/recipes/{worktree-flow,trello-mcp-workflow}/README.md`, `docs/recipes-catalog.md`, `openspec/specs/override-ownership/spec.md`, and WU2 test files. WU1 topology/planning work is untouched by reverting these paths. |

## Runtime harness smoke (manual, temp fixtures only)

`./tests/validate.sh` → **exit 0, Ran 1656 tests, OK (skipped=116)** — full
syntax checks (py_compile + bash -n), Go gate suite, and unittest discovery.
No live sync/init/doctor was run against user consumer projects
(`melon-alquimia`, `salones`, `venturi_coffee`); all runtime harness runs used
temp fixtures (cleaned via TemporaryDirectory) or the repo's own scratch
AI_SPECS_HOME symlink-farm pattern (cache excluded). No Gentle AI repository or
external authority was touched.

## Deviations from design

1. **Main-spec promotion during apply** — the tasks explicitly list
   `openspec/specs/{worktree-flow,plan-build-flow,override-ownership}/spec.md`
   as apply targets; this matches the repo convention (previous apply-progress
   "promoted canonical specs"). Deltas were merged deterministically with a
   script (MODIFIED replaced, ADDED appended, `(Previously:` annotations
   stripped). Archive's delta sync therefore operates on already-promoted
   content — flagging for verify/archive awareness. A stray pre-existing
   `(Previously:` note in `openspec/specs/vcs-pr-flow/spec.md` that was
   accidentally stripped by the cleanup pass was restored via `git checkout`.
2. **Actual changed-line count exceeds the forecast** — the final diff is
   ~1,788 changed lines (1,722 insertions + 66 deletions), above the 650–800
   authored-line forecast and the attempt's `--max-changed-lines 800`. Drivers:
   the three main-spec promotions (~570 lines of delta text), the mandated TDD
   test surface (41 test methods across 12 files), and doc updates. The
   maintainer-approved `size:exception` covers one PR; the orchestrator should
   settle the attempt with actuals recorded.
3. **`test_sentinel_upgrade_replaces_pre_go_gate` updated** — the pre-Go
   sentinel upgrade pinned unconditional rewrite, which the provenance model
   intentionally supersedes (design: "Gates without baseline → preserved +
   warning (no seeding)"). The test now pins preserve-on-ordinary-sync and
   upgrade via `--refresh-gates`. Same class as the design-mandated replacement
   of `test_hook_materialization_remains_unconditional`.
4. **`sync.sh --refresh-gates` flag added** — required so the documented
   `ai-specs sync --refresh-gates` command actually works end-to-end (the
   flag was specified on `recipe-materialize.py`; the CLI wrapper needed
   forwarding). Covered by the E2E CLI test.
5. **No `utils`-shared fanout helper** — `_worktree_flow_config` is duplicated
   (small) between `target-resolve.py` and `recipe-materialize.py`; extracting
   would couple modules for ~10 lines. Noted, not changed.
6. **`doctor.sh` unchanged logic** — it already execs `doctor.py`; the
   gate-provenance diagnostics flow through automatically. Its header comment
   was extended to document the check.

## Issues found

None blocking. One test-side misstep (a duplicate-class-header edit) was
caught by the suite and corrected; `vcs-pr-flow/spec.md` accidental edit was
reverted and verified clean.

## Workload / PR boundary

- Mode: single PR, `size:exception` (maintainer-approved), delivery
  `exception-ok`.
- Current work unit: full-change (WU1 + WU2 in one transaction, independent
  rollback boundaries per the tables above).
- Boundary: 13/13 tasks complete; `./tests/validate.sh` green; ready for
  `sdd-verify`.
- Estimated review budget impact: ~1,788 changed lines (above the 400-line
  default and the 800-line attempt cap; see deviation 2).

## Status

13/13 tasks complete. Ready for `sdd-verify`.
