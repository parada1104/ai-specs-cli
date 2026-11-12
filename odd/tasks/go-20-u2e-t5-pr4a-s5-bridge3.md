# GO-20 U2e T5 PR4a S5-3 — retire the binding and merge-config Python bridge tests

Base: epic/go-single-binary 94b9ac1. Branch: change/go-20-u2e-t5-pr4a-s5-bridge3.
Goal: delete `tests/test_binding_bridge.py` and `tests/test_merge_config_bridge.py` without losing any
behavior coverage that survives `lib/` (PR4b) and the worktree-gate bridge flags (PR4b).

## Specs

- S1: Standing decision (Robert): the bridge flags exist only so `lib/` could call Go; after the `lib/`
  deletion they have no production consumer and are removed in PR4b. Every assertion in the two files
  therefore falls in exactly one class: **(a)** Python adapter/fallback behavior (dies with `lib/`),
  **(b)** bridge-flag transport contract — JSON envelope, exit codes, argv, timeout constant (dies with
  the flags), **(c)** underlying behavior that must survive (what the shared core or the native path
  computes) — and every (c) assertion must be covered by a Go test.
- S2: Every test method and every assertion of the two files is classified (a)/(b)/(c) in the mapping
  tables below; every (c) assertion cites the Go test that covers the same input→output
  (`file:line`, naming the case/assertion) or is marked UNCOVERED.
- S3: Every UNCOVERED (c) assertion gets a Go test against the shared core or the native `internal/`
  path, added test-first (RED observed, then GREEN with the value taken from the Python assertion). No
  new Go test calls `--resolve-bindings` or `--plan-merge-config`; those flags die in PR4b.
- S4: Baseline before deletion: both files run green with a built gate binary
  (`dist/worktree-gate-current`), rc and test count recorded, so the later drop is reconcilable.
- S5: The two files are deleted as an unstaged deletion (plain `rm`). No production code, no `lib/`
  file, no golden and no other Python test is touched.
- S6: `gofmt -l` on the touched Go dirs is empty, `go vet` on the touched packages rc=0, `go test` on
  the touched packages rc=0.
- S7: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 and `./tests/validate.sh` rc=0; the Python
  discover count drops by exactly the two files' 72 tests (2369 → 2297).
- S8 (behavior difference after `lib/` deletion, recorded explicitly): for typed `datetime`/`date`/
  `time` and non-finite float manifest values, the Python bridge fell back to its own merge and KEPT
  the typed value. The native path REJECTS them (an acquisition/serialization boundary). That Python
  fallback result dies with `lib/`; the surviving contract is the native rejection, which must be
  pinned (exact native error text and the fatal outcome). No Go code is added to accept those values.

## Mapping table

Classification: (a) Python adapter/fallback, (b) bridge-flag transport, (c) surviving behavior.
Baseline (L2): 72 test methods (28 + 44), 0 skipped, rc=0.

### tests/test_binding_bridge.py — 28 methods: 11 (a), 0 (b), 17 (c)

