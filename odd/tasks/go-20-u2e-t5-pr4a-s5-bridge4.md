# GO-20 U2e T5 PR4a S5-4 — retire the last Python bridge tests

Base: epic/go-single-binary 1bfb7e7. Branch: change/go-20-u2e-t5-pr4a-s5-bridge4.
Goal: delete `tests/test_materialize_bridge.py`, `tests/test_hook_gate_bridge.py` and
`tests/test_template_actuator_bridge.py` without losing any behavior coverage that survives
`lib/` (PR4b) and the worktree-gate bridge flags (PR4b).

## Specs

- S1: Standing decision (Robert): the bridge flags (`--materialize-hook`, `--materialize-template`,
  `--plan-orphans`, `--apply-orphans`, `--write-lock`) exist only so `lib/` could call Go; after the
  `lib/` deletion they have no production consumer and are removed in PR4b. Every assertion in the
  three files therefore falls in exactly one class: **(a)** Python adapter/fallback behavior (dies
  with `lib/`), **(b)** bridge-flag transport contract — JSON envelope, exit codes, argv, timeout
  constant, bridge-side record validation, fail-open/fail-closed policy (dies with the flags),
  **(c)** underlying behavior that must survive (what the shared core or the native path computes) —
  and every (c) assertion must be covered by a Go test.
- S2: Every test method and every assertion of the three files is classified (a)/(b)/(c) in the
  mapping tables below; every (c) assertion cites the Go test that covers the same input→output
  (`file:line`, naming the case/assertion) or is marked UNCOVERED.
- S3: Every UNCOVERED (c) assertion gets a Go test against the shared core or the native `internal/`
  path, added test-first (RED observed, then GREEN with the value taken from the Python assertion). No
  new Go test calls `--materialize-hook`, `--materialize-template`, `--plan-orphans`,
  `--apply-orphans` or `--write-lock`; those flags die in PR4b. New coverage targets `internal/sync`
  (`MaterializeHook`, `MaterializeTemplate`, `CleanOrphans`, `materializeOrphanInputs`) only.
- S4: Baseline before deletion: all three files run green with a built gate binary
  (`dist/worktree-gate-current`), rc and test count recorded, so the later drop is reconcilable.
- S5: The three files are deleted as unstaged deletions (plain `rm`). No production code, no `lib/`
  file, no golden and no other Python test is touched.
- S6: `gofmt -l` on the touched Go dirs is empty, `go vet` on the touched packages rc=0, `go test` on
  the touched packages rc=0, also under `umask 077` for the focused `internal/sync` tests.
- S7: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 and `./tests/validate.sh` rc=0; the Python
  discover count drops by exactly the three files' 90 tests (2297 → 2207).
- S8: Lock assertions are mapped to the native writer `internal/lock` (`SetGateBaseline`,
  `SetManagedOverride`, `WriteLock`, `RemoveRecipeLockEntries`) plus the lock bytes the native
  materialize loop and orphan cleanup write — never to `shared.WriteLockFile`, whose only surviving
  consumer is the PR4b flag path.
- S9: `internal/sync` carries parallel tests; no test may pin a process-wide umask. Any fixture that
  needs exact permission bits chmods its own file explicitly.

## Mapping table

Classification: (a) Python adapter/fallback, (b) bridge-flag transport, (c) surviving behavior.
Baseline (L2): 90 test methods (25 + 33 + 32), 0 skipped, rc=0.

### tests/test_materialize_bridge.py — 25 methods: 7 (a), 12 (b), 6 (c)

