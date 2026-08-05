# Tasks: Centralized planning artifacts across submodule worktrees

Depth: full

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 700–780 lines (additions + deletions; generated consumer output excluded) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 → fixture/RED tests, hook resolver, and gate flow; PR 2 → recipe/docs/version surfaces, canonical spec promotion, and final validation |
| Delivery strategy | ask-on-risk |
| Chain strategy | pending |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: pending
400-line budget risk: High

The estimate is expected to remain within the 800-line review budget, but the hook, integration fixture, test matrix, and canonical specification promotion exceed a focused 400-line review unit. If the implementation estimate rises above 800 lines, stop before apply and re-split the work or obtain an explicit size exception. Generated `ai-specs/recipes/plan-build-flow/**` output is derived and is not counted as a hand-edited implementation surface.

## Scope and execution contract

- Execute tasks strictly in the numbered order below; each task has a finish condition that is required before its dependants begin.
- Use strict TDD: RED tests must be written and run before production hook edits; record the failing assertion and the reason it demonstrates the missing behavior. Then implement GREEN, run the focused suite, and refactor only while green.
- Keep the central planning convention topology-derived. Do not add configuration, migration, synchronization, orchestration, or worktree lifecycle behavior.
- Work only in the dedicated `cross-repo-worktree-artifact-scope` worktree. Do not hand-edit materialized `ai-specs/recipes/plan-build-flow/**` files; regenerate them from catalog sources when verification requires it.
- Evidence for every task is the named command output and the observable assertions listed in that task. Apply/verify reporting must preserve RED, GREEN, focused, and full-validation evidence.

## Apply PR1 Checklist

- [x] 1.1 Build the real topology fixture
- [x] 1.2 Add topology and boundary RED tests
- [x] 1.3 Add RED recipe-surface checks (reserved for PR2)
- [x] 2.1 Implement canonical event-path normalization
- [x] 2.2 Implement fail-safe topology-aware central-root resolution
- [x] 3.1 Integrate the normative decision order
- [x] 3.2 Triangulate hook behavior against the existing contract
- [x] 4. Documentation, recipe surfaces, and version bump (PR2)
- [x] 5. Canonical spec promotion (PR2)
- [x] 6. Focused verification and full validation (PR2)

## 1. Fixture and RED tests

### 1.1 Build the real topology fixture

- **Files:** `tests/test_plan_build_gate_hook.py`
- Add a fixture helper (for example `_make_super_with_submodule`) that creates a temporary superproject, a committed local submodule, an initialized gitlink, and a linked submodule worktree under the supported shared layout. Configure local Git identity and use `git -c protocol.file.allow=always submodule add` for the local source repository.
- Reuse the existing `_git`, `_event`, and `_run` helpers. Preserve `_run` cleanup of `PLAN_BUILD_GATE_MODE` and `PLAN_BUILD_GATE_PATHS`; extend only with fixture-specific environment or cwd support when required.
- Add controls for a central active plan, archived-only plan, absent plan, non-existent target paths, symlink escapes, uninitialized submodules, two superprojects with similar submodule names, and an unrelated outside path. Blocking fixtures MUST NOT seed a local subrepository `openspec/changes/` plan.
- **Acceptance:** the fixture independently proves the submodule is initialized, the target is a linked worktree, and `git rev-parse --show-superproject-working-tree` is empty for the linked-worktree case; fixture setup creates no plan or branch side effects outside its temporary directory.
- **Verification:** `python3 -m unittest discover -s tests -p 'test_plan_build_gate_hook.py'` runs the existing suite before new assertions are enabled; capture the baseline result.
- **Dependency:** none. Finish before 1.2.

### 1.2 Add the topology and boundary scenario tests first

- **Files:** `tests/test_plan_build_gate_hook.py`
- Add tests named and mapped to design §9: `test_submodule_worktree_allows_production_with_central_plan`, `test_submodule_worktree_blocks_without_central_plan`, `test_submodule_worktree_blocks_with_archived_only_central_plan`, `test_submodule_worktree_allows_central_plan_creation`, `test_submodule_worktree_allows_central_archive_write`, `test_submodule_worktree_blocks_superproject_production_path`, `test_superproject_probe_empty_still_resolves_central`, `test_similar_submodule_names_do_not_select_wrong_parent`, `test_uninitialized_submodule_does_not_grant_production_access`, `test_non_submodule_worktree_uses_own_root`, `test_central_nonexistent_tasks_path_allowed`, `test_symlinked_central_path_cannot_escape`, `test_changes_lookalike_prefix_is_not_artifact_root`, `test_outside_repository_path_not_broadened`, and `test_gate_evaluation_creates_nothing`.
- Assert exit `0`/`2`, central-path diagnostics where required, archive exclusion, component-aware prefix behavior, inert `PLAN_BUILD_GATE_MODE`, and unchanged standalone behavior. Snapshot worktree list, branch set, and relevant directory state for the read-only test.
- **Acceptance:** each new test fails for the intended missing topology-aware behavior against the unmodified hook; failures MUST be assertion failures, not fixture, import, or Git setup errors. Existing standalone tests continue to run and retain their current expectations.
- **Verification:** run `python3 -m unittest discover -s tests -p 'test_plan_build_gate_hook.py'`; record the RED failure names and stderr/exit-code evidence. Do not proceed to hook implementation until meaningful RED evidence exists.
- **Dependency:** 1.1. Finish before 1.3.

