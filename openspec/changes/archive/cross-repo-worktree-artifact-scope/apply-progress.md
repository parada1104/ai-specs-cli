# Apply Progress — PR1

## Structured status consumed

```yaml
schemaName: spec-driven
changeName: cross-repo-worktree-artifact-scope
artifactStore: both
planningHome:
  root: /Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/cross-repo-worktree-artifact-scope
  changesDir: openspec/changes
changeRoot: openspec/changes/cross-repo-worktree-artifact-scope
artifactPaths:
  proposal: [openspec/changes/cross-repo-worktree-artifact-scope/proposal.md]
  specs: [openspec/changes/cross-repo-worktree-artifact-scope/specs/plan-build-flow/spec.md]
  design: [openspec/changes/cross-repo-worktree-artifact-scope/design.md]
  tasks: [openspec/changes/cross-repo-worktree-artifact-scope/tasks.md]
  applyProgress: [openspec/changes/cross-repo-worktree-artifact-scope/apply-progress.md]
  verifyReport: []
  syncReport: []
contextFiles:
  proposal: [proposal.md]
  specs: [specs/plan-build-flow/spec.md]
  design: [design.md]
  tasks: [tasks.md]
  applyProgress: []
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
  total: 10
  complete: 6
  remaining: 4
  unchecked:
    - 1.3 Add RED recipe-surface checks (reserved for PR2)
    - 4. Documentation, recipe surfaces, and version bump (PR2)
    - 5. Canonical spec promotion (PR2)
    - 6. Focused verification and full validation (PR2)
applyState: ready
dependencies:
  apply: ready
  verify: blocked
  sync: blocked
  archive: blocked
actionContext:
  mode: repo-local
  workspaceRoot: /Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/cross-repo-worktree-artifact-scope
  allowedEditRoots:
    - /Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/cross-repo-worktree-artifact-scope
  warnings: []
nextRecommended: sdd-verify PR1, then continue PR2 documentation/spec work
```

## Completed tasks

- [x] **1.1** Built a hermetic superproject, initialized local submodule, and linked submodule worktree fixture. The fixture asserts the linked worktree root and the empty `--show-superproject-working-tree` probe, and uses canonical temporary paths for macOS `/private` aliases.
- [x] **1.2** Added topology and boundary scenarios in `tests/test_plan_build_gate_hook.py`, including central active/absent/archived plans, central creation/archive writes, central production blocking, empty superproject probe, similarly named parents, deinitialized submodule, ordinary worktree, non-existent targets, symlink escape, lookalike prefix, unrelated paths, and read-only topology checks.
- [x] **2.1** Replaced separate path probes in `plan-build-gate.sh` with one non-strict Python normalization step, component-aware canonical containment, and parameterized active-plan lookup.
- [x] **2.2** Added lazy fail-safe central-root resolution using canonical `--git-common-dir` `/modules/` structure, literal `.gitmodules` registration, initialized submodule checks, scoped status, and legacy corroboration fallback.
- [x] **3.1** Integrated the normative parse → normalize → nearest repository/boundary → agent config/artifact → production → nearest plan → lazy central resolver → central artifact/plan → diagnostic order.
- [x] **3.2** Triangulated all existing standalone, archive, mode, custom-path, agent-config, malformed-input, and outside-git tests against the new resolver.

## TDD evidence

| Phase | Command | Observed result |
|---|---|---|
| Baseline | `python3 -m unittest discover -s tests -p 'test_plan_build_gate_hook.py'` | `Ran 11 tests ... OK` |
| RED | `python3 -m unittest discover -s tests -p 'test_plan_build_gate_hook.py'` after fixture/tests and before hook edits | `Ran 26 tests ... FAILED (failures=4)`; central-plan allow and empty-probe allow returned exit 2; central-absence and similar-parent cases lacked the required central-root diagnostic. Failures were assertion failures, not fixture/setup errors. |
| GREEN | `python3 -m unittest discover -s tests -p 'test_plan_build_gate_hook.py'` | `Ran 26 tests ... OK` |
| Syntax | `bash -n catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh` | exit 0, no output |

