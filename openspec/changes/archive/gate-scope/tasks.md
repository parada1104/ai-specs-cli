# Tasks: topology-aware `gate_scope`

Depth: `domain_change` (explore → proposal → design → delta spec → tasks).

Plan refs: `proposal.md`, `design.md`, `specs/worktree-flow/spec.md`.

This artifact is the implementation plan only. No production code, generated
consumer output, or tests are to be changed while authoring it. Apply must use
strict RED → GREEN → TRIANGULATE/REFACTOR evidence because
`openspec/config.yaml` has `strict_tdd: true`.

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~950–1450 (implementation, hermetic fixtures/tests, migration diagnostics, docs, and generated-contract assertions) |
| 400-line budget risk | High |
| Chained PRs recommended | No |
| Suggested split | Single PR; the maintainer explicitly accepts the cohesive 950–1450 line review tradeoff and does not want chained PRs. |
| Delivery strategy | single-pr (authorized exception to the 800-line review budget) |
| Chain strategy | size-exception |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: size-exception
400-line budget risk: High

**Apply authorization:** The parent explicitly authorized one PR despite the
forecasted 950–1450 changed lines and 800-line review budget, because the
topology-aware gate change is cohesive. Do not split this change into chained
PRs.
### Suggested work units

| Unit | Goal | Finish boundary | Verification / rollback |
|------|------|-----------------|-------------------------|
| 1 | Declare and stamp `gate_scope`; preserve consumer ownership | Validated `auto|superrepo|subrepo`, stamped hook, stale-hook warning contract | Recipe/materialization tests green; revert recipe/materializer/docs slice without touching gate behavior |
| 2 | Prove repository ownership and apply central-path policy | Shared decision function handles structured and shell candidates | Hermetic superrepo/subrepo fixture matrix green; revert hook/test slice as one unit |
| 3 | Promote contract and document migration | Catalog/spec/skill/docs agree with runtime | Docs/content tests plus full validation green; revert prose/spec slice independently |

## Authorization gate

- **Status:** PENDING — planning is complete; production apply is not authorized
  by this file alone.
- **Required before Phase 1 RED:** parent/orchestrator records the apply
  authorization and confirms whether the forecasted chain is one PR or the
  feature-branch chain above. Auto-forecast may choose the split automatically,
  but it must not silently exceed the 800-line review budget.
- **Apply boundary:** do not modify production files, generated consumer
  outputs, or tests during this tasks phase. Once authorized, execute phases in
  dependency order and keep each RED failure, GREEN result, and final
  verification evidence in the apply/verify artifacts.
- **Tracker gate:** retain the Trello link below in the change artifacts before
  any production apply work.

## Tracker