| # | method | assertion | class | surviving coverage |
|---|---|---|---|---|
| 1 | `OrphanParityTests.test_matches_the_retained_python_authority_case_by_case` | 7 fixtures: recipe/deps/inproject orphan, nothing, all scopes + stale lock, empty dirs; Go equals the Python authority | the parity comparison: (a); the 7 outcomes: (c) | all-scopes + stale lock: `internal/sync/orphans_test.go:120` `TestCleanOrphansSuccessPrunesStaleLock`; nothing: `:581` differential case "nothing orphaned is a silent no-op"; empty dirs: UNCOVERED → closed in L3 by `TestCleanOrphansEmptyRootsIsASilentNoOp` |
| 2 | `...test_orphan_in_each_scope_is_removed_with_todays_messages` | three removed lines, exact scope prefixes, kept children | (c) | `orphans_test.go:120` (byte-exact stdout incl. `.recipe/`, `.deps/`, `ai-specs/.deps/`); `:581` case 1 |
| 3 | `...test_nothing_orphaned_is_a_silent_no_op` | empty stdout, tree unchanged | (c) | `orphans_test.go:581` subtest "nothing orphaned is a silent no-op" |
| 4 | `...test_stale_lock_entries_are_pruned_on_the_go_path` | `removed stale lock entries for recipe 'old'`, kept recipe never named, section gone from the lock | (c) | `orphans_test.go:120` (line + `lock.LockHeader` bytes), `:375` `TestCleanOrphansSuccessWithoutStaleEntriesLeavesLockUntouched`; writer: `internal/lock/lock_test.go:703` `TestMutationHelpers` "Remove helpers return presence" / `SetRecipeSkillHashes` |
| 5 | `...test_non_directory_children_are_never_listed_as_orphans` | a stray file child is never planned or printed and survives | (c) — the listing is acquisition | plan-side: `orphans_test.go:213` `TestCleanOrphansPreservesStrayFilesAndSymlinks` (stray preserved, symlink skipped); acquisition-side: UNCOVERED → closed in L3 by `TestMaterializeOrphanInputsListDirectoriesOnly` |
| 6 | `OrphanEnvelopeContractTests.test_go_envelope_round_trips_and_sorts` | plan sorted/deduped across four scopes, never null | plan semantics: (c); argv/JSON transport: (b) | `internal/sync/gateshared_import_test.go:24` `TestSharedOrphansPlanContract` (sort+dedupe, all four scopes), `:50` `TestSharedRunPlanOrphansEnvelope` (fields, never null) |
| 7 | `...test_bridge_invokes_plan_orphans_with_json_stdin` | argv `--plan-orphans`, stdin envelope shape | (b) | — |
| 8 | `OrphanFailOpenFallbackTests.test_missing_binary_falls_back_to_python_with_one_warning` | one `GO_ORPHANS_BRIDGE_FALLBACK` line, legacy removes | (a) | — |
| 9 | `...test_binary_that_fails_falls_back_with_one_warning` | `exited 2` | (a) | — |
| 10 | `...test_unreadable_binary_output_falls_back_with_one_warning` | one line | (a) | — |
| 11 | `...test_malformed_envelope_falls_back_with_one_warning` | one line | (a) | — |
| 12 | `...test_fallback_path_is_marked_temporary_in_the_source` | constant + `TEMPORARY` docstring | (a) | — |
| 13 | `OrphanApplyBridgeTests.test_apply_orphans_receives_plan_input_plus_roots` | apply stdin = plan input + three absolute roots | transport: (b); roots/sorted inputs: (c) | roots + sorted ids: UNCOVERED → closed in L3 by `TestMaterializeOrphanInputsListDirectoriesOnly` |
| 14 | `...test_applied_outcome_prints_todays_messages_and_prunes_lock` | success lines, no `'a'`, lock pruned | (c) | `orphans_test.go:120` (stdout + pruned lock bytes); writer `internal/lock/lock_test.go:703` |
| 15 | `...test_partial_outcome_fails_closed` | partial → no legacy rerun, no success line, one fail-closed line, lock not pruned | deletion/lock half: (c); fail-closed policy: (b) | `orphans_test.go:269` `TestCleanOrphansStopsAfterFirstFailureAndPreservesLock` (partial, no stdout, lock bytes preserved); shared core `orphans_apply.go:96` |
| 16 | `...test_malformed_apply_output_fails_closed` | garbage stdout → no deletion, fail-closed | (b) | — |
| 17 | `...test_applied_with_remaining_fails_closed` | applied + remaining is unusable → fail-closed, lock kept | (b) — the shared core cannot emit applied+remaining | — |
| 18 | `...test_applied_with_uncertain_entry_fails_closed` | applied + uncertain entry → fail-closed, lock kept | (b) | — |
| 19 | `...test_undecodable_apply_output_fails_closed` | invalid UTF-8 → fail-closed | (b) | — |
| 20 | `...test_exit_2_after_invocation_fails_closed` | exit 2 after the call → fail-closed | (b) | — |
| 21 | `...test_exit_0_with_non_applied_status_fails_closed` | non-applied status → fail-closed | (b) | — |
| 22 | `...test_clean_pre_apply_failed_falls_back_to_legacy_with_one_warning` | clean pre-apply → legacy deletes, one warning, no fail-closed line | core: (c); fallback policy: (a) | `orphans_test.go:330` `TestCleanOrphansPreApplyFailureSkipsDeletionAndLockPrune` (nothing deleted, lock not pruned), `:412` lock-read boundary |
| 23 | `...test_uncertain_pre_apply_failed_fails_closed` | uncertain pre-apply → fail-closed | (b) | — |
| 24 | `...test_spawn_failure_before_apply_falls_back_to_legacy_with_one_warning` | `FileNotFoundError` before apply → legacy, one warning | (a) | — |
| 25 | `...test_timeout_after_invocation_fails_closed` | timeout after invoking → fail-closed | (b) | — |