### 1.3 Add RED recipe-surface checks needed by the contract

- **Files:** `tests/test_plan_build_flow_recipe.py`
- Extend the recipe tests for the planned `1.4.0` contract, central-root wording, no new configuration surface, vocabulary guards, and the unchanged hook/brief wiring. Keep existing artifact-store schema assertions intact; do not remove the existing `artifact_store_default` configuration merely because this change adds no new configuration.
- **Acceptance:** the new or updated assertions fail only because the catalog documentation/version/brief has not yet been changed, and they pin observable output rather than source formatting.
- **Verification:** run `python3 -m unittest discover -s tests -p 'test_plan_build_flow_recipe.py'`; record RED failures separately from topology-hook RED evidence.
- **Dependency:** 1.1. Finish before 2.1; may run after 1.2 in the same RED phase.

## 2. Hook normalization and central resolver

### 2.1 Implement canonical event-path normalization

- **File:** `catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh`
- Replace the separate path probes with one non-strict `python3` normalization step that resolves the event target relative to `cwd`, preserves non-existent destination components, resolves existing symlink ancestors/final targets, and finds the nearest existing probe directory for Git queries.
- Keep malformed JSON, missing `file_path`/`notebook_path`, unusable targets, and missing repository facts fail-open (`exit 0`). Keep the hook self-contained; do not add a helper dependency in `lib/_internal/util.py`.
- Add a repository-boundary-aware `is_under` helper using canonical component comparisons and parameterize the existing active-plan lookup so archive paths remain excluded.
- **Acceptance:** non-existent central plan paths retain their intended `openspec/changes/**` location; symlink escapes are evaluated at their resolved destination; prefix lookalikes such as `openspec/changes-archive` are outside the artifact root; malformed/unrelated events do not gain a central allowance.
- **Verification:** rerun the tests from 1.2. The normalization/boundary assertions may still fail, but no existing test may fail due to a syntax or parse error. Run `bash -n catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh` before moving to 2.2.
- **Dependency:** 1.2 and 1.3 RED evidence. Finish before 2.2.

### 2.2 Implement fail-safe topology-aware central-root resolution

- **File:** `catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh`
- Add a lazy `resolve_central_root` resolver, invoked only after the nearest-root production check has no active plan. Use `git -C <probe_dir> rev-parse --show-toplevel` as the first repository fact and use `--git-common-dir` as the primary absorbed-submodule signal.
- Derive and validate the candidate superproject from the git-common-dir `/modules/` relationship; verify a real superproject checkout, literal `.gitmodules` registration for the submodule name/path, initialized status via scoped `git submodule status`, and candidate/repository distinction. Retain `--show-superproject-working-tree` only as corroboration/fallback for supported legacy layouts, never as the sole signal.
- Reject ambiguous, similarly named, deinitialized, nested, or otherwise unproven relationships. On resolver failure, return no central candidate so the nearest-root gate remains the safe decision.
- **Acceptance:** linked submodule primary and linked worktree checkouts resolve to the actual containing superproject even when the superproject probe is empty; unrelated or similarly named repositories never contribute their plans; standalone and ordinary non-submodule worktrees resolve no central root; resolution performs no worktree, branch, plan, archive, or synchronization mutations.
- **Verification:** run the topology tests from 1.2 and inspect exit codes plus resolver diagnostics. Use the fixture's before/after topology snapshot for the read-only acceptance. Run `bash -n` again.
- **Dependency:** 2.1. Finish before 3.1.

## 3. Gate decision flow

### 3.1 Integrate the normative decision order

