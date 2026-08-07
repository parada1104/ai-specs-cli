# Apply Progress: plan-build-depth-adversarial

## Status consumed

```yaml
schemaName: spec-driven
changeName: plan-build-depth-adversarial
artifactStore: openspec
planningHome:
  root: /Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/plan-build-depth-adversarial/openspec
  changesDir: openspec/changes
changeRoot: openspec/changes/plan-build-depth-adversarial
artifactPaths:
  proposal: [openspec/changes/plan-build-depth-adversarial/proposal.md]
  specs: [openspec/changes/plan-build-depth-adversarial/specs/plan-build-flow/spec.md]
  design: []
  tasks: [openspec/changes/plan-build-depth-adversarial/tasks.md]
  applyProgress: [openspec/changes/plan-build-depth-adversarial/apply-progress.md]
  verifyReport: [openspec/changes/plan-build-depth-adversarial/verify-report.md]
  syncReport: []
artifacts:
  proposal: done
  specs: done
  design: not required for Standard depth
  tasks: done
  applyProgress: done
  verifyReport: done
  syncReport: missing
taskProgress:
  total: 18
  complete: 18
  remaining: 0
  unchecked: []
applyState: ready
dependencies:
  apply: ready
  verify: ready
  sync: blocked
  archive: blocked
actionContext:
  mode: repo-local
  workspaceRoot: /Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/plan-build-depth-adversarial
  allowedEditRoots: [/Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/plan-build-depth-adversarial]
  warnings: []
nextRecommended: enable serialized #60 evaluation after #59 handoff
```

## Completed implementation

- Updated the bundled classifier policy in `catalog/recipes/plan-build-flow/skills/plan-build-flow/SKILL.md` to compute signal and explicit requested depth separately, detect conflicts in both directions, ask before planning, support same-turn resolution, preserve signal-only behavior, emit the standalone annotation contract, and require a complete deeper planning chain.
- Updated the recipe README planning-depth guidance and retargeted the enable example to `1.5.0`.
- Updated `catalog/recipes/plan-build-flow/recipe.toml` to `1.5.0` from development's `1.4.0` baseline and extended the classify brief rule with compare/ask/annotation guidance; preserved development's seventh topology rule and added no depth configuration keys.
- Updated the plan-build entry in `docs/recipes-catalog.md` to `1.5.0` and added the Unreleased changelog entry. The historical topology release entry remains unchanged.
- Promoted the authorized classifier delta into `openspec/specs/plan-build-flow/spec.md`; unrelated artifact-minimum, verify-gate, PR/archive, and guardian requirements were not edited.
- Added focused classifier, brief, and standalone annotation regressions and updated version-pinned assertions in `tests/test_plan_build_flow_recipe.py`.
- Persisted completed implementation, eval, and verification checkboxes in `openspec/changes/plan-build-depth-adversarial/tasks.md`; dogfood brief refresh is explicitly N/A and non-blocking.

## TDD Cycle Evidence

| Task | Test file | Layer | Safety net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| Classifier/docs/version/spec implementation | `tests/test_plan_build_flow_recipe.py` | `Unit/materialization` | `19/19 baseline passing` | `19 tests, 4 failures after conflict/version assertions were written first` | `22 tests passing, including distinct standalone annotation assertions` | `Clarified `Decision source: user` semantics and fixed regex assertion; 22 tests still passing` |

### Test Summary

- Baseline: `python3 -m unittest discover -s .worktrees/plan-build-depth-adversarial/tests -p 'test_plan_build_flow_recipe.py'` — 19 tests passed before implementation.
- RED: same focused discovery — 19 tests run, 4 expected failures (missing classifier policy and `1.5.0` surfaces).
- GREEN/TRIANGULATE: focused discovery — 22 tests passed after rebasing and preserving the seventh topology rule.
- Mandated validation: `sh .worktrees/plan-build-depth-adversarial/tests/validate.sh` — 1319 tests passed.
- No formatter, linter, merge, push, or guardian changes were run.

## Scope and ownership guard

- #60-owned artifact minima, staged verify gates, PR/archive guardian behavior, and
  `lib/_internal/premerge_guardian.py` were left untouched.
- Runtime coverage is now present; generated dogfood brief refresh is explicitly N/A
  for #59 and was not run. Any future sync is verification-only and non-blocking.



## Runtime eval coverage

- Completed the optional eval task in `tasks.md` (`- [x]`): added two deterministic
  plan-build live scenarios. `ac_depth_conflict_ask` sends an unresolved Full vs
  Standard request and requires both tiers plus an ask marker in the transcript,
  while forbidding `openspec/**` and production edits. `ac_depth_conflict_same_turn`
  sends an explicit same-turn Full resolution and requires `Depth: full`, all four
  annotation labels, proposal/design/spec/tasks artifacts, and no `src/**` edits.
- Extended `eval_plan_build_flow_live.py` with both scenario selectors and a
  `required_transcript_one_of` assertion for the unresolved ask. Added a focused
  smoke contract test in `eval_harness_smoke.py` for scenario metadata.
- Files changed:
  - `tests/evals/eval_plan_build_flow_live.py`
  - `tests/evals/eval_harness_smoke.py`
  - `tests/evals/scenarios/plan-build-flow/ac_depth_conflict_ask/{scenario.toml,prompt.txt}`
  - `tests/evals/scenarios/plan-build-flow/ac_depth_conflict_same_turn/{scenario.toml,prompt.txt}`
  - `openspec/changes/plan-build-depth-adversarial/tasks.md`
- Focused evidence:
  - `python3 -m unittest tests.evals.eval_harness_smoke.HarnessSmokeTests.test_depth_conflict_scenarios_cover_runtime_contract -v` — 1 passed.
  - `EVALS_LIVE=1 EVALS_RUNTIMES=claude EVALS_SCENARIOS=ac_depth_conflict_ask EVALS_TIMEOUT_SEC=300 EVALS_MAX_TURNS=12 ./tests/evals/run-live.sh` — 1 selected live scenario passed; 7 unrelated tests skipped.
  - `EVALS_LIVE=1 EVALS_RUNTIMES=claude EVALS_SCENARIOS=ac_depth_conflict_same_turn EVALS_TIMEOUT_SEC=300 EVALS_MAX_TURNS=16 ./tests/evals/run-live.sh` — 1 selected live scenario passed; 7 unrelated tests skipped.
  - `python3 -m py_compile tests/evals/eval_plan_build_flow_live.py tests/evals/eval_harness_smoke.py` — passed.
- TDD evidence: the focused metadata smoke test is the RED/GREEN safety net for
  the new scenario contract; live runtime trials are the behavioral TRIANGULATE
  evidence. No production code was changed and no formatter/full suite ran.
- Deviation: the unresolved prompt explicitly states that the user has not chosen
  a winning tier and requests the question before any file creation; this keeps
  the runtime scenario natural while making the required stop observable.

## Remaining tasks

- None. The conditional dogfood brief refresh is N/A for #59; any future sync is
  verification-only and non-blocking, and must follow dogfood-verification-isolation.

## Workload / PR boundary

- Small #59 follow-up only; no chained PR or size exception is needed. Changes are
  limited to the existing eval harness/scenarios and persisted apply notes.
- `actionContext.mode: repo-local`; authoritative workspace and allowed edit root
  remain `.worktrees/plan-build-depth-adversarial`; no merge or push performed.