UNCOVERED: 3 (rows 1-empty-dirs, 5-acquisition, 13-roots), closed in L3 by two test additions.

### tests/test_hook_gate_bridge.py — 33 methods: 19 (a), 10 (b), 4 (c)

| # | method | assertion | class | surviving coverage |
|---|---|---|---|---|
| 1 | `HookGateGoAuthorityTests.test_fresh_write_go_path_matches_the_python_reference` | Go equals Python: bytes, mode 0755, line, identical lock bytes | parity leg: (a); write/mode/record/lock: (c) | `internal/sync/hookactuator_test.go:365` `TestMaterializeHookStateParity` h1 (bytes, `Mode: "755"`, record, message), `:699` `TestMaterializeHookFrozenDifferential` h1; lock bytes: `internal/sync/materialize_loop_test.go:219` `TestMaterializeLoopDifferentialFrozen` "runtime hook fresh collects the resolved entry" (whole-tree + lock compare) |
| 2 | `...test_user_modified_go_path_preserves_and_warns` | user edits preserved, exact `is user-modified` warning, no fallback | (c) | `hookactuator_test.go:365` h4 (exact warning + skip line + bytes), `materialize_loop_test.go:219` "runtime hook user-modified is preserved" |
| 3 | `...test_refresh_go_path_backs_up_and_rewrites` | refresh rewrites, prior bytes in the content-hash backup, lock baseline updated | (c) | `hookactuator_test.go:365` h6 (backup path + backup sha), `materialize_loop_test.go:219` "runtime hook --refresh-gates backs up and rewrites"; baseline writer `internal/lock/lock_test.go:735` "SetGateBaseline pins kind and policy" |
| 4 | `...test_envelope_contract_one_call_per_hook` | argv `--materialize-hook`, one call, exact request envelope, record lands in lock | (b) — argv/envelope mapping; native request mapping covered by `materialize_loop_test.go:219` hook scenarios | — |
| 5 | `...test_stub_refusal_envelope_fails_closed` | Go refusal → `RuntimeError`, no fallback, dest untouched | (b) — bridge mapping of a shared refusal; native refusal: `hookactuator_test.go:387` `TestMaterializeHookRefusalsAndBoundary` |
| 6 | `...test_timeout_matches_the_bridge_family` | timeout constant 60 | (b) | — |
| 7 | `HookGateFallbackTests.test_missing_binary_falls_back_with_one_warning` | one warning, Python writes dest + lock | (a) | — |
| 8 | `...test_garbage_stdout_falls_back_with_one_warning` | one warning | (a) | — |
| 9 | `...test_exit_2_without_error_envelope_falls_back` | `exited 2 without a hook envelope` | (a) | — |
| 10 | `...test_envelope_mismatch_falls_back` | `did not match the materialize-hook envelope` | (a) | — |
| 11–15 | `...test_record_{target,source,recipe,kind,sha…}_mismatch_falls_back*` (5 methods) | each bridge-side record field mismatch → one warning, Python records its own sha | (b) — the bridge's validation of the subprocess record; the native loop writes the shared actuator's own truthful record (`materialize_loop_test.go:219`) | — |
| 16 | `...test_wrote_true_without_record_falls_back` | `wrote without a record` | (b) | — |
| 17 | `...test_fallback_warning_matches_the_bridge_family_format` | warning prefix/suffix format | (a) | — |
| 18 | `...test_fallback_source_not_found_raises_the_exact_runtime_error` | `hook script not found: <path>` | (c) for the message; (a) for who raises it | `hookactuator_test.go:398` `TestMaterializeHookRefusalsAndBoundary` subtest "source missing" (exact message) |
| 19 | `...test_fallback_invalid_scope_raises_the_exact_runtime_error` | `invalid gate_scope 'bogus'; allowed: auto \| superrepo \| subrepo` | (c) | UNCOVERED in a surviving package (only `gate/hookgateactuator_test.go:267`, a PR4b flag test) → closed in L4 by a `TestMaterializeHookRefusalsAndBoundary` subtest |
| 20 | `...test_fallback_refuses_symlinked_destination` | identical refusal string, victim never created | (c) | `hookactuator_test.go:431` "symlink never written through" (both `refresh` values, exact refusal, target never written, link never replaced), `:365` h8/h9 |
| 21 | `...test_fallback_chmods_the_open_fd_not_the_path` | mode 0755 via fd, never path chmod | (a) — Python implementation detail; the surviving mode contract is `hookactuator_test.go:365` h1 `Mode: "755"` (native chmod on the open handle) | — |
| 22 | `...test_fallback_never_raises_on_bridge_infrastructure_failures` | exit 70 still degrades to the Python body | (a) | — |
| 23 | `HookGateRepairTests.test_wrote_true_null_record_with_written_dest_reconciles_the_lock` | Go wrote, `record: null` → lock records on-disk digest | (a/b) — bridge repair; the native loop always writes the baseline with the record (`materialize_loop.go:418`, `materialize_loop_test.go:219` hook scenarios) | — |
| 24 | `...test_record_sha_mismatching_disk_preserves_user_bytes_and_lock` | mismatching digest never applied | (b) — bridge-side disk verification of a subprocess record; native `MaterializeHook` returns the record for bytes it wrote | — |
| 25 | `...test_record_matching_disk_is_applied_as_before` | matching record applied, no fallback | (b) | — |
| 26 | `...test_malformed_stdout_after_go_write_reconciles_the_lock` | write then garbage stdout → lock reconciled | (a/b) — bridge-only repair (no subprocess in the native path) | — |
| 27 | `...test_crash_exit_70_after_go_write_reconciles_the_lock` | same for a crash | (a/b) | — |
| 28 | `...test_timeout_after_go_write_reconciles_the_lock` | same for a timeout | (a/b) | — |
| 29 | `...test_infra_failure_with_preexisting_user_gate_preserves_bytes_and_baseline` | user bytes + old baseline preserved across an infra failure | (c) — the preserve decision | `hookactuator_test.go:365` h4 (user-modified preserved, no rewrite, no warning drift), `materialize_loop_test.go:219` "runtime hook user-modified is preserved" (lock bytes unchanged) |
| 30 | `...test_infra_failure_leaving_unproven_changed_bytes_fails_closed` | unprovable changed bytes → RuntimeError, no baseline | (a/b) — bridge-only containment | — |
| 31 | `...test_fallback_refresh_refuses_dangling_symlink_destination` | refresh refuses a dangling link, victim not created | (c) | `hookactuator_test.go:431` (both refresh values) + `:365` h9 (refresh symlink guard) |
| 32 | `...test_fallback_refresh_refuses_symlink_to_existing_file` | same, victim bytes unchanged | (c) | `hookactuator_test.go:431` (both refresh values, link never replaced) |
| 33 | `...test_exactly_one_materialize_hook_script_definition_remains` | source has one dispatcher and one fallback body | (a) — a source assertion on `lib/` | — |

