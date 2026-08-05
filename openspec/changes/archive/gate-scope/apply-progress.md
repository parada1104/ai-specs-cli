# Apply Progress: topology-aware `gate_scope`

## Structured status consumed

```yaml
schemaName: spec-driven
changeName: gate-scope
artifactStore: openspec
planningHome:
  root: /Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/gate-scope/openspec
  changesDir: openspec/changes
changeRoot: openspec/changes/gate-scope
artifactPaths:
  proposal: [openspec/changes/gate-scope/proposal.md]
  specs: [openspec/changes/gate-scope/specs/worktree-flow/spec.md]
  design: [openspec/changes/gate-scope/design.md]
  tasks: [openspec/changes/gate-scope/tasks.md]
  applyProgress: [openspec/changes/gate-scope/apply-progress.md]
  verifyReport: [openspec/changes/gate-scope/verify-report.md]
  syncReport: []
contextFiles:
  proposal: [openspec/changes/gate-scope/proposal.md]
  specs: [openspec/changes/gate-scope/specs/worktree-flow/spec.md, openspec/specs/worktree-flow/spec.md]
  design: [openspec/changes/gate-scope/design.md]
  tasks: [openspec/changes/gate-scope/tasks.md]
  applyProgress: [openspec/changes/gate-scope/apply-progress.md]
  verifyReport: []
  syncReport: []
artifacts:
  proposal: done
  specs: done
  design: done
  tasks: partial
  applyProgress: done
  verifyReport: missing
  syncReport: missing
taskProgress:
  total: 58
  complete: 54
  remaining: 4
  unchecked: ["- [ ] 6.1 After Phase 5 passes, update the Trello card to the verify/archive", "- [ ] 6.2 Create `openspec/changes/gate-scope/verify-report.md` with the exact", "- [ ] 6.3 Create `openspec/changes/gate-scope/archive-report.md` only after the", "- [ ] 6.4 Archive the change folder using the repository’s normal OpenSpec"]
applyState: ready
dependencies:
  apply: all_done
  verify: ready
  sync: blocked
  archive: blocked
actionContext:
  mode: repo-local
  workspaceRoot: /Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/gate-scope
  allowedEditRoots: [/Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/gate-scope]
  warnings: ["One PR exception explicitly authorized by parent despite High review-budget risk; no chained PRs."]
nextRecommended: verify gate-scope change and prepare verify-report
```

## Authorization and workload

The parent explicitly selected a single PR and accepted the forecasted 950–1450
changed-line review tradeoff. `tasks.md` records `Decision needed before apply:
No`, `Chained PRs recommended: No`, `Chain strategy: size-exception`, and the
one-PR authorization. No commit, push, PR, merge, tag, or release was performed.

## Completed implementation

- Marked implementation tasks 0.1–5.5 complete in `tasks.md` as work finished.
- Added validated `gate_scope = auto | superrepo | subrepo` config with empty/missing
  default behavior and recipe version bump to 1.4.0.
- Added `__WORKTREE_GATE_SCOPE__` stamping and validated materialization. Runtime
  precedence is valid `WORKTREE_GATE_SCOPE` override, stamped scope, then warning
  fallback to `auto`; `gate_mode=off` exits before scope evaluation.
- Added non-destructive stale materialized worktree-hook warning with exact
  `rm <hook-path> && ai-specs sync` guidance and byte preservation. Duplicate
  warnings for the two shared hook registrations are suppressed per destination.
- Reworked the hook decision path so structured and shell candidates share
  canonicalization, Git ownership facts, linked-worktree allowance, exact branch
  matching, and canonical superrepo planning exception logic.
- Topology proof requires `.git`, `.gitmodules`, registered component-contained
  module path, initialized submodule status, and matching `.git/modules/<path>`
  common Git directory. Ambiguous/unproven relationships do not grant central
  access. Only `<superrepo>/openspec/changes/**` is excepted.