- **Card:** [Trello #65 — follow-up worktree-flow gate scope for cross-repo superrepos](https://trello.com/c/LIoDU2xL/65-follow-up-worktree-flow-gate-scope-for-cross-repo-superrepos)
- **Board:** `69ec097f13e2d38ecd89a557`
- **Change:** `gate-scope`
- **Phase policy:** move/update the card at apply, verify, and archive transitions; do not claim completion from a task checkbox alone.

## Non-goals (apply MUST NOT)

- Change `gate_mode` values, `off`/`ask` precedence, remediation wording, or
  exact `WORKTREE_GATE_PROTECTED` token matching.
- Rename, remove, or reinterpret `repo_topology`; change worktree creation,
  cleanup enumeration, shared `.worktrees` layout, or submodule initialization.
- Treat `project.subrepos`, cwd, basename, or an uncorroborated
  `--show-superproject-working-tree` result as topology truth.
- Add nested/recursive submodule support, per-subrepo plan stores, a plan-copy or
  synchronization protocol, or a change-to-repository ownership matcher.
- Grant a broad superproject primary-worktree bypass, authorize subrepository
  production files, or make an active central `tasks.md` replace plan-build
  authorization.
- Extend the central exception beyond the canonical component-aware
  `<superrepo>/openspec/changes/**` subtree; `.gitmodules`, source trees, root
  configuration, release files, and prefix lookalikes remain protected.
- Change linked feature-worktree behavior; linked worktrees remain allowed before
  scope evaluation for all valid scope values.
- Silently overwrite customized or stale consumer hooks, automatically remove or
  migrate local planning folders, or move branches/worktrees/plans.
- Implement a general shell parser, OS sandbox, MCP interception, or unrelated
  hook coverage. Existing shell heuristics only receive the same scope decision
  as structured candidates.
- Hand-edit derived `ai-specs/recipes/worktree-flow/**` consumer output.
- Run formatters or tests while authoring this task artifact.

---

## Phase 0 — Apply preflight and fixture contract

**Files:** `tests/test_worktree_gate_hook.py`,
`tests/test_worktree_flow_recipe.py`, `tests/test_recipe_materialize.py`,
`tests/test_doctor.py`.

- [x] 0.1 Confirm the apply worktree/branch and authorization with the parent;
      record the selected PR split and tracker phase before writing code.
- [x] 0.2 Define one hermetic temporary-superproject builder reusable by recipe
      and gate tests. It must create a real `.git`, `.gitmodules`, an
      initialized local submodule with a distinct path/name, and (where needed)
      an uninitialized `-` status entry. It must also create the subrepo primary
      checkout and a linked subrepo feature worktree under the shared layout.
- [x] 0.3 Define fixture helpers for canonical central targets (active change,
      archive, nonexistent descendant, `changes-archive` prefix lookalike,
      symlink escape, and outside path) and shell events. Do not encode
      basename/cwd heuristics in the fixture itself.
- [x] 0.4 Record baseline exit/status and stderr expectations for existing
      path-mode and shell-mode gate tests before refactoring the decision path.

## Phase 1 — Recipe enum and materialization contract (RED → GREEN)

**Files:** `catalog/recipes/worktree-flow/recipe.toml`,
`lib/_internal/recipe-materialize.py`,
`tests/test_worktree_flow_recipe.py`,
`tests/test_recipe_materialize.py`, and, if the existing doctor surface is the
migration owner, `lib/_internal/doctor.py` plus `tests/test_doctor.py`.

**Requirements:** `gate_scope configuration`; `stamped and runtime-resolved
scope`; `existing materialized hook safety`.

- [x] 1.1 RED: extend recipe tests for a missing `gate_scope` resolving to
      `auto`, an empty value resolving to `auto`, and independent preservation
      of `gate_mode` and `repo_topology`.
- [x] 1.2 RED: add invalid-enum cases for `repository` and alternate spelling
      `super-repo`; assert non-zero sync/materialization, the invalid value in
      diagnostics, and the exact allowed set `auto | superrepo | subrepo`.
- [x] 1.3 RED: add materialization assertions for each effective value. A
      configured `superrepo` must replace `__WORKTREE_GATE_SCOPE__` in the
      generated hook, leave no placeholder, and remain runnable without reading
      the manifest or importing project Python at hook runtime.
- [x] 1.4 RED: add stamp/consumer migration cases: missing or invalid stamped
      scope is diagnosed; an existing catalog-owned or customized generated
      hook lacking the scope contract is not overwritten; sync/doctor emits a
      non-blocking warning naming the exact path and giving
      `rm <hook-path> && ai-specs sync` guidance. Assert customized bytes are
      unchanged and valid current hooks do not warn.
- [x] 1.5 GREEN: add `[config.gate_scope]` to `recipe.toml` with type `string`,
      optional field, default `auto`, exact enum, help text distinguishing it
      from `gate_mode` and `repo_topology`; bump the worktree-flow recipe version
      according to the catalog versioning convention.
- [x] 1.6 GREEN: extend `recipe-materialize.py` with a dedicated scope
      placeholder replacement using the merged validated config. Keep the hook
      self-contained; do not make it read `ai-specs.toml` or Python internals.
      Integrate stale-hook diagnostics with the existing ownership policy and
      preserve the non-destructive `not_exists` behavior.
- [x] 1.7 GREEN: wire the sync/doctor warning through the existing diagnostic
      owner only if the current hook materialization path requires a new check;
      keep warnings non-blocking and do not duplicate existing stale-template
      diagnostics.
- [x] 1.8 TRIANGULATE: run the focused recipe/materialization/doctor tests;
      verify old manifests without the key still materialize with `auto`, and
      verify `gate_mode=off` remains independently represented. Capture RED and
      GREEN evidence in the apply report.
- [x] 1.9 REFACTOR: centralize the allowed scope values/diagnostic shape where
      existing enum validation already lives; remove duplicate literals and
      confirm no alias or implicit normalization was introduced.

## Phase 2 — Runtime scope resolver and topology proof (RED → GREEN)

**Files:** `catalog/recipes/worktree-flow/hooks/worktree-gate.sh`,
`tests/test_worktree_gate_hook.py` (and a narrowly scoped helper test file only
if the implementation extracts a reusable pure helper).

**Requirements:** `proven repository ownership and topology scope`;
`topology-aware protected-branch decision`; `canonical superproject planning
boundary`.

- [x] 2.1 RED: add tests for runtime scope precedence: valid
      `WORKTREE_GATE_SCOPE` overrides the stamp; invalid override warns and uses
      the valid stamp; missing/invalid stamp warns and uses `auto`; empty
      override does not replace the stamp; `gate_mode=off` allows before scope,
      topology, or branch checks even with an invalid scope override.
- [x] 2.2 RED: add topology classification tests using the real fixture:
      proven initialized subrepo primary → `subrepo`; proven superproject
      primary → `superrepo`; uninitialized `-` module, similar names, nested or
      ambiguous registrations, symlink-escaping relationships, and unresolved
      Git facts → unproven with no scope-based allow. Include standalone and
      `monorepo-apps` cases that retain current protected-primary behavior.
- [x] 2.3 RED: add linked-worktree tests for a subrepo feature worktree where
      `git_dir != common_dir`; all scope values must allow it through the
      existing linked-worktree rule without requiring the central exception.
- [x] 2.4 RED: add protected-branch matrix tests for `auto`, `superrepo`, and
      `subrepo`: subrepo production writes block on `main`/`development` even
      when a central active plan exists; superrepo non-central writes block; a
      feature-like branch such as `main-feature` is not protected; configured
      branch tokens remain exact and slash-containing names are not globbed.
- [x] 2.5 RED: add canonical-boundary tests for active central artifacts,
      archive artifacts, and nonexistent descendants under the exact
      `<superrepo>/openspec/changes/**` subtree. Test all valid scope values.
      Add blocking cases for `changes-archive`, sibling/root/source/release
      paths, symlink escapes, outside paths, unrelated repositories, and paths
      that merely share a basename.
- [x] 2.6 GREEN: implement a self-contained scope resolver in the hook. Resolve
      `gate_mode` first; parse event/candidates; canonicalize the event cwd and
      each target via nearest existing ancestor with component-aware,
      symlink-safe containment; then resolve owning Git facts. Preserve existing
      malformed/unrelated-event fail-open behavior.
- [x] 2.7 GREEN: prove subrepo ownership only from the owning repository’s
      common Git directory plus a real containing superproject `.git`,
      `.gitmodules` registration, initialized `git submodule status` prefix
      (` `, `+`, or `U`), unique component containment, and matching Git-dir
      relationship. Treat `--show-superproject-working-tree` as corroboration,
      never sole proof. Refuse ambiguous, nested, escaped, or unresolved
      relationships.
- [x] 2.8 GREEN: classify a containing repository as the proven superrepo only
      after the same relationship proof. Apply the central exception solely when
      the canonical target is a descendant of the exact superrepo
      `openspec/changes` directory; do not broaden it based on scope value,
      cwd, basename, or active-plan presence.
- [x] 2.9 GREEN: preserve exact `WORKTREE_GATE_PROTECTED` matching, linked
      worktree allowance, existing generated-runtime allowlist, `always`/`ask`
      behavior, and conservative block behavior for all other protected primary
      paths. Keep plan-build authorization out of this function.
- [x] 2.10 TRIANGULATE: run the focused gate suite against structured path events
       and record every matrix result (exit `0` allow, exit `2` block, or
       documented fail-open). Confirm no test depends on `project.subrepos`.
- [x] 2.11 REFACTOR: split the resolver into small shell functions or embedded
       helpers with one shared final decision path; remove any duplicated
       path-mode/shell-mode branch logic and make diagnostics identify scope
       proof failures without leaking unrelated filesystem details.

## Phase 3 — Structured/shell parity and regression matrix (RED → GREEN)

**Files:** `catalog/recipes/worktree-flow/hooks/worktree-gate.sh`,
`tests/test_worktree_gate_hook.py`.

**Requirement:** MODIFIED `Shell Command Write-Bypass Detection` plus all
structured/shell parity scenarios in the delta spec.

- [x] 3.1 RED: add shell cases for protected non-central redirection (`>` and
      `>>`), `tee`/`tee -a`, `sed -i`/`perl -i`, `cp`/`mv`, and Python
      `Path.write_text`/`write_bytes` or `open(..., 'w'|'a'|'x')`; each must
      use the same topology-aware decision and block outside central planning.
- [x] 3.2 RED: add shell cases allowing central redirection and interpreter
      writes beneath active/nonexistent central change folders under each valid
      scope. Add linked-subrepo shell allow, read-only command allow, missing
      command/malformed or ambiguous command fail-open, and outside `/tmp`
      target fail-open cases.
- [x] 3.3 RED: add explicit shell `gate_mode=off`, invalid/valid scope override,
      protected-branch exactness, and ask-mode remediation parity tests. Keep
      existing path-mode assertions unchanged as regression guards.
- [x] 3.4 GREEN: route every confident shell candidate through the Phase 2 shared
      resolver; do not add a second topology heuristic or alter existing
      high-precision extraction/fail-open boundaries. Ensure top-level Cursor
      command/cwd payloads continue to use the same resolver.
- [x] 3.5 GREEN: preserve candidate-relative-to-event-cwd resolution, candidate
      deduplication, linked-worktree handling, and exit/stderr contracts while
      applying the central exception to shell candidates.
- [x] 3.6 TRIANGULATE: run the gate hook suite and compare structured versus
      equivalent shell events for every central, non-central, linked, protected,
      off, ask, malformed, and outside-path case. Any parser-only gap must remain
      fail-open rather than becoming a new bypass.
- [x] 3.7 REFACTOR: keep topology proof and canonical containment in one
      decision function; remove shell-specific exceptions that could allow a
      path blocked in structured mode.

## Phase 4 — Recipe/skill/docs and canonical contract promotion

**Files:** `catalog/recipes/worktree-flow/README.md`,
`catalog/recipes/worktree-flow/skills/worktree-flow/SKILL.md`,
`docs/recipes-catalog.md`, `docs/ai-specs-toml.md`, and
`openspec/specs/worktree-flow/spec.md` if the canonical spec is promoted in
this project’s normal archive flow; extend recipe/doc-content tests where
those files are already covered.

**Requirements:** configuration distinction, scope matrix, migration safety,
plan-build separation, and user-facing behavior.

- [x] 4.1 RED: add content assertions for the exact `gate_scope` enum/default,
      its orthogonality to `gate_mode` and `repo_topology`, runtime override
      precedence, protected-subrepo safety floor, canonical central boundary,
      linked-worktree allowance, and fail-safe ambiguity/symlink behavior.
- [x] 4.2 GREEN: document in the recipe README and catalog/TOML docs how
      `auto`, `superrepo`, and `subrepo` behave; state that central planning is
      the only superrepo exception and that production writes still require the
      plan-build gate.
- [x] 4.3 GREEN: update the worktree-flow skill with the which-repository check
      (`git rev-parse --show-toplevel` and relevant Git facts) before
      write-capable delegation, central-plan versus subrepo-code ownership, and
      stale-hook refresh guidance. Do not claim a hook is the sole safety guard.
- [x] 4.4 GREEN: reconcile the canonical `openspec/specs/worktree-flow/spec.md`
      with the accepted delta only if this repository’s normal change archive
      promotes the delta during apply; never weaken an existing requirement or
      duplicate the delta spec as a second conflicting contract.
- [x] 4.5 TRIANGULATE: run documentation/recipe tests and inspect materialized
      outputs from a fixture sync. Confirm derived consumer files are generated,
      not hand-edited, and stale customized bytes remain unchanged.
- [x] 4.6 REFACTOR: align terminology (`superrepo`, `subrepo`, `central
      planning subtree`, `proven relationship`) across recipe, hook diagnostics,
      skill, docs, and canonical spec.

## Phase 5 — End-to-end verification and evidence

- [x] 5.1 Run focused suites in dependency order: recipe/schema and
      materialization; doctor/migration diagnostics; worktree gate; then any
      affected recipe/catalog or sync-pipeline suites.
- [x] 5.2 Run `./tests/validate.sh` from the change worktree as the final code
      validation gate (py_compile, `bash -n`, and the project test runner).
      Record unavailable quality signals explicitly: coverage, linter,
      type-checker, and formatter are not configured according to
      `openspec/config.yaml`.
- [x] 5.3 Exercise a live temporary superproject with an initialized local
      submodule and linked subrepo worktree. Verify central planning allow,
      superrepo production block, subrepo production block, linked-worktree
      allow, and uninitialized/ambiguous fail-safe behavior. Treat this as
      smoke evidence in addition to hermetic tests, not as a replacement.
- [x] 5.4 Cross-check every ADDED and MODIFIED scenario in
      `specs/worktree-flow/spec.md` against a RED→GREEN test or an explicitly
      documented manual smoke. Confirm plan-build active-plan checks,
      worktree creation, cleanup enumeration, and `repo_topology` behavior are
      unchanged.
- [x] 5.5 Review the final diff for accidental generated-output edits, broad
      superrepo allowlists, branch globbing, manifest/runtime coupling, silent
      hook overwrite, and stale consumer migration regressions.

## Phase 6 — Archive and delivery

- [x] 6.1 After Phase 5 passes, update the Trello card to the verify/archive
      phase and record the verification evidence and any residual limitations.
- [x] 6.2 Create `openspec/changes/gate-scope/verify-report.md` with the exact
      commands/scenarios exercised, exit outcomes, scenario coverage map, and
      unavailable quality signals. Do not claim tests or formatters were run
      before apply authorization.
- [x] 6.3 Create `openspec/changes/gate-scope/archive-report.md` only after the
      review branch is ready. Confirm source recipe, canonical spec/docs,
      generated-contract expectations, and migration guidance are all included;
      warn before any destructive delta or consumer-hook removal.
- [x] 6.4 Archive the change folder using the repository’s normal OpenSpec
      archive workflow before merge; preserve the tracker link and reports, and
      do not merge/push or delete worktrees without the required human/parent
      instruction.

## Requirement → phase map

| Delta requirement | Kind | Phases |
|-------------------|------|--------|
| `gate_scope` configuration | ADDED | 1 |
| Stamped and runtime-resolved scope | ADDED | 1–2 |
| Proven repository ownership and topology scope | ADDED | 0, 2 |
| Topology-aware protected-branch decision | ADDED | 2–3 |
| Canonical superproject planning boundary | ADDED | 0, 2–3 |
| Scope and plan-build authorization remain separate | ADDED | 2, 4–5 |
| Existing materialized hook safety | ADDED | 1, 4–6 |
| Shell Command Write-Bypass Detection | MODIFIED | 3, 5 |

## Authorization checkpoint

**Status: PLANNING COMPLETE — apply NOT authorized by this artifact alone.**

Await parent/orchestrator authorization before Phase 1 RED. Once authorized,
follow the selected PR chain, preserve RED/GREEN evidence, run final validation,
and archive the change before merge.