UNCOVERED: 1 (row 19), closed in L4 by one `MaterializeHook` subtest.

### tests/test_template_actuator_bridge.py — 32 methods: 22 (a), 5 (b), 5 (c)

| # | method | assertion | class | surviving coverage |
|---|---|---|---|---|
| 1 | `TemplateActuatorGoAuthorityTests.test_fresh_write_go_path_matches_the_python_reference` | Go equals Python: bytes, source mode copied, line, identical lock bytes | parity leg: (a); write/mode/record/lock: (c) | `internal/sync/templatedocs_test.go:448` `TestMaterializeStateParity` t1 (write + record), `:916` `TestMaterializeFrozenDifferential` t1; mode copy: `:996` `TestMaterializeTemplateGitDestParity` (`Mode` from source); lock bytes: `materialize_loop_test.go:219` "templates fresh managed-current and user-modified" |
| 2 | `...test_seed_rendered_copy_go_path_records_without_rewrite` | existing rendered copy → skip line + record of its own sha, no rewrite | (c) | `templatedocs_test.go:448` t2/t3 (skip + record of the on-disk bytes, `DiskSHA` unchanged), `:916` |
| 3 | `...test_managed_stale_auto_refresh_go_path` | auto refresh rewrites, info line, new baseline | (c) | `templatedocs_test.go:448` t6 (`info:refreshed managed template`, new bytes + record), `materialize_loop_test.go:219` template scenario |
| 4 | `...test_user_modified_refusal_go_path_preserves_and_skips` | `override user-modified` warning, bytes preserved, no fallback | (c) | `templatedocs_test.go:448` t8; `materialize_loop_test.go:219` "templates fresh managed-current and user-modified" |
| 5 | `...test_envelope_contract_one_call_per_template` | argv `--materialize-template`, exact request envelope, record in lock | (b) | — |
| 6 | `...test_invalid_policy_refusal_envelope_fails_closed` | refusal wraps `invalid update policy 'force'`, dest untouched, no lock entry | native message: (c); bridge mapping: (b) | `templatedocs_test.go:489` "template invalid policy" (exact message) |
| 7 | `...test_stub_refusal_envelope_fails_closed_without_fallback` | `worktree-gate refused the template: …`, no fallback, nothing written | (b) | — |
| 8 | `...test_timeout_matches_the_bridge_family` | timeout 60 | (b) | — |
| 9–12 | `TemplateActuatorFallbackTests.test_{missing_binary,garbage_stdout,exit_2_without_error_envelope,envelope_mismatch}_falls_back*` (4 methods) | one warning then the Python body | (a) | — |
| 13–17 | `...test_record_{target,source,sha_mismatch,sha_not_lowercase,wrote_without_record}*` (5 methods) | each record mismatch is unusable → fallback, no lock write from it | (b) — bridge-side record validation | — |
| 18 | `...test_fallback_refuses_escaping_target` | `template target ../outside/evil.sh escapes the project root…`, nothing outside | (c) | `templatedocs_test.go:500` "template escaping target" (exact message) |
| 19 | `...test_fallback_refuses_git_parent_directory_escape` | `.git/../../escape.sh` refused, `.git` never created | (c) | UNCOVERED in a surviving package (only `gate/templateactuator_test.go:888`, a PR4b flag test) → closed in L5 by a `TestMaterializeRefusalsAndBoundary` subtest |
| 20 | `...test_fallback_materializes_git_hook_target_inside_root` | `.git/hooks/example.sh` contained and materialized | (c) | `templatedocs_test.go:996` `TestMaterializeTemplateGitDestParity` "normal" (`.git/hooks/pre-commit` inside the primary repo) |
| 21 | `...test_fallback_materializes_linked_worktree_shared_hook` | linked worktree materializes into the main checkout's shared hooks dir | (c) | `templatedocs_test.go:996` "linked" (realpath-relative destination in the primary repo) |
| 22 | `...test_fallback_refuses_symlinked_ancestor` | `ancestor path of sub/evil.sh is a symlink…`, nothing through the link | (c) | `templatedocs_test.go:511` "template ancestor symlink" (exact message) |
| 23 | `...test_fallback_chmods_the_open_fd_not_the_path` | 0755 via fd | (a) — Python detail; surviving mode contract is `templatedocs_test.go:996` `Mode` and `:448` write cases | — |
| 24 | `...test_fallback_seeds_existing_rendered_copy_without_rewrite` | untracked seed arm: no rewrite, record on-disk sha | (a) — Python body; the surviving decision is (c) row 2 | — |
| 25 | `...test_fallback_managed_current_backfills_without_rewrite` | managed-current backfills provenance, no rewrite | (c) — the classification/backfill decision | `templatedocs_test.go:448` t5 (record backfilled, `DiskSHA` unchanged) |
| 26 | `...test_fallback_managed_stale_auto_refreshes` | stale auto refresh | (a) — Python body; (c) row 3 | — |
| 27 | `...test_fallback_managed_stale_confirm_preserves_with_warn` | `override managed-stale (confirm-required)`, bytes preserved | (c) | `templatedocs_test.go:448` t7 (exact warning + preserved bytes) |
| 28 | `...test_fallback_warning_matches_the_bridge_family_format` | warning format | (a) | — |
| 29 | `...test_fallback_source_not_found_raises_the_exact_runtime_error` | `template source not found: <path>` | (c) for the message | `templatedocs_test.go:478` "template source missing" (exact message) |
| 30 | `...test_fallback_invalid_policy_raises_the_exact_runtime_error` | `invalid update policy 'force' for template '<t>'; expected auto \| confirm \| never-force` | (c) for the message | `templatedocs_test.go:489` (exact message) |
| 31 | `...test_fallback_refuses_symlinked_destination` | both arms (`always` on a link to a file, `not_exists` on a dangling link) refuse with the exact refusal, victim untouched | (c) | UNCOVERED in a surviving package (only `gate/templateactuator_test.go:769`) → closed in L5 by a two-arm `TestMaterializeRefusalsAndBoundary` subtest |
| 32 | `...test_fallback_never_raises_on_bridge_infrastructure_failures` | exit 70 degrades to the Python body | (a) | — |