- Added hermetic initialized-submodule fixture coverage for central planning allow,
  superrepo production block, and subrepo production block across all three scope
  stamps, plus uninitialized module, symlink escape, ambiguous duplicate, and
  nested-registration fail-safe assertions.
- Scope now has enforcement semantics: `auto` gates proven superrepo and subrepo,
  `superrepo` gates only proven superrepo, and `subrepo` gates only proven
  subrepo (explicit Melón workflow behavior). The hook stamps and validates
  `repo_topology`; explicit `standalone`/`monorepo-apps` disables topology proof.
- Added Git 2.20-compatible `--git-common-dir` fallback anchoring relative
  output to the owning repository root rather than the hook process cwd.
- Promoted the accepted scope contract into the canonical worktree-flow spec and
  updated recipe README, skill, catalog docs, and TOML docs.

## Strict TDD evidence

| Cycle | Evidence |
|---|---|
| RED | New recipe tests initially failed for missing config default, missing stamp, and invalid enum; new gate test initially failed on missing scope stamp behavior. |
| GREEN | Added recipe schema/materializer and hook resolver; focused recipe suite passed 16 tests and gate suite passed 55 tests. |
| TRIANGULATE | Real temporary superproject with local initialized submodule exercised all-scope central/production matrix, explicit owner-scope enforcement, vendored topology disablement, uninitialized/ambiguous/nested/symlink fail-safe behavior, valid/invalid overrides, shell parity, and relative Git-common-dir fallback; structured and shell regression matrix passed. |
| REFACTOR | Shared resolver retained one final decision path; scope enum constants and stale warning deduplication were centralized. |

## Verification evidence

Commands run:

- `bash -n catalog/recipes/worktree-flow/hooks/worktree-gate.sh` — passed.
- `python3 tests/test_worktree_flow_recipe.py` — **16 passed**.
- `python3 tests/test_worktree_gate_hook.py` — **55 passed** (including explicit
  scope enforcement matrix, repo-topology stamp/classification, uninitialized,
  symlink escape, ambiguous/nested topology, shell parity, valid/invalid scope
  overrides, and relative Git-common-dir fallback cases).
- `bash tests/validate.sh` — py_compile and bash syntax checks passed; full
  unittest run executed 1306 tests and had one baseline failure in
  `test_trello_mcp_workflow_recipe.TrelloMcpWorkflowRecipeTests.test_tracking_declaration_matches_recipe_config`
  because the dogfood manifest remains `gate_mode=always` while
  `openspec/config.yaml` declares tracking `gate_mode=warn`. This unrelated
  consistency mismatch is intentionally not changed by gate-scope.

Unavailable quality signals per `openspec/config.yaml`: coverage, linter,
type-checker, and formatter are not configured. Formatters/linters were not run,
as instructed.

## Remaining tasks

- [ ] 6.1 Update Trello card to verify/archive phase (parent-owned transition).
- [ ] 6.2 Create `verify-report.md` with final scenario map (parent verify phase).
- [ ] 6.3 Create `archive-report.md` after review branch readiness.
- [ ] 6.4 Archive change folder only through the normal OpenSpec archive workflow.

## Files changed

- `catalog/recipes/worktree-flow/recipe.toml`
- `catalog/recipes/worktree-flow/hooks/worktree-gate.sh`
- `catalog/recipes/worktree-flow/skills/worktree-flow/SKILL.md`
- `catalog/recipes/worktree-flow/README.md`
- `lib/_internal/recipe-materialize.py`
- `tests/test_worktree_gate_hook.py`
- `tests/test_worktree_flow_recipe.py`
- `docs/recipes-catalog.md`
- `docs/ai-specs-toml.md`
- `openspec/specs/worktree-flow/spec.md`
- `openspec/changes/gate-scope/tasks.md`
- `openspec/changes/gate-scope/apply-progress.md`