## Files changed

- `catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh`
- `tests/test_plan_build_gate_hook.py`
- `openspec/changes/cross-repo-worktree-artifact-scope/tasks.md` (PR1 checklist markers)
- `openspec/changes/cross-repo-worktree-artifact-scope/apply-progress.md`

## Deviations and boundaries

- Recipe-surface RED checks, documentation, version changes, canonical spec promotion, generated outputs, and full validation remain intentionally deferred to PR2 as directed.
- No `[sdd]`, artifact-root selector, decision matrix, per-subrepository store, worktree-flow change, topology mutation, synchronization, or derived-output edit was introduced.
- The resolver is self-contained in the distributed hook and performs only Git/filesystem reads.

## Remaining tasks after PR1

PR1 intentionally deferred the following work to PR2; all are now complete in the PR2 section below:

- [x] 1.3 Add RED recipe-surface checks (completed in PR2)
- [x] 4. Documentation, recipe surfaces, and version bump (completed in PR2)
- [x] 5. Canonical spec promotion (completed in PR2)
- [x] 6. Focused verification and full validation (completed in PR2)

## Workload / PR boundary

Parent approval selected two chained PRs (`auto-chain`). This is PR1 only: fixture/RED tests, hook normalization/resolver, and normative gate flow. The current source diff is 2 files, approximately 298 added and 74 removed lines; derived consumer output is untouched. PR2 owns recipe/docs/version/spec promotion and final validation.

## Apply Progress — PR2

### Structured status consumed and produced

```yaml
schemaName: spec-driven
changeName: cross-repo-worktree-artifact-scope
artifactStore: both
planningHome:
  root: /Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/cross-repo-worktree-artifact-scope
  changesDir: openspec/changes
changeRoot: openspec/changes/cross-repo-worktree-artifact-scope
artifactPaths:
  proposal: [openspec/changes/cross-repo-worktree-artifact-scope/proposal.md]
  specs: [openspec/changes/cross-repo-worktree-artifact-scope/specs/plan-build-flow/spec.md, openspec/specs/plan-build-flow/spec.md]
  design: [openspec/changes/cross-repo-worktree-artifact-scope/design.md]
  tasks: [openspec/changes/cross-repo-worktree-artifact-scope/tasks.md]
  applyProgress: [openspec/changes/cross-repo-worktree-artifact-scope/apply-progress.md]
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
  total: 10
  complete: 10
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
  workspaceRoot: /Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/cross-repo-worktree-artifact-scope
  allowedEditRoots:
    - /Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/cross-repo-worktree-artifact-scope
  warnings: []
nextRecommended: sdd-verify cross-repo-worktree-artifact-scope
```

### Completed tasks and persisted checklist updates

- [x] **1.3** Added recipe-surface RED assertions for the 1.4.0 version, topology-derived central planning wording, no-new-configuration vocabulary guards, and unchanged runtime hook/on-sync wiring. `tasks.md` line 35 is checked.
- [x] **4** Updated the catalog README, bundled skill, recipe manifest, and catalog documentation. The recipe is now 1.4.0 at the four pinned locations (manifest, README enablement, catalog snippet, and test expectation); the existing artifact-store schema and worktree coexistence contract remain intact. `tasks.md` line 40 is checked.
- [x] **5** Promoted the verified topology, root-discovery, canonical-path, centralized-artifact, narrow-write, read-only, hook, and classic-flow coexistence requirements into `openspec/specs/plan-build-flow/spec.md`. The change delta remains available and was not archived or deleted. `tasks.md` line 41 is checked.
- [x] **6** Ran focused hook and recipe suites, regenerated derived recipe output through `recipe-materialize.py`, and ran the full test and validation scripts. `tasks.md` line 42 is checked.

### TDD Cycle Evidence

