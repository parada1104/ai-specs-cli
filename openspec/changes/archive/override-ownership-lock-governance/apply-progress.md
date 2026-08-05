# Apply Progress: override-ownership-lock-governance

## Status consumed

```yaml
schemaName: spec-driven
changeName: override-ownership-lock-governance
artifactStore: openspec
planningHome:
  root: /Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/override-ownership-lock-governance
  changesDir: openspec/changes
changeRoot: openspec/changes/override-ownership-lock-governance
artifactPaths:
  proposal: [openspec/changes/override-ownership-lock-governance/proposal.md]
  specs:
    - openspec/changes/override-ownership-lock-governance/specs/sync-lock/spec.md
    - openspec/changes/override-ownership-lock-governance/specs/override-ownership/spec.md
    - openspec/changes/override-ownership-lock-governance/specs/recipe-schema/spec.md
    - openspec/changes/override-ownership-lock-governance/specs/worktree-flow/spec.md
  design: [openspec/changes/override-ownership-lock-governance/design.md]
  tasks: [openspec/changes/override-ownership-lock-governance/tasks.md]
  applyProgress: [openspec/changes/override-ownership-lock-governance/apply-progress.md]
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
  total: 19
  complete: 19
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
  workspaceRoot: /Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/override-ownership-lock-governance
  allowedEditRoots: [/Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/override-ownership-lock-governance]
  warnings: []
nextRecommended: sdd-verify
```

Authorization was confirmed by the parent before implementation. No commit,
push, PR, merge, tag, or release was performed.

## Completed tasks and persisted checkboxes

- T1.1–T1.7: Added lock, classifier, materialization, schema, doctor,
  migration, refresh, and hook regression tests. RED was confirmed with
  `python3 -m unittest tests.test_override_ownership` before implementation;
  failures were missing APIs/behavior rather than import or syntax errors.
- T2.1–T2.2: Implemented `[managed.*]` lock records and shared ownership
  classifier with normalized hashes and rendered-placeholder comparison.
- T3.1–T3.4: Wired policy-aware template sync, conservative metadata migration,
  doctor diagnostics, schema validation, and documented `rm <target> && ai-specs sync`
  refresh. Runtime hooks remain unconditional CLI rewrites.
- T4.1–T4.3: Updated worktree/Trello recipe docs and promoted canonical specs.
  `DocRef.condition` remains outside governance in v1: docs continue their
  existing copy behavior and are not classified or lock-recorded.
- T5.1–T5.3: Focused and full validation completed; all task lines are checked
  in `tasks.md`.

## Files changed

- `lib/_internal/lock.py`
- `lib/_internal/util.py`
- `lib/_internal/recipe-materialize.py`
- `lib/_internal/doctor.py`
- `lib/_internal/recipe_schema.py`
- `tests/test_override_ownership.py`
- `catalog/recipes/worktree-flow/README.md`
- `catalog/recipes/trello-mcp-workflow/README.md`
- `openspec/specs/sync-lock/spec.md`
- `openspec/specs/override-ownership/spec.md`
- `openspec/specs/recipe-schema/spec.md`
- `openspec/specs/worktree-flow/spec.md`
- `openspec/changes/override-ownership-lock-governance/tasks.md`
- `openspec/changes/override-ownership-lock-governance/apply-progress.md`

## TDD Cycle Evidence

| Task group | Test file / command | Safety net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|
| T1.1/T2.1 lock | `tests/test_lock.py`, `tests/test_override_ownership.py` | 197 baseline tests passed | Missing `set_managed_override`/managed section | Focused lock tests passed | Legacy sections, agents, and managed provenance cases | Consolidated lock serialization and normalized hashing |
| T1.2/T2.2 classifier | `tests/test_override_ownership.py` | util covered by baseline suite | Missing classifier API | 5 ownership states passed | Rendered bytes, CRLF normalization, and untracked migration cases | Pure helper in `util.py` |
| T1.3/T3.1 sync | `tests/test_recipe_materialize.py`, `tests/test_override_ownership.py` | materialize baseline passed | Missing `recipe_id`/governance behavior | Managed auto refresh, user preserve, migration, policy, hook tests passed | Existing catalog placeholder and compact-output regressions | Shared render/hash/record helpers |
| T1.4/T3.3 schema | `tests/test_recipe_schema.py`, governance tests | schema baseline passed | `TemplateRef.update_policy` absent | valid/default/invalid policies passed | all enum values and invalid diagnostic | Dataclass default + parser enum |
| T1.5/T3.2 doctor | `tests/test_doctor.py`, governance test | doctor baseline passed | classifier-aligned messaging absent | user-modified warning and auto stale silence passed | lock-backed vs untracked paths | Reused shared classifier |
| T1.6/T3.4 refresh | materialize stale/missing tests | materialize baseline passed | no managed reseed path | delete/missing seed and lock assertions passed | legacy pre-render migration path | Explicit documented refresh only; no optional force flag |

## Verification commands

- `python3 -m unittest tests.test_lock tests.test_override_ownership tests.test_recipe_materialize tests.test_recipe_schema tests.test_doctor` — passed after implementation.
- `./tests/run.sh` — final full run passed (`1275` tests).
- `./tests/validate.sh` — passed; includes Python compile, Bash syntax, and full unittest run (`1275` tests).

## Deviations and risks

- No `--refresh-managed` flag was added; the designed and documented explicit
  delete-plus-sync path is supported, while user-modified content remains safe.
- Existing pre-rendered placeholder copies are treated as a conservative
  migration match and recorded with their actual disk hash; subsequent sync can
  classify them as managed-stale and render the current bytes.
- `verify-report.md` and sync/archive artifacts are intentionally not created by
  apply; parent should run the verify/sync phases next.