UNCOVERED: 2 (rows 19, 31), closed in L5 by two `MaterializeTemplate` subtests.

Totals: 90 methods = 48 (a) + 27 (b) + 15 (c); UNCOVERED (c) = 6, closed by five test additions
(two orphan-acquisition/empty-root tests, one hook gate_scope subtest, two template refusal
subtests).

## PR4b deletion candidates (reported, not deleted)

- `catalog/recipes/worktree-flow/gate/hookgateactuator_test.go`,
  `catalog/recipes/worktree-flow/gate/templateactuator_test.go`,
  `catalog/recipes/worktree-flow/gate/orphans_plan_test.go`,
  `catalog/recipes/worktree-flow/gate/orphans_apply_test.go` (and the remaining flag halves of
  `bindings_test.go`, `lockwrite_test.go`, `witness_write_test.go`, `mergeconfig_test.go`): every one
  of them is driven through `run([]string{"--materialize-hook" | "--materialize-template" |
  "--plan-orphans" | "--apply-orphans" | "--write-lock" | ...})`, so with the flags gone the file has
  no production subject left.
- `catalog/recipes/worktree-flow/gate/hookgateactuator.go`, `…/templateactuator.go`: thin wrappers
  whose only production caller is the flag dispatch in `main.go`.
- Coverage consequence: after PR4b the shared cores `shared.MaterializeHook`,
  `shared.MaterializeTemplate`, `shared.ApplyOrphans` and `shared.PlanOrphans` keep their surviving
  tests only through `internal/sync` (`hookactuator_test.go`, `templatedocs_test.go`,
  `orphans_test.go`, `gateshared_import_test.go`), which call them in-process. The gate-side
  `--plan-classify`-driven `classify_test.go` is unrelated to this change and stays.