| Task | Test file | Layer | Safety net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 1.3 | `tests/test_plan_build_flow_recipe.py` | Unit/materialization | Pre-change 17-test recipe invocation was green before the PR2 assertions | 18 tests, 3 assertion failures for missing topology guidance, 7th brief rule, and 1.4.0 pins; no fixture/import failures | 18 tests OK after catalog sources, brief, and version edits | 19 tests OK after materializing generated README/skill and checking the second output path | Test assertions corrected to the schema's runtime-hook model and generated skill destination; 19 tests remained green |
| 4 | `tests/test_plan_build_flow_recipe.py` | Unit/materialization | 18 tests green | Covered by task 1.3 RED assertions | 18 tests OK | 19 tests OK through materialized output | Source-only catalog edits; derived files were regenerated, not hand-edited |
| 5 | `tests/test_plan_build_flow_recipe.py`, `tests/test_plan_build_gate_hook.py` | Contract/unit | Hook suite 26 tests green after PR1 | Existing PR1 topology RED evidence retained above | Hook 26 tests OK and recipe 19 tests OK | Full suite 1283 tests OK | Canonical spec aligned section-by-section with the delta |
| 6 | Focused suites and repository scripts | Unit/full validation | Focused suites green | N/A — verification-only task | Hook 26 OK; recipe 19 OK; `./tests/run.sh` 1283 OK; `./tests/validate.sh` 1283 OK | Materializer regeneration completed successfully | No formatter/linter/type-checker configured |

### Commands and observed results

- `python3 -m unittest discover -s /Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/cross-repo-worktree-artifact-scope/tests -p 'test_plan_build_flow_recipe.py'` — RED: `Ran 18 tests ... FAILED (failures=3)` before source edits.
- Same absolute focused recipe command — GREEN: `Ran 18 tests ... OK` after source docs/version edits.
- Same absolute focused recipe command after materialization triangulation — `Ran 19 tests ... OK`.
- `python3 -m unittest discover -s /Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/cross-repo-worktree-artifact-scope/tests -p 'test_plan_build_gate_hook.py'` — `Ran 26 tests ... OK`.
- `python3 /Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/cross-repo-worktree-artifact-scope/lib/_internal/recipe-materialize.py /Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/cross-repo-worktree-artifact-scope /Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/cross-repo-worktree-artifact-scope` — completed successfully; regenerated the enabled recipe outputs, with only existing tag-overlap and customized-worktree-override notices.
- `bash /Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/cross-repo-worktree-artifact-scope/tests/run.sh` — `Ran 1283 tests ... OK`.
- `bash /Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/cross-repo-worktree-artifact-scope/tests/validate.sh` — Python compilation, Bash syntax, smoke checks, and the same `Ran 1283 tests ... OK` result passed.

### Files changed in PR2

- `catalog/recipes/plan-build-flow/README.md`
- `catalog/recipes/plan-build-flow/skills/plan-build-flow/SKILL.md`
- `catalog/recipes/plan-build-flow/recipe.toml`
- `docs/recipes-catalog.md`
- `tests/test_plan_build_flow_recipe.py`
- `openspec/specs/plan-build-flow/spec.md`
- `openspec/changes/cross-repo-worktree-artifact-scope/tasks.md`
- `openspec/changes/cross-repo-worktree-artifact-scope/apply-progress.md`

### Deviations, boundaries, and remaining work

- No `[sdd]`, `artifact_root`, decision-matrix, per-subrepository store, orchestration, synchronization, worktree-flow, tracker, memory, or unrelated-recipe-version behavior was added or changed.
- No materialized `ai-specs/recipes/plan-build-flow/**` output was hand-edited; output was regenerated and validated by the existing materializer and tests.
- The materializer reported pre-existing tag-overlap and customized `worktree-flow` override notices; these did not fail the command or alter the allowed PR2 source scope.
- No unchecked task lines remain. Review/verify reporting and any later sync/archive phase remain outside this apply assignment.
- Parent-approved workload boundary: PR2 is the second chained PR and contains only recipe-surface RED/tests, catalog docs/skill/manifest/version, canonical spec promotion, generated-output verification, and final validation; PR1 hook/fixture files remain unchanged by PR2.