| # | method | assertion | class | surviving coverage |
|---|---|---|---|---|
| 1 | `BindingParityTests.test_explicit_binding_selects_the_named_recipe` | an explicit binding wins → `{"tracker": "recipe-b"}`; the `_forbid_python_authority` patch | result: (c); the forbid patch: (a) | `internal/sync/bindings_test.go:199` `TestNativeBindingsDifferentialAgainstFrozenOracle` case "explicit binding wins over two declarers"; `catalog/recipes/worktree-flow/gate/bindings_test.go:21` "explicit binding overrides auto-bind and clears the ambiguity" |
| 2 | `...test_auto_bind_single_provider` | one declarer of each of two capabilities auto-binds both | (c) | `bindings_test.go:199` "two capabilities auto-bind independently" and "single tracker provider auto-binds" |
| 3 | `...test_ambiguity_skips_the_capability` | two declarers → the capability is absent from bindings | (c) | `bindings_test.go:199` "ambiguous tracker is not bound" |
| 4 | `...test_duplicate_explicit_binding_is_a_fatal_error` | `duplicate explicit binding for capability 'tracker'`; `.code == "duplicate-explicit-binding"` | message: (c); `.code` attribute: (a) | `bindings_test.go:199` "duplicate explicit binding raises" (frozen error string); `gate/bindings_test.go:21` "duplicate explicit binding is a fatal conflict and a resolution error" |
| 5 | `...test_disabled_or_unknown_recipe_is_an_error` | `...references disabled/unknown recipe 'recipe-b'`; `.code` | message: (c); `.code`: (a) | `bindings_test.go:199` "disabled or unknown recipe raises"; `gate/bindings_test.go:21` same-named case |
| 6 | `...test_undeclared_capability_is_an_error` | `...references recipe 'recipe-b' which does not declare that capability`; `.code` | message: (c); `.code`: (a) | `bindings_test.go:199` "undeclared capability raises"; `gate/bindings_test.go:21` "explicit binding to a recipe that does not declare the capability is an error" |
| 7 | `...test_ambiguity_conflict_warning_names_every_provider` | conflict `("capability", "tracker", [recipe-a, recipe-b])`, severity `warning` | (c) | `bindings_test.go:199` conflicts for "ambiguous tracker is not bound"; `gate/bindings_test.go:21` "ambiguous capability is not bound and warns with every provider" |
| 8 | `...test_duplicate_binding_conflict_is_fatal` | one conflict, severity `fatal`, both recipes listed | (c) | `bindings_test.go:199` "duplicate explicit binding raises" (fatal conflict); `gate/bindings_test.go:21` "duplicate explicit binding is a fatal conflict…" |
| 9 | `...test_bridge_matches_the_retained_python_authority_case_by_case` | Go equals `_python_resolve_bindings` / `_python_check_capability_conflicts` over 7 cases | (a) | the 7 cases themselves are covered by rows 1–8; the parity leg dies with `lib/` |
| 10 | `WitnessBridgeTests.test_go_writes_the_bound_witness` | `state=bound`, `recipe_id=test-tracker-ledger`, `candidates=[]` | (c) | `internal/sync/bindings_test.go:312` `TestNativeResolveBindingsWritesWitnessStates` subtest "bound" |
| 11 | `...test_go_writes_the_ambiguous_witness_without_guessing` | `state=ambiguous`, sorted candidates, `recipe_id=""` | (c) | `:312` "ambiguous" (`[alpha, beta]`, empty recipe id) |
| 12 | `...test_go_writes_the_declared_not_bound_witness` | `state=declared-not-bound`, `recipe_id=""` | (c) | `:312` "declared-not-bound" |
| 13 | `...test_go_writes_the_unbound_witness_on_an_empty_repository` | `state=unbound`, `candidates=[]` | (c) | `:312` "unbound" |
| 14 | `...test_no_witness_is_persisted_when_resolution_fails` | a duplicate binding aborts resolution → no witness file; the `RuntimeError` itself | no-write: (c); raise: (a) | `bindings_test.go:453` `TestNativeResolveBindingsResolutionErrorNeverWrites` |
| 15 | `...test_go_witness_bytes_match_the_python_writer` | same document, same key order; the Python-writer comparison | key order/bytes: (c); Python parity: (a) | `bindings_test.go:250` `TestNativeWitnessBytesMatchFrozenWriter` (frozen bytes, sorted keys, all four states) |
| 16 | `...test_witness_written_at_is_utc_second_precision` | `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$` and the stamp is fresh (<300 s) | (c) | UNCOVERED in a surviving package (only `gate/witness_write_test.go:134`, a PR4b wrapper test) → closed in L3 by `gate/shared/witness_test.go` `TestWitnessWrittenAtIsUTCSecondPrecision` |
| 17 | `...test_read_only_resolution_never_writes_the_witness` | resolving from inside the repo without a write opt-in writes nothing | (c) | UNCOVERED (the no-repository no-op at `bindings_test.go:397` is a different branch) → closed in L3 by `TestNativeResolveBindingsWriteOptInBoundaries`/`no write opt-in` |
| 18 | `...test_write_opt_in_without_a_project_root_never_defaults_to_cwd` | a write request with no owning repository never writes into the process cwd | (a) — reclassified in L3: it pins the Python adapter's own guard | the shared core deliberately defaults the witness root to the process cwd for the flag ("The root defaults to cwd"), so the native authority has no equivalent request shape; every native write-capable call site passes its own root (see L3 finding) |
| 19 | `FailOpenFallbackTests.test_missing_binary_falls_back_to_python_with_one_warning` | one `GO_BINDINGS_BRIDGE_FALLBACK` line naming "no verified worktree-gate binary" | (a) | — |
| 20 | `...test_binary_that_fails_falls_back_with_one_warning` | one warning containing `exited 2` | (a) | — |
| 21 | `...test_unreadable_binary_output_falls_back_with_one_warning` | one warning | (a) | — |
| 22 | `...test_python_fallback_still_writes_the_bound_witness` | the Python writer persists the bound state | (a); the state contract is row 10 | — |
| 23 | `...test_python_fallback_still_writes_the_unbound_witness` | the Python writer persists the unbound state | (a); the state contract is row 13 | — |
| 24 | `...test_fallback_path_is_marked_temporary_in_the_source` | constant value + `TEMPORARY` docstring marker | (a) | — |
| 25 | `...test_verified_cache_candidate_requires_its_acquisition_receipt` | a cache candidate without a verification receipt is not selected; with it, selected | (c) | `internal/gatebinary/gatebinary_test.go:760` `TestResolveVerifiedBinary` ("No receipt: never selected" → receipt + committed digest selected) |
| 26 | `BindingAcquisitionTests.test_sync_binding_step_acquires_and_uses_the_go_authority` | the sync binding step acquires once (`gate_impl=auto`, `offline=false`) and uses the result | (a) | the Go authority is not called from the native pipeline yet (`internal/sync/bindings.go`: "S14 owns the sync-pipeline wiring"), so this Python call-site behavior has no native consumer |
| 27 | `...test_sync_binding_step_falls_back_offline_with_one_warning` | one fallback line, no duplicated acquisition warning | (a) | — |
| 28 | `...test_read_only_resolution_never_acquires` | the doctor shape resolves without acquiring | (a) | — |

