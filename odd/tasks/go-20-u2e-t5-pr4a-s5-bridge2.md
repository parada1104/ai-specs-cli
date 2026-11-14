# GO-20 U2e T5 PR4a S5-2 — retire four more Python bridge test files

Base: epic/go-single-binary c23dcb9. Branch: change/go-20-u2e-t5-pr4a-s5-bridge2.
Goal: delete `tests/test_reconcile_stamps_bridge.py`, `tests/test_lock_bridge.py`,
`tests/test_recipe_conflict_bridge.py` and `tests/test_copy_apply_bridge.py` without losing any
behavior coverage that survives `lib/` (PR4b) and the worktree-gate bridge flags (PR4b).

## Specs

- S1: Standing decision (Robert): the bridge flags exist only so `lib/` could call Go; after the `lib/`
  deletion they have no production consumer and are removed in PR4b. Every assertion in the four
  files therefore falls in exactly one class: **(a)** Python adapter/fallback behavior (dies with
  `lib/`), **(b)** bridge-flag transport contract — JSON envelope, exit codes, argv, timeout
  constant (dies with the flags), **(c)** underlying behavior that must survive (what the shared core
  or the native path computes) — and every (c) assertion must be covered by a Go test.
- S2: Every test method and every assertion of the four files is classified (a)/(b)/(c) in the
  mapping table below; every (c) assertion cites the Go test that covers the same input→output
  (`file:line`, naming the case/assertion) or is marked UNCOVERED.
- S3: Every UNCOVERED (c) assertion gets a Go test against the shared core or the native
  `internal/` path, added test-first (RED observed, then GREEN with the value taken from the Python
  assertion). No new Go test calls `--plan-reconcile-stamps`, `--write-lock`,
  `--resolve-primitive-conflicts` or `--apply-copy`; those flags die in PR4b.
- S4: Baseline before deletion: the four files run green with a built gate binary
  (`dist/worktree-gate-current`), rc and test count recorded, so the later drop is reconcilable.
- S5: The four files are deleted as an unstaged deletion (plain `rm`). No production code, no `lib/`
  file, no golden and no other Python test is touched.
- S6: `gofmt -l` on the touched Go dirs is empty, `go vet` on the touched packages rc=0, `go test` on
  the touched packages rc=0.
- S7: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 and `./tests/validate.sh` rc=0; the Python
  discover count drops by exactly the four files' 43 tests (2412 → 2369).

## Mapping table

Classification: (a) Python adapter/fallback, (b) bridge-flag transport, (c) surviving behavior.
Baseline (L2): 43 test methods, 0 skipped, rc=0.

### tests/test_reconcile_stamps_bridge.py — 6 methods: 3 (a), 1 (b), 2 (c)