- **File:** `catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh`
- Implement the design §3 order without broadening existing scope: parse/fail-open, canonicalize target/probe, discover nearest repository, reject unrelated targets, allow agent config, allow nearest artifact paths, classify production paths, allow nearest active plans, lazily resolve central root, allow only central `openspec/changes/**`, then consult central active plans and block with a central-aware diagnostic.
- Preserve default production directories (`src`, `lib`, `catalog`) and the `PLAN_BUILD_GATE_PATHS` scope override. Keep `PLAN_BUILD_GATE_MODE` inert and expose no on/off/ask bypass.
- Make central artifact writes unconditional within the canonical `openspec/changes/**` boundary, including creation of missing plan files and archive preparation, while keeping superproject production paths and unrelated outside-repository paths gated by the ordinary decision.
- **Acceptance:** all tests from design §9 pass, including central active-plan allow, central absence/archived-only block, central creation/archive allow, central production block, standalone compatibility, symlink escape block/allow transition, lookalike-prefix rejection, outside-path safe handling, and no side effects. Central diagnostics identify the expected superproject planning location. No previously allowed standalone or local-plan case becomes blocked.
- **Verification:** run `python3 -m unittest discover -s tests -p 'test_plan_build_gate_hook.py'`; record GREEN output and the exact focused command. If any test fails, fix the decision at the source and rerun before documentation work.
- **Dependency:** 2.2. Finish before 3.2.

### 3.2 Triangulate hook behavior against the existing contract

- **Files:** `catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh`, `tests/test_plan_build_gate_hook.py`
- Review every existing hook test (standalone, archive, mode, custom production paths, agent config, malformed input, and outside Git) against the new resolver and gate order. Add only missing observable cases; do not change tests to accommodate an implementation shortcut.
- **Acceptance:** the hook remains non-bypassable, fail-open only for malformed/unassociated events, and monotonic for previously allowed artifact/local-plan writes. The script remains executable through the existing `bash` test/runtime contract.
- **Verification:** run `python3 -m unittest discover -s tests -p 'test_plan_build_gate_hook.py'` and `bash -n catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh`; preserve the all-green result as the prerequisite for PR 1 review or the next task group.
- **Dependency:** 3.1. Finish before 4.1.

## 4. Documentation, recipe surfaces, and version bump

### 4.1 Update the catalog recipe contract and user guidance

- **Files:** `catalog/recipes/plan-build-flow/README.md`, `catalog/recipes/plan-build-flow/skills/plan-build-flow/SKILL.md`, `catalog/recipes/plan-build-flow/recipe.toml`, `docs/recipes-catalog.md`
- Explain that recognized submodule code worktrees can have a different repository root from the single canonical superproject planning tree; state central active-plan lookup, central artifact-write boundaries, standalone compatibility, fail-safe unresolved topology, and the no-duplication/no-orchestration boundary.
- Update the recipe version from `1.3.0` to `1.4.0` in `recipe.toml`, the plan-build README enablement snippet, and the plan-build section of `docs/recipes-catalog.md`. Do not update unrelated recipe versions.
- If a brief rule is added, keep it short and vocabulary-clean: no new `[config.*]` table, `[sdd]`, decision matrix, `artifact_root`, per-subrepository store, or forbidden slash-command vocabulary. Preserve the existing artifact-store configuration and all existing workflow rules.
- **Acceptance:** prose names topology-derived central planning without prescribing a user-configured root; README/brief/catalog guards pass; version is consistently `1.4.0` at the four pinned locations (including the recipe test expectation); no `gentle-*` terms are introduced into guarded catalog surfaces.
- **Verification:** run the recipe tests from 1.3 and inspect the materialized recipe README/brief using the existing test harness. Do not hand-edit `ai-specs/recipes/plan-build-flow/**`.
- **Dependency:** 3.2. Finish before 4.2.

### 4.2 Update recipe contract assertions and derived-output checks

- **File:** `tests/test_plan_build_flow_recipe.py`
- Update only the plan-build version/documentation expectations and add assertions for central-root wording, absence of forbidden configuration, no hard worktree dependency, and unchanged classic-flow surfaces. Keep existing schema, materializer, artifact-store, and vocabulary tests meaningful.
- **Acceptance:** recipe materialization still produces the bundled skill and hook with the same matcher/blocking contract; generated consumer output is reproducible from catalog sources; no new user-facing configuration or removed contract is present.
- **Verification:** run `python3 -m unittest discover -s tests -p 'test_plan_build_flow_recipe.py'` and, after all sources are complete, `./tests/run.sh`.
- **Dependency:** 4.1. Finish before 5.1.

## 5. Canonical spec promotion

### 5.1 Promote the verified delta to the canonical capability spec