UNCOVERED: 2 (rows 16–17), closed in L3 by two test additions (one shared-core format test, one native
no-write-opt-in test). Row 18 was reclassified (a) in L3 after the RED run proved the shared core
resolves the process cwd for an empty root by design.

### tests/test_merge_config_bridge.py — 44 methods: 11 (a), 4 (b), 29 (c)

| # | method | assertion | class | surviving coverage |
|---|---|---|---|---|
| 1 | `MergeConfigParityTests.test_defaults_false_zero_and_empty_still_apply` | `false`/`0`/`""` defaults are selected and overridable, in declaration order | (c) (the `_assert_parity` leg is (a)) | UNCOVERED: the frozen matrix only carries truthy defaults (`ratio=1.0`, `count=1`) → closed in L4 |
| 2 | `...test_defaults_apply_when_manifest_omits_them` | an absent manifest value keeps the default (`{"flag": false, "count": 0}`) | (c) | `internal/sync/mergeconfig_test.go:302` differential "defaults in field order with a manifest override"; the falsy branch is row 1 |
| 3 | `...test_missing_required_field_error` | `missing required config field 'board_id'` | (c) | `mergeconfig_test.go:302` differential "missing required field is a semantic error" (exact frozen message) |
| 4 | `...test_required_error_precedes_enum_error` | the required loop runs before the enum loop | (c) | UNCOVERED natively (only `gate/mergeconfig_test.go:TestMergeConfigRequiredPrecedesEnum`, a PR4b flag test) → closed in L4 |
| 5 | `...test_enum_error_lists_allowed_values` | `... value 'rust' is invalid; allowed: always \| ask \| off` | (c) | `mergeconfig_test.go:302` differential "enum violation is a semantic error" (exact frozen message) |
| 6 | `...test_gate_impl_enum_error_is_special` | `invalid gate_impl 'bash'; bash has been removed; allowed: auto \| go` | (c) | UNCOVERED natively → closed in L4 (fixture + real catalog) |
| 7 | `...test_gate_scope_falsy_and_whitespace_resolves_to_auto` | `""`, `"   "`, `False`, `0` → `auto` | (c) | the frozen matrix covers only `"   "` → UNCOVERED for `""`/`false`/`0` → closed in L4 |
| 8 | `...test_gate_scope_truthy_value_wins` | a non-blank override is kept | (c) | UNCOVERED natively → closed in L4 |
| 9 | `...test_gate_scope_not_declared_is_left_alone` | undeclared `gate_scope` is an unknown key: empty config + one warning | (c) | UNCOVERED natively → closed in L4 |
| 10 | `...test_unknown_keys_warn_in_manifest_order` | two unknown keys warn in manifest order, `board_id` kept | (c) | the frozen matrix has one unknown key only → order UNCOVERED → closed in L4 |
| 11 | `...test_warnings_are_emitted_before_a_semantic_error` | the unknown-key warning is emitted before the required-field error | (c) | UNCOVERED natively → closed in L4 |
| 12 | `...test_config_key_order_is_defaults_then_manifest` | defaults in field order, then manifest-new keys | (c) | `mergeconfig_test.go:317` `TestNativeMergeConfigFieldOrderIsRecipeDeclarationOrder` + the differential's ordered config (`orderedEqual` compares key order) |
| 13 | `...test_structured_reconcile_table_is_carried_through` | a valid `reconcile` table (incl. the `expectations` list) is carried through verbatim | (c) | the frozen "override keeps position…" carries a two-scalar table only → the declared list-carrying table was UNCOVERED → closed in L4 |
| 14 | `...test_structured_unknown_sub_key_error` | `recipe 'R': invalid config field 'reconcile': [config.reconcile]: unknown key 'bogus'` | (c) | the shared grader's message is pinned by `internal/config/recipeconfig_test.go:188` ("dotted unknown sub-key carries the shared grader's exact message"); the merge-config wrapper was UNCOVERED → closed in L4 |
| 15 | `...test_structured_nested_type_error` | `[config.reconcile].expectations[0].event: expected string, got int` | (c) | `mergeconfig_test.go:302` differential "nested structured shape order drives the error" (exact frozen message) |
| 16 | `...test_structured_bool_is_not_an_integer` | `expected integer, got bool` | (c) | UNCOVERED natively → closed in L4 |
| 17 | `...test_structured_list_over_the_cap` | `expected at most 32 entries, got 33` | (c) | UNCOVERED natively → closed in L4 |
| 18 | `...test_structured_non_table_value` | `expected table, got str` | (c) | UNCOVERED natively → closed in L4 |
| 19 | `...test_canonical_float_and_negative_zero_render_identically` | `0.1`, `-0.0`, `1e30` render as `0.1`, `-0.0`, `1e+30` in the enum error | (c) | UNCOVERED natively (`shared.pyString` has no sibling test) → closed in L4 |
| 20 | `...test_nested_repr_in_enum_error` | `value '{'x': [1, 'two']}'` (Python repr of a container) | (c) | UNCOVERED natively (`internal/config/repr_test.go:53` pins strings only) → closed in L4 |
| 21 | `...test_repr_parity_for_apostrophes_and_control_chars` | `["a'b"]`, `{'k': "o'clock"}`, `["a\x01b"]` are rejected (`assertIn("is invalid")`) | (c) | the string repr rule is pinned by `internal/config/repr_test.go:53`; the merge-config integration was UNCOVERED → closed in L4 with the exact Python-derived texts |
| 22 | `...test_real_worktree_flow_schema_defaults_and_overrides` | the shipped worktree-flow schema: `.worktrees`, `gate_impl=auto`, `auto_remove_merged=true` | (c) | UNCOVERED: no native test loads the real catalog recipe → closed in L5 |
| 23 | `...test_real_worktree_flow_schema_overrides_and_unknown_key` | overrides applied, `gate_scope="   "` → `auto`, one `mystery` warning | (c) | UNCOVERED → closed in L5 |
| 24 | `...test_real_worktree_flow_schema_rejects_invalid_gate_impl` | real schema + `bash` → the gate_impl message | (c) | UNCOVERED → closed in L5 |
| 25 | `...test_real_plan_build_schema_enum` | plan-build default `openspec`, `both` accepted, `vault` rejected | (c) | UNCOVERED → closed in L5 |
| 26 | `MergeConfigBridgeTransportTests.test_bridge_runs_the_pinned_binary_with_the_exact_flag_and_envelope` | argv `[binary, --plan-merge-config]`, request envelope shape/order, one warning emitted | argv/envelope transport: (b); envelope order: (c) | request-envelope parity: `mergeconfig_test.go:302` via `assertMergeConfigRequestParity` (frozen `envelope` per case) |
| 27 | `...test_valid_response_never_executes_the_python_fallback` | a valid response is returned verbatim, fallback forbidden | (a) | — |
| 28 | `...test_semantic_error_raises_without_falling_back` | a semantic error raises `RuntimeError` with no fallback line | (b) — the bridge's exit-0/error-envelope mapping | — |
| 29 | `MergeConfigHomeResolutionTests.test_binary_at_active_home_is_found_when_home_is_passed` | the `home` argument selects `<home>/cache` binary | (a) | — |
| 30 | `...test_without_home_the_active_home_binary_is_not_consulted` | no `home` → package-root resolution, never the active home | (a) | — |
| 31 | `DatetimeTransportBoundaryTests.test_datetime_manifest_value_falls_back_and_keeps_type` | Python keeps the typed `datetime` (fallback) | (a) | the surviving contract is the native REJECTION (S8): `mergeconfig_test.go:388` `TestNativeMergeConfigDatetimeIsABoundary` |
| 32 | `...test_date_and_time_manifest_values_fall_back_and_keep_type` | same for `date(2024,1,2)` and `time(3,4,5)` | (a) | `date` rejection: `mergeconfig_test.go:388` (frozen case `datetime 2024-01-02`); local `time` was UNCOVERED → closed in L6 |
| 33 | `...test_nested_datetime_falls_back_and_keeps_type` | a datetime nested in a table/list is kept | (a) | the nested rejection was UNCOVERED → closed in L6 |
| 34 | `...test_structured_string_field_supplied_datetime_stays_a_type_error` | Python raises `expected string, got datetime` | (a) | the surviving contract is the earlier native serialization rejection (S8), pinned by `mergeconfig_test.go:388` |
| 35 | `FailOpenFallbackTests.test_missing_binary_falls_back_with_one_warning` | one fallback line, "no verified worktree-gate binary" | (a) | — |
| 36 | `...test_nonzero_exit_falls_back_with_one_warning` | one line containing `exited 2` | (a) | — |
| 37 | `...test_non_json_output_falls_back_with_one_warning` | one line containing `not JSON` | (a) | — |
| 38 | `...test_malformed_envelope_falls_back_with_one_warning` | one line containing `merge-config envelope` | (a) | — |
| 39 | `...test_timeout_falls_back_with_one_warning` | the timeout constant bounds the call | (b) | — |
| 40 | `...test_nonfinite_float_serialization_falls_back_with_one_warning` | `inf` falls back and Python keeps `inf` | (a) | the surviving contract is the native REJECTION (S8): `mergeconfig_test.go:340` `TestNativeMergeConfigNonFiniteFloatIsRejected` |
| 41 | `...test_fallback_still_raises_semantic_errors` | transport degraded → one line, then the Python semantic error | (a) | — |
| 42 | `...test_unicode_decode_error_falls_back_with_one_warning` | a decode failure is a transport failure, not a crash | (b) | — |
| 43 | `...test_undecodable_go_output_falls_back_with_one_warning` | invalid UTF-8 stdout degrades to one line | (b) | — |
| 44 | `...test_fallback_path_is_marked_temporary_in_the_source` | constant value + `TEMPORARY` docstring marker | (a) | — |