- `tests/parity/parity.py:840` still names the deleted `tests/test_materialize_bridge.py`, and
  `_norm_bridge_fallback_notice` / `_norm_hook_bridge_reconcile_notice` normalize bridge lines that
  can no longer appear once `lib/` is gone. Reported, not edited (out of scope, `tests/parity/`).

## Tasks

| ID | Specs | Route | Commit |
|---|---|---|---|
| T1 | S1–S3 | inline | |
| T2 | S4, S5 | inline | |
| T3 | S6, S7 | inline | |

## Log

- L1 (Robert): "ok vamos con tu recomendación si, la idea es deshacernos de lo innecesario" —
  approving the same option as S5-1..S5-3: retire the Python bridge tests, verify the (c) coverage in
  Go, drop the bridge flags in PR4b.
- L2 — Baseline before deletion (S4): `scripts/build-gate.sh` rc=0 (4 targets, `logs/01-build-gate.log`),
  then `python3 -m unittest tests.test_materialize_bridge tests.test_hook_gate_bridge
  tests.test_template_actuator_bridge -v` → `Ran 90 tests in 23.888s — OK`, rc=0, 0 skipped
  (`logs/02-baseline-three-files.log`, `.rc`). Method inventory from `git show HEAD:<file> | grep -c
  "def test_"`: 25 + 33 + 32 = 90, matching `Ran 90 tests`; the Go-path classes ran against the real
  gate binary, so the baseline is a live bridge run, not a skip.