| # | method | assertion | class | surviving coverage |
|---|---|---|---|---|
| 1 | `GoReconcileStampsAuthorityTests.test_go_authority_stamps_exactly_like_the_python_authority` | Go manifest equals the Python authority's manifest (parity of the two legs) | (a) | parity against `_stamp_recipe_reconcile_defaults_python`, dies with `lib/` |
| 1 | ″ | the stamped manifest equals `EXPECTED_CONFIGS`: stamp-full gets the declared `reconcile` table + `workflow="feature"`, `base_branch=false`, `protect_base=0` (defaults *selected* by the mapping), the absent `no_default_field` and the unreferenced `unused_field` stay out, and the pre-existing `unrelated="kept"` survives; stamp-min gets only `reconcile={max_age_seconds:3600}` | (c) | staging: `internal/sync/stamps_test.go:114` `TestPlanReconcileStampsAcquisition` — `:188` "used defaults survive and false zero absent and unused stay correct" (TOML false, 0, absent default and unused field); byte-exact plan `internal/sync/stamps_test.go:43` `TestReconcileStampsDifferentialAgainstPython` (frozen `testdata/stamps_golden.json`: false + 0 + expectations). Writing: `internal/sync/materialize_test.go:756` `TestMaterializeSpineStampManifestDifferentialFrozen` (frozen manifest bytes for added recipe / updated value / no-op / sorted-key order); a sibling key surviving a rewrite `catalog/recipes/worktree-flow/gate/recipeconfigwrite_test.go:85` `TestRecipeConfigWriteFlatReplace` (`max_age_seconds = 30   # freshness window` kept while `mode` is replaced) |
| 2 | `...test_envelope_keeps_enabled_order_and_silently_skips` | envelope entry order, the two stamps' content, `GO_RECONCILE_STAMPS_BRIDGE_TIMEOUT_SECONDS == 60` | transport/order: (b) envelope + timeout constant; order/skips: (c) | `internal/sync/stamps_test.go:195` "enabled order is preserved and repeated ids acquire once"; `:202` "skipped recipes leave an empty JSON list" (ghost, no-config, broken recipe.toml) |
| 3 | `...test_fully_skipped_catalog_writes_nothing_and_warns_nothing` | a catalog of only reconcile-less and broken recipes stamps nothing and warns nothing | (c) | composition: `PlanReconcileStamps` returns an empty plan (`internal/sync/stamps_test.go:202`) and `stampReconcileDefaults` (`internal/sync/materialize.go:393`) iterates it, so no writer runs and no warning can print; the untouched-manifest shape is pinned by the no-op case of `internal/sync/materialize_test.go:756` |
| 4 | `ReconcileStampsFallbackTests.test_unusable_go_authority_falls_back_with_one_warning` | 4 infra cases (missing, nonzero exit, non-JSON, wrong envelope) → one `GO_RECONCILE_STAMPS_BRIDGE_FALLBACK` warning + Python stamping | (a) | — |
| 5 | `...test_invalid_utf8_output_falls_back_with_one_warning` | invalid UTF-8 stdout → one warning + Python stamping | (a) | — |
| 6 | `ReconcileStampsWriterFailureTests.test_writer_failure_warns_verbatim_and_keeps_other_stamps` | a failing manifest write for ONE recipe warns once, verbatim (`  ! recipe '<name>': reconcile defaults not stamped (<Type>: <exc>)`), on both the Go and the Python path, and the other recipe is still stamped | (c) | UNCOVERED natively → closed in L3 by `internal/sync/stamps_test.go:219` `TestStampReconcileDefaultsWarnsPerRecipeAndContinues` (native twin: `warnReconcileStampFailure` at `internal/sync/materialize.go:417` + the `continue` at `:404`/`:409`) |