UNCOVERED: 22 (rows 1, 4, 6, 7, 8, 9, 10, 11, 13, 14, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 32-time,
33), closed in L4–L6 by three test additions.

Totals: 72 methods = 22 (a) + 4 (b) + 46 (c); UNCOVERED (c) = 24, closed by five test additions (one
shared-core timestamp test, one native no-write-opt-in test, one native plan matrix with 25 cases,
one native real-catalog test with four subtests, one native time/nested-datetime boundary test).

## Behavior difference after `lib/` deletion (S8)

The Python bridge's `DatetimeTransportBoundaryTests` and the `inf` fallback case describe what the
Python authority did when the Go envelope refused a non-JSON value: it fell back and returned the
typed `datetime`/`date`/`time` object or the non-finite float. Those methods are (a): the value never
crossed the transport, and after `lib/` is deleted nothing performs that fallback. The native path
(`internal/sync.PlanMergeConfig` → `acquireManifestConfig`) REJECTS the same input as a fatal
serialization failure — the sync aborts instead of silently keeping an unserializable value. That
rejection is the surviving contract and is pinned by `mergeconfig_test.go:340`
(`TestNativeMergeConfigNonFiniteFloatIsRejected`, exact text
`manifest "ratio" value: Out of range float values are not JSON compliant`) and `:388`
(`TestNativeMergeConfigDatetimeIsABoundary`, exact text
`manifest "starts" value: Object of type datetime is not JSON serializable`), plus the local-`time` and
nested cases added in L6. No Go code is added to accept these values.

