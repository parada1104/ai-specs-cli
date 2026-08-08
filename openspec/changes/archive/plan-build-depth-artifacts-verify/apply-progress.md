# Apply Progress: plan-build-depth-artifacts-verify

## Structured status consumed

```yaml
schemaName: spec-driven
changeName: plan-build-depth-artifacts-verify
artifactStore: openspec
planningHome:
  root: /Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/plan-build-depth-artifacts-verify
  changesDir: openspec/changes
changeRoot: openspec/changes/plan-build-depth-artifacts-verify
artifactPaths:
  proposal: [openspec/changes/plan-build-depth-artifacts-verify/proposal.md]
  specs: [openspec/changes/plan-build-depth-artifacts-verify/specs/plan-build-flow/spec.md]
  design: [openspec/changes/plan-build-depth-artifacts-verify/design.md]
  tasks: [openspec/changes/plan-build-depth-artifacts-verify/tasks.md]
  applyProgress: [openspec/changes/plan-build-depth-artifacts-verify/apply-progress.md]
  verifyReport: [openspec/changes/plan-build-depth-artifacts-verify/verify-report.md]
  syncReport: []
artifacts:
  proposal: done
  specs: done
  design: done
  tasks: partial
  applyProgress: done
  verifyReport: done
taskProgress:
  total: 21
  complete: 20
  remaining: 1
  unchecked:
    - "5.2 PR only after planning + implementation commits exist; verify gate before archive-tail; archive-tail on the review branch; guardian --stage pre-merge OK before merge."
applyState: ready
dependencies:
  apply: all_done
  verify: all_done
  sync: ready
  archive: blocked
actionContext:
  mode: repo-local
  workspaceRoot: /Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/plan-build-depth-artifacts-verify
  allowedEditRoots: []
  warnings: []
nextRecommended: review-branch archive-tail, then pre-merge guardian before merge; task 5.2 remains open until both are actually exercised
```

The implementation snapshot is committed in this worktree. Archive-tail is intentionally not run in this assignment, and no archive or PR claim is made.

## Completed work and persisted checkboxes

- Phase 0 rebase/auth baseline recorded; #59 parent SHA is
  `e2774c430f4c2a35e9e9988b803793ff046ee717`.
- Phase 1 guardian tests and implementation complete; tasks 1.1–1.4 are `[x]`.
- Phase 2 skill, recipe, README/docs, tests, and changelog complete; tasks 2.1–2.5 are `[x]`.
- Phase 3 canonical spec promotion/reconciliation and #59 preservation checks
  complete; tasks 3.1–3.4 are `[x]`.
- Phase 4 focused tests, full validation, and this change's own Full verify report
  complete; tasks 4.1–4.3 are `[x]`.
- Task 5.1 preservation re-check and 5.3 dogfood isolation decision are `[x]`.
- Persisted task artifact was re-read after updates; only task 5.2 remains unchecked.

## Files changed

- `lib/_internal/premerge_guardian.py`
- `tests/test_premerge_guardian.py`
- `tests/test_plan_build_flow_recipe.py`
- `catalog/recipes/plan-build-flow/skills/plan-build-flow/SKILL.md`
- `catalog/recipes/plan-build-flow/recipe.toml`
- `catalog/recipes/plan-build-flow/README.md`
- `docs/recipes-catalog.md`
- `CHANGELOG.md`
- `openspec/specs/plan-build-flow/spec.md`
- `openspec/changes/plan-build-depth-artifacts-verify/{proposal.md,explore.md,design.md,tasks.md}`
- `openspec/changes/plan-build-depth-artifacts-verify/specs/plan-build-flow/spec.md`
- `openspec/changes/plan-build-depth-artifacts-verify/apply-progress.md`
- `openspec/changes/plan-build-depth-artifacts-verify/verify-report.md`

The planning artifacts were updated from the rebased #59 context and were not
replaced by pre-#59 copies.

## TDD Cycle Evidence

| Task | Test file | Layer | Safety net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| 1.1 | `tests/test_premerge_guardian.py` | Unit | 8 existing tests | Written; focused run failed on missing proposal/report/stage behavior | 17/17 focused tests passed after guardian implementation | Added Light, Standard, Full, aliases, siblings, and prearchive cases | Consolidated tier/minimum/evidence parsing helpers; 17/17 remained green |
| 1.2 | `tests/test_premerge_guardian.py` | Unit | 8 existing tests | Same RED cycle | `check_verify_evidence` and minima pass 17/17 | Missing fields, qualified verdicts, strict Full marker | Pure field/minimum helpers retained |
| 1.3 | `tests/test_premerge_guardian.py` | Unit/CLI | Existing CLI signature tests | `--stage` rejected and `check_prearchive` absent | `--stage pre-archive` active-folder case passes; 17/17 | Pre-merge default plus active prearchive path | Shared `_inspect_folder` path |
| 1.4 | `tests/test_premerge_guardian.py` | Unit | Focused guardian suite | Existing inference path exposed new minima failures | 17/17 | Inference, explicit tier, aliases, sibling isolation | No behavior change to `DEPTH_RE` |
| 2.4 | `tests/test_plan_build_flow_recipe.py` | Integration-style recipe/materialization | 23/23 baseline tests | New pins/markers failed against 1.5.0 surfaces | 24/24 focused recipe tests passed | #59 assertions, seven-rule topology, vocabulary/materialization | No schema/materializer changes |

### Test summary

- Focused guardian: `python3 -m unittest tests.test_premerge_guardian` — 30/30 PASS (includes Full mapping, impossible-date, nested-heading, and fenced-evidence regressions).
- Focused recipe: `python3 -m unittest tests.test_plan_build_flow_recipe` — 25/25 PASS.
- Full validation: `./tests/validate.sh` — 1344 tests PASS, exit 0.
- Preservation script: #59 classifier diff is exactly three deletions plus the
  three intended chain/pointer/Light changes; rule 1/rule 7 and seven-rule
  topology invariants PASS.
- Prearchive smoke: guardian CLI with `--tier full --stage pre-archive` — OK
  on the active folder.
- Pre-merge smoke before archive-tail: guardian CLI with `--tier full
  --stage pre-merge` — BLOCKED as expected because the active folder remains
  and the archive is absent; this is not an archive-tail pass.
- Coverage, linter, type checker, and formatter are unavailable per
  `openspec/config.yaml`; no unavailable signal is claimed as passing.

## Deviations from design

- The canonical spec delta was promoted manually into the post-#59 canonical
  spec, preserving the four #59 requirements and classifier/annotation text.
- Guardian report parsing accepts the specified label synonyms and list/plain/
  Markdown-table punctuation. Full success-criteria mapping is machine-checked
  against proposal/design criteria ordinals with strict PASS rows.
- No recipe schema, materializer, hook, commit, push, merge, archive-tail, or PR
  behavior was changed.

## Remaining work

- **5.2 (unchecked):** the snapshot commit and pre-archive check are recorded,
  but archive-tail and a passing pre-merge guardian on the review branch were
  not run under this assignment. Do not mark 5.2 complete until both are
  exercised by the owning review workflow.
- Sync is ready but must be performed by the owning review workflow after
  review; archive is blocked until the review-branch archive-tail and guardian
  pass.