UNCOVERED: 1 (method 6's native warn-and-continue), closed in L3.

### tests/test_lock_bridge.py — 8 methods: 3 (a), 1 (b), 4 (c)

Native-vs-shared finding (L5): the retired bridge flag `--write-lock` wraps
`shared.WriteLockFile` (`catalog/recipes/worktree-flow/gate/shared/lockwrite.go:317`, called only from
`RunWriteLock` at `:387` and from `gate/lockwrite_test.go`). The **native** path is a different
implementation, `internal/lock.WriteLock` (`internal/lock/lock.go:720-726`), called from
`internal/sync/materialize_loop.go:158`. Every (c) row below is therefore mapped against
`internal/lock` tests, not against the shared emitter. `shared.WriteLockFile` has **no non-flag
caller**: it is a PR4b deletion candidate (reported, not deleted).

| # | method | assertion | class | surviving coverage |
|---|---|---|---|---|
| 1 | `GoLockWriteAuthorityTests.test_go_bridge_writes_byte_identical_lock_without_the_python_writer` | the bridge-written lock is byte-identical to the retained Python writer's lock (meta, managed, agents; legacy `[skills]`/`[recipes]` dropped) | bytes-vs-Python: (a); emitted document: (c) | `internal/lock/lock_test.go:284` `TestWriteLockDifferential` — `:293` "all sections" (frozen byte-exact `[meta]` → `[managed.*]` → `[deps.*]` → `[agents.*]` from `testdata/write_lock_golden.json`) |
| 1 | ″ | `GO_LOCK_WRITE_BRIDGE_TIMEOUT_SECONDS == 60` | (b) | — |
| 2 | `...test_go_bridge_drops_legacy_sections_through_the_dispatcher` | no `[skills.`/`[recipes.` survives a rewrite of a legacy lock, and the rest equals the reference bytes | (c) | `internal/lock/lock_test.go:480` `TestLoadLockWriteRoundTripDifferential` — the writer must drop `[recipes.`/`[skills.` (`:520`), round-trip bytes frozen; `:404` `TestLoadLockParse` keeps the legacy maps on read only |
| 3 | `...test_go_bridge_leaves_no_temp_files_behind` | no `.ai-specs.lock.*.tmp` remains after a write | (c) | `internal/lock/lock_test.go:599` "fresh write mode 0600 and no temp files" (`assertNoTempFiles`), `:663` "rename failure after temp creation cleans up", `:685` "success replaces bytes entirely" |
| 4 | `...test_go_bridge_rewrites_a_missing_parent_directory` | a missing parent dir is created and the lock written | (c) | `internal/lock/lock_test.go:633` "parent dirs auto-created" (`deep/nested/.ai-specs.lock` → header bytes) |
| 5 | `LockWriteFallbackTests.test_all_failures_fall_back_with_one_warning_and_identical_bytes` | 7 infra cases (missing binary, resolution raises, OSError, timeout, invalid UTF-8, non-JSON, envelope mismatch) → one `GO_LOCK_WRITE_BRIDGE_FALLBACK` warning naming the fault + Python-writer bytes | (a) | — |
| 6 | `...test_fallback_writer_reemits_deps_between_managed_and_agents` | D17: `[deps.*]` is re-emitted between `[managed.*]` and `[agents.*]`, dep ids sorted, rels sorted, raw shape `[deps."<dep>".skills."<skill>"]` | (c) | `internal/lock/lock_test.go:293` "all sections" (frozen bytes: managed → `[deps."dep1".skills."skill1"]` → agents) and `:299` "unsorted insertion order sorts" (`[deps."a-dep"...]` before `[deps."z-dep"...]`, `"a.md"` before `"z.md"`) |
| 7 | `...test_fallback_warning_matches_the_bridge_family_format` | the exact warning prefix/suffix | (a) | — |
| 8 | `LockWriteRefusalFailClosedTests.test_refusal_envelope_raises_and_never_falls_back` | exit-2 `{"error": …}` raises `RuntimeError`, no fallback, lock untouched | transport/fail-closed: (b); the refusal conditions themselves: (c) | `internal/lock/lock_test.go:300`+ (refusal cases: meta newline, managed control char, agents DEL, deps skill/hash newline, refusal walk order) and `:532` `TestControlCharRefusalOrder` (first-locator determinism; nothing written) |

UNCOVERED: none.

### tests/test_recipe_conflict_bridge.py — 13 methods: 7 (a), 3 (b), 3 (c)

| # | method | assertion | class | surviving coverage |
|---|---|---|---|---|
| 1 | `PrimitiveConflictGoAuthorityTests.test_go_grades_the_conflicts_and_never_reaches_python` | a stubbed envelope's conflicts map to `[(skill, skill-x, [Alpha, Beta], fatal)]` and Python is never reached | graded shape: (c); "never reaches Python" (the stub dispatch): (a) | `catalog/recipes/worktree-flow/gate/shared/primitiveconflicts_test.go:40` "shared skill id conflicts with recipe names" |
| 2 | `...test_bridge_invokes_resolve_primitive_conflicts_with_ordered_recipes` | argv `--resolve-primitive-conflicts --catalog-dir <dir> --recipe beta --recipe alpha` | (b) | — |
| 3 | `...test_empty_envelope_is_a_valid_clean_result_without_fallback` | `{"conflicts": []}` → `[]`, no fallback warning | (b) | — |
| 4 | `PrimitiveConflictParityTests.test_go_and_python_shapes_names_and_order_agree` | the shipped `tests/fixtures/recipes` catalog grades `EXPECTED_FIXTURE_SHAPE`: one fatal pair per kind with recipe **names** (not TOML ids), in skill → command → mcp order | (c) | per-kind + order: `shared/primitiveconflicts_test.go:40` (skill), `:48` (command), `:56` (mcp), `:103` "skill claims are graded before command claims", `:111` "command claims are graded before mcp claims", `:119` "recipes are sorted and deduplicated"; names-from-catalog: `catalog/recipes/worktree-flow/gate/primitiveconflicts_test.go:22` `TestLoadRecipePrimitives` (`dir-a` → id `alpha`, name `Alpha`). The exact six-recipe / three-conflict composition in one grade was UNCOVERED → one table case added at `shared/primitiveconflicts_test.go:137` (L4) |
| 5 | `...test_clean_catalog_returns_empty_from_go` | a single clean recipe grades `[]` | (c) | `shared/primitiveconflicts_test.go:32` "no overlapping claims produce no conflict"; `internal/sync/conflicts_core_test.go:246` `TestNativeConflictPrimitiveBoundary` (non-nil empty list, `[]` JSON) |
| 6 | `...test_invalid_recipe_toml_falls_back_and_still_raises` | Go exits 2 and Python's own `TOMLDecodeError` surfaces | Python raise: (a); the loader boundary Go keeps: (c) | `catalog/recipes/worktree-flow/gate/primitiveconflicts_test.go:123` `TestLoadRecipePrimitivesRejectsUnreadableRecipes`; `internal/sync/conflicts_core_test.go:246` (invalid recipe.toml propagates the loader error) |
| 7 | `...test_missing_recipe_dir_falls_back_and_raises_validation_error` | Go exits 2 for a missing recipe dir; Python raises `RecipeValidationError` | Python raise: (a); boundary: (c) | `internal/sync/conflicts_core_test.go:246` (`recipe directory not found`); `TestLoadRecipePrimitivesRejectsUnreadableRecipes` (missing recipe.toml) |
| 8 | `PrimitiveConflictFailOpenFallbackTests.test_missing_binary_falls_back_to_python_with_one_warning` | one warning, "temporary", "no verified worktree-gate binary", the graded shape | (a) | — |
| 9 | `...test_binary_that_fails_falls_back_with_one_warning` | one warning, `"exited 2"` | (a) | — |
| 10 | `...test_unreadable_binary_output_falls_back_with_one_warning` | one warning, `"was not JSON"` | (a) | — |
| 11 | `...test_malformed_envelope_falls_back_with_one_warning` | one warning, `"did not match the primitive-conflict envelope"` | (a) | — |
| 12 | `...test_envelope_item_without_type_defaults_to_capability` | an envelope item without `type` defaults to `capability` | (b) — the adapter's JSON leniency, not a shared-core rule | — |
| 13 | `...test_fallback_path_is_marked_temporary_in_the_source` | the constant's value and the `TEMPORARY` marker in the docstring | (a) | — |

UNCOVERED: 1 (the shipped fixture composition of method 4), closed in L4.

### tests/test_copy_apply_bridge.py — 16 methods: 5 (a), 5 (b), 6 (c)

| # | method | assertion | class | surviving coverage |
|---|---|---|---|---|
| 1 | `CopyApplyGoAuthorityTests.test_bundled_skill_go_path_matches_the_python_reference` | Go stdout == Python stdout (`    ✓ bundled skill my-skill`), stderr parity, tree identity (structure, content, mode, mtime), identical lock bytes | print: (c); tree identity: (c); lock bytes + Python parity: (a) | print: `internal/sync/materialize_loop_test.go:219` `TestMaterializeLoopDifferentialFrozen` via frozen `testdata/materialize_golden.json:165` (`    ✓ bundled skill demo`); tree identity: `catalog/recipes/worktree-flow/gate/copyapply_test.go:145` `TestApplyCopyBundledSkillReplacesDestWholesale` (`assertTreeIdentity`: content/mode/mtime, stale entries gone) + `shared/copyapply_test.go:14` `TestCopyFileStatPreservesModeAndMtime`, `:52` `TestCopyTreeDirModeParity`, `:82` `TestCopyTreePreservesSetgidDirBit` |
| 2 | `...test_command_go_path_warns_and_overwrites_like_the_reference` | `    ✓ command deploy`, the warn `recipe command 'deploy' overwrites existing managed command`, dest bytes replaced | (c) | `testdata/materialize_golden.json:165-166` (frozen `✓ command review` + the verbatim overwrite warning with the dest path) — the native call site is `internal/sync/materialize_loop.go:289-292`; decision: `gate/copyapply_test.go:223` `TestApplyCopyCommandDifferingDestSignalsOverwriteAndRewrites` |
| 3 | `...test_command_go_path_missing_dest_copies_without_warn` | fresh dest → confirmation printed, NO warning, source bytes copied | (c) | decision: `gate/copyapply_test.go:195` `TestApplyCopyCommandIdenticalDestIsNotOverwrite`; the native warn condition with an absent dest was UNCOVERED → closed in L4 by `internal/sync/copyapply_test.go:851` |
| 4 | `...test_command_go_path_identical_dest_does_not_warn` | identical dest → confirmation, no warning | (c) | decision: `gate/copyapply_test.go:195`; the native call-site silence was UNCOVERED → closed in L4 by `internal/sync/copyapply_test.go:871` |
| 5 | `...test_doc_go_path_matches_the_python_reference` | `    ✓ doc docs/guide.md`, stderr parity, dest bytes | (c) | `testdata/materialize_golden.json:201` (frozen `✓ doc guide/fresh.md` + the managed-current/user-modified skip lines); blind overwrite: `gate/copyapply_test.go:317` `TestApplyCopyDocIsABlindCopy` |
| 6 | `...test_source_missing_raises_the_exact_runtime_error_on_the_go_path` | exact `RuntimeError` per kind: `bundled skill not found: <src>`, `command source not found: <src>`, `doc source not found: <src>` | (c) | doc: `internal/sync/templatedocs_test.go:531` (`doc source not found: ` + path); bundled-skill and command messages were UNCOVERED at the native call site → closed in L4 by `internal/sync/copyapply_test.go:825` and `:838` |
| 7 | `...test_command_dest_directory_execution_fails_closed` | dest is a directory → the copy fails; the bridge fails CLOSED on the exit-2 envelope and no fallback copy runs | fail-closed transport: (b); decision data `overwrite=true` for a non-file dest: (c) | `gate/copyapply_test.go:257` `TestApplyCopyCommandDestDirectorySignalsOverwrite`; execution failure → error envelope `:447` `TestApplyCopyItemExecutionFailureExits2`; native error surface `internal/sync/copyapply_test.go:330` "execution failure returns an error" |
| 8 | `...test_envelope_contract_one_item_per_call_with_commands_dir` | argv `--apply-copy`, stdin `{"items":[{kind,id,src,dest,commands_dir}]}`, one `✓` line | (b) | — |
| 9 | `...test_timeout_matches_the_bridge_family` | `GO_COPY_APPLY_BRIDGE_TIMEOUT_SECONDS == 60` | (b) | — |
| 10 | `CopyApplyFallbackTests.test_all_failures_fall_back_with_one_warning_and_python_executes` | 8 infra cases (missing, resolution raises, OSError, timeout, invalid UTF-8, non-JSON, no-results, mismatch) → one warning naming the fault + the Python body copies | (a) | — |
| 11 | `...test_fallback_bundled_skill_rewrites_dest_wholesale` | the fallback rmtree+copytree drops a stale dest file and mirrors the source tree | (a) — the Python fallback body; the wholesale-replacement semantics of the surviving authority are (c) and covered by `gate/copyapply_test.go:145` | — |
| 12 | `...test_fallback_source_missing_raises_the_exact_runtime_error` | the Python body's own message | (a) | — |
| 13 | `...test_fallback_warning_matches_the_bridge_family_format` | the exact warning prefix/suffix | (a) | — |
| 14 | `...test_dep_skill_boundary_stays_python` | `materialize_dep_skill` still delegates to `vendor-skills.py` and never calls `go_apply_copy` | (a) — source inspection of the Python body | — |
| 15 | `CopyApplyRefusalFailClosedTests.test_refusal_envelope_raises_and_never_falls_back` | exit-2 `{"error": …}` → `RuntimeError("worktree-gate refused the copy: …")`, no fallback, dest untouched | (b) | — |
| 16 | `CopyApplyResultsCountMismatchTests.test_empty_results_for_one_item_is_an_envelope_mismatch` | `results: []` for a one-item plan → fallback + `"0 result(s) for 1 item(s)"` | (b) | — |

UNCOVERED: 2 groups (method 3+4's native warn condition, method 6's two native messages), closed in L4
by one native test with four subtests.

Totals: 43 methods = 18 (a) + 10 (b) + 15 (c); UNCOVERED (c) = 4, closed by three test additions
(one native stamp test, one native copy call-site test with four cases, one shared table case).

## Tasks

| ID | Specs | Route | Commit |
|---|---|---|---|
| T1 | S1–S3 | inline | |
| T2 | S4, S5 | inline | |
| T3 | S6, S7 | inline | |

## Log

- L1 (Robert): "ok vamos con tu recomendación si, la idea es deshacernos de lo innecesario" —
  approving the same option (1) as S5-1: retire the Python bridge tests, verify the (c) coverage in
  Go, drop the bridge flags in PR4b.
- L2 — Baseline before deletion (S4): `dist/worktree-gate-current` built from the gate module
  (`scripts/build-gate.sh`, rc=0, `logs/01-build-gate.log`), then
  `python3 -m unittest tests.test_reconcile_stamps_bridge tests.test_lock_bridge
  tests.test_recipe_conflict_bridge tests.test_copy_apply_bridge -v` → `Ran 43 tests in 5.312s — OK`,
  rc=0, 0 skipped (`logs/02-baseline-four-files.log`, `.rc`). All 43 methods ran against the real gate
  binary, so the baseline is a live bridge run, not a skip.
- L3 — (c) gap 1 closed test-first (S3): native writer-failure warn-and-continue.
  `TestStampReconcileDefaultsWarnsPerRecipeAndContinues` (`internal/sync/stamps_test.go:219`) drives
  `stampReconcileDefaults` with two planned stamps where beta's `reconcile` key already holds a
  multiline value (the shared writer's own refusal) while alpha is a plain first stamp. RED first with
  the expectation copied from the Python literal,
  `(OSError: disk full)`: `go test ./internal/sync/ -run
  'TestStampReconcileDefaultsWarnsPerRecipeAndContinues' -count=1` → FAIL printing the actual
  `  ! recipe 'Beta': reconcile defaults not stamped (RuntimeError: cannot replace multiline value for
  key 'reconcile')`, rc=1 (`logs/03-red-native.log`). The expectation was replaced with that native
  rendering; the rest of the Python assertion (the other stamp survives, the sync never fails) needed
  no change and did not report. GREEN: rc=0 (`logs/04-green-native.rc`). Finding: the native warning
  hardcodes `RuntimeError: <err>` instead of Python's exception type name — a recorded difference, not
  behavior anyone depends on after `lib/` dies.
- L4 — (c) gaps 2–4 closed test-first (S3), same RED-then-GREEN shape
  (`logs/03-red-native.log`, `logs/03b-red-primitive-composition.log`, rc=1 both):
  - `TestCopyApplyCallSites` (`internal/sync/copyapply_test.go:808`) — four cases: the two native
    missing-source messages (`bundled skill not found: `, `command source not found: `; the doc twin is
    already pinned at `templatedocs_test.go:531`) and the native command warn condition (fresh dest,
    identical dest). RED expectations were deliberately wrong (`bundled skill unknown:`, `command
    missing source:`, and an expected overwrite warning on both no-warn cases); the observed values
    match the Python assertions verbatim, and the expectations were replaced with them.
  - `TestCheckPrimitiveConflicts` gained the case
    "the shipped fixture catalog grades skill then command then mcp"
    (`shared/primitiveconflicts_test.go:137`) — the retired fixture's six recipes and its
    `EXPECTED_FIXTURE_SHAPE` verbatim. RED expected alphabetical kind order; the actual grading was
    exactly the Python shape (skill, command, mcp; names, not ids; always fatal), which replaced the
    expectation. GREEN: `go -C catalog/recipes/worktree-flow/gate test ./shared/ -run
    'TestCheckPrimitiveConflicts' -count=1` rc=0 (`logs/04b-green-primitive-composition.rc`).
  No production code changed, so nothing else could move.
- L5 — Lock finding (S1 special attention): the bridge flag `--write-lock` wraps
  `shared.WriteLockFile`; the native path uses `internal/lock.WriteLock`. All four (c) lock
  assertions map to `internal/lock/lock_test.go` (see the table), including the D17 deps placement,
  which is pinned there by the frozen byte-exact golden rather than by an index comparison.
  `shared.WriteLockFile` has no non-flag caller (`RunWriteLock` at
  `gate/shared/lockwrite.go:387` is the only production call site, `gate/lockwrite_test.go` the only
  test one) → PR4b deletion candidate, reported to the parent, not deleted here.
- L6 — Deletion (S5): plain `rm` of the four files, never `git rm`, so the deletions stay unstaged.
  Method count of the deleted files (`grep -c "^    def test_"`): 6 + 8 + 13 + 16 = 43, matching the
  baseline's `Ran 43 tests`.
- L7 — (S6) `gofmt -l` over the touched Go dirs → no output; `go vet` and `go test` on
  `./internal/sync/`, `./internal/lock/` and the gate `./shared/` package → rc=0 (logs 05–07).
- L8 — (S7) `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 (`logs/08-build.log`), then
  `./tests/validate.sh` rc=0 in 728s (`logs/09-validate.log`): `gofmt` clean, gate module + root module
  Go suites green, `parity summary: gate-absent failing=0, gate-present failing=0 — PASS`, and the
  Python discover leg `Ran 2369 tests in 727.970s — OK (skipped=1, expected failures=3)`.
  Count reconciliation: base 2412 → 2369, a drop of exactly the 43 deleted methods and the
  pre-deletion live baseline (the single skip is pre-existing and elsewhere; the baseline had 0
  skips). `git status --short` shows only the three test files, the four deletions and this doc — no
  scratch file or tool cache inside the worktree.