## PR4b deletion candidates (reported, not deleted)

- `catalog/recipes/worktree-flow/gate/bindings.go` + `witness_write.go`: thin wrappers whose only
  production caller is `runResolveBindings` (`--resolve-bindings`). With the flag gone,
  `bindings_test.go` and the non-flag half of `witness_write_test.go` in `package main` have no
  production subject left, because `gate/shared` has no test file of its own for `bindings.go`,
  `witness.go` or `mergeconfig.go` — the shared decision cores are covered only through
  `internal/sync/*_test.go` (which survives) and the gate wrappers (which do not).
- `catalog/recipes/worktree-flow/gate/mergeconfig_test.go`: entirely flag-driven
  (`run([]string{"--plan-merge-config"}, …)`).
- `catalog/recipes/worktree-flow/gate/bindings_test.go`: `TestRunResolveBindingsCommand`,
  `TestParseManifestBindings`' caller half and `witness_write_test.go`'s
  `TestRunResolveBindingsWritesWitness` are flag-driven; `TestGradeBindings` and the witness payload
  tests exercise only the wrapper.

## Tasks

| ID | Specs | Route | Commit |
|---|---|---|---|
| T1 | S1–S3 | inline | |
| T2 | S4, S5 | inline | |
| T3 | S6, S7 | inline | |