- L3 — Orphan gaps closed test-first (rows 1-empty-dirs, 5-acquisition, 13-roots), two additions to
  `internal/sync/orphans_test.go`:
  - `TestMaterializeOrphanInputsListDirectoriesOnly` (RED `logs/03-red-materialize.log`, rc=1): the
    wrong expectations were `RecipeSkills = [a b link stray.txt]` and `LockRecipes = [old a]`; the
    observed actuals are `[a b]` (a symlink child and a stray file are never listed) and `[a old]`
    (sorted). GREEN with the actual/Python-asserted values, `logs/04-green-materialize.log`, rc=0.
  - `TestCleanOrphansEmptyRootsIsASilentNoOp`: RED expected `pre_apply_failed`; the native authority
    returns `applied` with empty output and no lock file (same run and log), corrected in GREEN.
  - Correction: the first GREEN edit script also rewrote the status assertion of the pre-existing
    `TestCleanOrphansLockReadFailureIsCleanPreApply`, which the full-package run caught
    (`logs/07-test.log`: `status = "pre_apply_failed", want "applied"`). Restored to
    `shared.StatusPreApplyFailed`; package green afterwards.
- L4 — Hook gap closed test-first (row 19): `TestMaterializeHookRefusalsAndBoundary` subtest "invalid
  gate scope". RED expected `invalid gate_scope 'bogus'; allowed: auto | superrepo`; the shared
  renderer returns `… | subrepo` (Python's exact text), `logs/03b-red-hook.log` rc=1 → GREEN
  `logs/04b-green-hook.log` rc=0. The subtest also pins that the refusal fires before any write.
- L5 — Template gaps closed test-first (rows 19, 31): two `TestMaterializeRefusalsAndBoundary`
  subtests. "template destination symlink" covers both Python arms (`always` on a link to a regular
  file, `not_exists` on a dangling link): RED wanted a truncated refusal, the shared actuator returns
  `shared.TemplateSymlinkRefusal(target)` verbatim, victim bytes/absence preserved and the link never
  replaced (`logs/03c-red-template.log` rc=1). "template git parent escape" pins that
  `.git/../../escape.sh` is refused by `TemplateEscapingTargetRefusal` and that nothing is written
  outside the root and no `.git` tree appears; RED expected the wrong tail. GREEN
  `logs/04c-green-template.log` rc=0.
- L6 — Deletion (S5): plain `rm` of the three files. `git status --short` →
  ` D tests/test_hook_gate_bridge.py`, ` D tests/test_materialize_bridge.py`,
  ` D tests/test_template_actuator_bridge.py` (unstaged). No production file, no `lib/` file, no
  golden and no other Python test touched.
- L7 — (S6) `gofmt -l internal/sync/ internal/lock/ catalog/recipes/worktree-flow/gate/shared/` → no
  output, rc=0 (`logs/05-gofmt.log`); `go vet ./internal/sync/ ./internal/lock/` rc=0
  (`logs/06-vet.log`); `go test ./internal/sync/ ./internal/lock/ -count=1` rc=0 (`logs/07-test.log`,
  `internal/sync` 13.919 s); the focused `internal/sync` additions also pass under `umask 077`
  (`logs/07b-test-umask077.log` rc=0). No test sets a process umask (S9).
- L8 — (S7) `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 (`logs/08-build-cli.log`), then
  `./tests/validate.sh` rc=0 in ~579 s (`logs/09-validate.log`): `gofmt` clean, both Go modules green,
  `parity summary: gate-absent failing=0, gate-present failing=0 — PASS`, and the Python discover leg
  `Ran 2207 tests in 578.794s — OK (skipped=1, expected failures=3)`. Count reconciliation:
  2297 (base) − 90 (the three retired files) = 2207 exactly; the single skip and the three expected
  failures are pre-existing and elsewhere.
- L9 — Finding (reported, not edited): `tests/parity/parity.py:840` still names
  `tests/test_materialize_bridge.py`, and two normalizers (`_norm_bridge_fallback_notice` at :192,
  `_norm_hook_bridge_reconcile_notice` at :196) strip bridge lines that no longer exist. PR4b
  candidates are listed in the section above.