- **File:** `openspec/specs/plan-build-flow/spec.md`
- Promote the topology-aware artifact-root, robust submodule discovery, canonical normalization/symlink boundaries, centralized artifact convention, narrowly allowed central writes, read-only/no-orchestration behavior, modified pre-tool-use hook contract, and classic-flow coexistence requirements from `openspec/changes/cross-repo-worktree-artifact-scope/specs/plan-build-flow/spec.md`.
- Preserve the existing plan-build requirements and acceptance map. Use RFC 2119 language and Given/When/Then scenarios; retain explicit standalone, non-submodule, archived-only, unresolved-topology, no-mode-bypass, central-production-path, and no-new-configuration cases.
- **Acceptance:** every scenario in the change delta has an equivalent canonical requirement/scenario; the canonical spec forbids `[sdd]`, decision-matrix, `artifact_root`, per-subrepository store, synchronization, and topology/worktree lifecycle side effects; no unrelated capability spec is modified.
- **Verification:** compare the canonical spec against the delta section by section, then run the focused recipe and hook tests. Confirm the change delta remains available for review until implementation/verification and is not silently archived or deleted.
- **Dependency:** 4.2 and all GREEN hook behavior. Finish before 5.2.

### 5.2 Check source/derived/spec consistency

- **Files:** `openspec/changes/cross-repo-worktree-artifact-scope/specs/plan-build-flow/spec.md`, `openspec/specs/plan-build-flow/spec.md`, catalog recipe files, and derived `ai-specs/recipes/plan-build-flow/**` (verification only)
- Check that the canonical spec, hook behavior, recipe README/SKILL/brief, and generated files describe the same central-root and boundary contract. Resolve only actual contract inconsistencies; do not introduce implementation logic into generated files.
- **Acceptance:** no scenario is documented as allowed when the hook blocks it, no central allowance is described as a superproject-wide write bypass, and no documentation implies plan copying, branch/PR orchestration, or a new root selector.
- **Verification:** use the full focused suite in 6.1 after this consistency pass; record any intentional generated-file differences as materialization evidence.
- **Dependency:** 5.1. Finish before 6.1.

## 6. Focused verification and full validation

### 6.1 Run the complete focused contract matrix

- **Files under test:** `tests/test_plan_build_gate_hook.py`, `tests/test_plan_build_flow_recipe.py`, plus generated recipe output through the existing materializer tests
- Run the hook and recipe suites separately, then run all repository tests. Review the result against every row in design §9, including the existing standalone tests 1–11 and `test_no_new_config_surface`.
- **Commands/evidence:**
  1. `python3 -m unittest discover -s tests -p 'test_plan_build_gate_hook.py'` — all topology, boundary, gate, and legacy hook tests pass.
  2. `python3 -m unittest discover -s tests -p 'test_plan_build_flow_recipe.py'` — version, documentation, materialization, and vocabulary tests pass.
  3. `./tests/run.sh` — complete unittest discovery and the vendor smoke script pass.
- **Acceptance:** no focused or full-suite regression; RED/GREEN evidence is recorded with the test names and observed exit codes; generated consumer outputs are validated without hand edits.
- **Dependency:** 5.2. Finish before 6.2.

### 6.2 Run repository validation and close the evidence loop

- **Files:** no new implementation files; record evidence in the change's apply/verify reporting when those phases run.
- Run `./tests/validate.sh` (which covers Python compilation, Bash syntax checks, and `./tests/run.sh`). Inspect the final diff for the allowed file list and confirm no changes to `openspec/config.yaml`, worktree creation/cleanup, topology detection, `lib/_internal/util.py`, templates, tracker gates, memory behavior, or unrelated recipe versions.
- **Acceptance:** validation exits `0`; every design §9 test row is mapped to a passing test or an explicitly retained existing test; unavailable quality signals remain stated (no configured coverage, linter, type checker, or formatter); the final changed-line estimate is recalculated against the 800-line review budget.
- **Rollback evidence:** if validation or review exposes a regression, revert the source hook/docs/spec/test/version changes as one source-level rollback; no consumer migration, branch orchestration, artifact movement, or persisted state needs reversal.
- **Dependency:** 6.1. This is the final task and the completion gate for implementation.

## Non-goals and explicit exclusions

The implementation MUST NOT:

- add `[sdd]`, a decision matrix, an `artifact_root`/planning-root selector, or any new recipe configuration field;
- create, duplicate, migrate, synchronize, archive, or delete per-subrepository planning stores;
- create or clean worktrees, branches, pull requests, archive entries, or other repository state during gate evaluation;
- change `/worktree-new`, `/worktree-clean`, cleanup enumeration, topology detection, or the shared `.worktrees/<subrepo>-<slug>` layout;
- broaden the central allowance beyond the canonical superproject `openspec/changes/**` path or bypass production-directory gating;
- change tracker gates, memory behavior, classic SDD surfaces, cursor's advisory-only behavior, or the pre-merge archive contract;
- hand-edit derived `ai-specs/recipes/plan-build-flow/**` output or update unrelated recipe version pins.