## Log

- L1 (Robert): "ok vamos con tu recomendación si, la idea es deshacernos de lo innecesario" —
  approving the same option as S5-1/S5-2: retire the Python bridge tests, verify the (c) coverage in
  Go, drop the bridge flags in PR4b.
- L2 — Baseline before deletion (S4): `dist/worktree-gate-current` built from the gate module
  (`scripts/build-gate.sh`, rc=0, `logs/01-build-gate.log`), then
  `python3 -m unittest tests.test_binding_bridge tests.test_merge_config_bridge -v` →
  `Ran 72 tests in 5.555s — OK`, rc=0, 0 skipped (`logs/02-baseline-two-files.log`, `.rc`). All 72
  methods ran against the real gate binary, so the baseline is a live bridge run, not a skip. Method
  inventory: 28 + 44 = 72, matching `Ran 72 tests`.
- L3 — Binding gaps closed test-first (S3), three deliberately wrong expectations carried into the RED
  run so each new test proves it bites:
  - `TestWitnessWrittenAtIsUTCSecondPrecision` (`gate/shared/witness_test.go`) — RED expected
    RFC3339Nano for all three cases; the Python cases are UTC second precision. `go -C
    catalog/recipes/worktree-flow/gate test ./shared/ -run TestWitnessWrittenAtIsUTCSecondPrecision
    -count=1` → 3 FAIL, rc=1 (`logs/03b-red-shared.log`). GREEN after replacing the expectations with
    the Python values (`-…T03:04:05Z`, offset → `T06:04:05Z`, sub-second truncated) → rc=0
    (`logs/04b-green-shared.log`).
  - `TestNativeResolveBindingsNoWriteOptIn` (`internal/sync/bindings_test.go`) — RED asserted a witness
    exists where the contract says none; GREEN asserts no witness and no warning inside a real repo.
  - Reclassification (row 18): the RED run of the ported
    `write_opt_in_without_a_project_root_never_defaults_to_cwd` case FAILED with "the native writer
    wrote a witness into the process cwd" — verified directly:
    `git -C "" rev-parse --path-format=absolute --git-common-dir` resolves the process cwd's
    repository (rc=0). The shared core documents that on purpose for the flag ("The root defaults to
    cwd", `gate/bindings.go:95`), so the Python assertion pinned only the adapter's guard. Finding
    reported, not "fixed" (production edits are out of scope): the native authority is safe today
    because `materialize.go:155`/`:196` pass their owned project root with the write opt-in while
    `resolvedconfigonly.go:70` and `briefview.go:179` pass no opt-in at all; a future caller that
    passes an empty root WITH the write opt-in would write into the cwd repository, and that guard no
    longer exists anywhere.
- L4 — merge-config plan matrix closed test-first: `TestNativeMergeConfigPlanMatrix`
  (`internal/sync/mergeconfig_test.go`) with 25 cases covering rows 1, 4, 6, 7, 8, 9, 10, 11, 13, 14,
  16, 17, 18, 19, 20, 21. RED carried three deliberately wrong expectations (the bool case expected
  `got float`, the worktree-flow default expected `worktrees`, and the plan-build config expected a
  spaced JSON object); observed actuals replaced them verbatim from the Python assertions (`got bool`,
  `.worktrees`, compact JSON). `go test ./internal/sync/ -run '…' -count=1` → rc=1
  (`logs/03-red-native.log`) then rc=0 (`logs/04-green-native.log`). The float/negative-zero/1e30
  cases confirm `1e+30`, `-0.0` and `0.1` render exactly as Python's repr; the container cases confirm
  `{'x': [1, 'two']}`, `["a'b"]`, `{'k': "o'clock"}` and `['a\x01b']`.
- L5 — real catalog schemas (rows 22–25): `TestNativeMergeConfigRealCatalogSchema` loads
  `../../catalog/recipes/worktree-flow/recipe.toml` and `…/plan-build-flow/recipe.toml` through
  `PlanMergeConfig` and pins the shipped defaults, the overrides with their single `mystery` warning,
  the removed-`bash` rejection, and plan-build's enum default/acceptance/rejection. RED: the
  worktree-flow case expected `worktrees` (actual `.worktrees`) and plan-build expected a spaced JSON
  object (actual compact); both corrected to the Python-asserted values.
- L6 — non-JSON boundary shapes (rows 32-time, 33): `TestNativeMergeConfigTimeAndNestedDatetimeAreBoundaries`
  pins the local-`time` and nested-datetime rejections with the native texts
  `manifest "starts" value: Object of type time is not JSON serializable` and
  `manifest "blob" value: Object of type datetime is not JSON serializable`. RED expected
  `Object of type datetime` for the time case; the observed native text was the `time` message. This
  closes the S8 behavior difference: where the Python bridge fell back and kept the typed value, the
  native path aborts the sync.
- L7 — Deletion (S5): plain `rm` of the two files, never `git rm`, so the deletions stay unstaged
  (`git status --short` → ` D tests/test_binding_bridge.py`, ` D tests/test_merge_config_bridge.py`).
  Method count of the deleted files (`grep -c "^    def test_"`): 28 + 44 = 72, matching the
  baseline's `Ran 72 tests`. No production file, no `lib/` file, no golden and no other Python test was
  touched.
- L8 — (S6) `gofmt -l internal/sync catalog/recipes/worktree-flow/gate/shared` → no output, rc=0
  (`logs/05-gofmt.log`); `go vet ./internal/sync/ ./internal/config/ ./internal/cli/ ./internal/doctor/`
  rc=0 (`logs/06-vet.log`) and `go -C catalog/recipes/worktree-flow/gate vet ./shared/` rc=0
  (`logs/06b`); `go test ./internal/sync/ ./internal/config/ -count=1` rc=0 (`logs/07`) and
  `go -C … test ./shared/ -count=1` rc=0 (`logs/07b`).
- L9 — (S7) `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 (`logs/08-build-cli.log`), then
  `./tests/validate.sh` rc=0 in 586 s (`logs/09-validate.log`): `gofmt` clean, gate module + root module
  Go suites green, `parity summary: gate-absent failing=0, gate-present failing=0 — PASS`, and the
  Python discover leg `Ran 2297 tests in 586.441s — OK (skipped=1, expected failures=3)`.
  Count reconciliation: base 2369 → 2297, a drop of exactly the 72 deleted methods (the single skip and
  the three expected failures are pre-existing and elsewhere; the pre-deletion baseline of the two files
  had 0 skips). `git status --short` shows only the two modified Go test files, the two deletions, the
  new shared test file and this doc — no scratch file or tool cache inside the worktree.
