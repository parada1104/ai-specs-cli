# GO-20 U2e T5 PR4a S5 — retire the four Python bridge test files

Base: epic/go-single-binary c952464. Branch: change/go-20-u2e-t5-pr4a-s5-bridge1.
Goal: delete `tests/test_resolved_config_bridge.py`, `tests/test_tag_conflict_bridge.py`,
`tests/test_classify_bridge.py` and `tests/test_recipe_config_write_bridge.py` without losing any
behavior coverage that survives `lib/` (PR4b) and the worktree-gate bridge flags (PR4b).

## Specs

- S1: Standing decision (Robert, option (1)): the four bridge flags exist only so `lib/` could call Go;
  after the `lib/` deletion they have no production consumer and are removed in PR4b. Therefore every
  assertion in the four Python files falls in exactly one class: **(a)** Python adapter/fallback
  behavior (dies with `lib/`), **(b)** bridge-flag transport contract — JSON envelope, exit codes,
  argv (dies with the flags), **(c)** underlying behavior that must survive (what the shared core or
  the native path computes) — and every (c) assertion must be covered by a Go test.
- S2: Every test method and every assertion of the four files is classified (a)/(b)/(c) in the mapping
  table below; every (c) assertion cites the Go test that covers the same input→output
  (`file:line`, naming the case/assertion) or is marked UNCOVERED.
- S3: Every UNCOVERED (c) assertion gets a Go test against the shared core or the native
  `internal/` path, added test-first (RED observed, then GREEN with the value taken from the Python
  assertion). No new Go test calls `--plan-resolved-config`, `--resolve-tag-conflicts`,
  `--plan-classify` or `--write-recipe-config`; those flags die in PR4b.
- S4: Baseline before deletion: the four files run green with a built gate binary
  (`dist/worktree-gate-current`), rc and test count recorded, so the later drop is reconcilable.
- S5: The four files are deleted as an unstaged deletion (plain `rm`). No production code, no `lib/`
  file, no golden and no other Python test is touched.
- S6: `gofmt -l` on the touched Go dirs is empty, `go vet` on the touched packages rc=0, `go test` on
  the touched packages rc=0.
- S7: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 and `./tests/validate.sh` rc=0; the Python
  discover count drops by exactly the four files' 35 tests (2345 → 2310).

## Mapping table

Classification: (a) Python adapter/fallback, (b) bridge-flag transport, (c) surviving behavior.
Baseline (L2): 35 test methods, 0 skipped, rc=0.

### tests/test_resolved_config_bridge.py — 5 methods: 3 (a), 1 (b), 1 (c)

| # | method | assertion | class | surviving coverage |
|---|---|---|---|---|
| 1 | `ResolvedConfigParityTests.test_matches_the_retained_python_authority_case_by_case` | `without_project_root(go) == without_project_root(py)`, all 5 fixtures | (a) | parity against `_python_build_resolved_config`, dies with `lib/` |
| 1 | ″ | `go_result["project_root"] == str(go_root.resolve())` (and the py twin) | (c) | `internal/sync/resolvedconfig_test.go` frozen `project_root` (`<PROJECT_ROOT>` token replayed with the `shared.RealPath` rule) in every differential case; `catalog/recipes/worktree-flow/gate/resolved_config_test.go:190` `TestBuildResolvedConfigEnvelope` (`project_root == RealPath(root)`) |
| 1 | ″ | fixture `flat-and-config-recipes`: flat keys kept minus `enabled`/`version`, `[recipes.<id>.config]` merged, disabled recipe still projected but absent from `enabled` | (c) | `internal/sync/resolvedconfig_test.go` frozen case **"flat and config recipes with enabled order and bindings"** — the same manifest verbatim; also `gate/resolved_config_test.go:19` `TestProjectRecipesFlatAndConfigStyle` and `:74` `TestProjectEnabledManifestOrder` |
| 1 | ″ | fixture `explicit-bindings`: `{capability: recipe}` map | (c) | frozen case **"explicit bindings with an incomplete pair dropped"**; `gate/resolved_config_test.go:88` `TestProjectBindingsExplicitOnly` |
| 1 | ″ | fixture `project-topology`: `[project].repo_topology` → `source=project`, `via=config` | (c) | frozen case **"project topology is the project source label"**; `gate/resolved_config_test.go:146` `TestResolveRepoTopologyConfiguredPassthrough`, `:122` `TestTopologyConfiguredPrecedence` |
| 1 | ″ | fixture `legacy-recipe-topology`: `recipes.worktree-flow.config.repo_topology` → `source=legacy-recipe` | (c) | frozen case **"legacy recipe config topology is the legacy-recipe label"**; `gate/resolved_config_test.go:122` (legacy branch) |
| 1 | ″ | fixture `empty-manifest`: empty bindings/recipes, `enabled == []`, topology auto standalone, `source=default` | (c) | frozen case **"empty manifest is auto standalone at the default label"** |
| 2 | `ResolvedConfigFallbackTests.test_missing_binary_falls_back_to_python_with_one_warning` | one `GO_RESOLVED_CONFIG_BRIDGE_FALLBACK` warning, "temporary", "no verified worktree-gate binary", result equals the Python projection | (a) | — |
| 3 | `...test_non_json_output_falls_back_with_one_warning` | one warning, Python projection returned | (a) | — |
| 4 | `...test_failing_binary_falls_back_with_one_warning` | one warning, `"exited 3"`, Python projection | (a) | — |
| 5 | `EnvelopeContractTests.test_go_envelope_carries_the_documented_types` | envelope key set `{bindings, recipes, enabled, project_root, topology}`, member types, topology key set `{resolved, configured, via, source, submodules, gitmodules_present}`, `submodules` list, `gitmodules_present` bool, empty stderr | (b) | the `--plan-resolved-config` stdout envelope. No native consumer emits this JSON: `grep` over `internal/cli/` and `cmd/` finds no resolved-config JSON encoder; the surviving authority returns Go structs |

UNCOVERED: none.

### tests/test_tag_conflict_bridge.py — 9 methods: 5 (a), 2 (b), 2 (c)

| # | method | assertion | class | surviving coverage |
|---|---|---|---|---|
| 1 | `TagConflictGoAuthorityTests.test_go_grades_the_conflicts_and_never_reaches_python` | stub binary prints an envelope; the Python authority is forbidden; mapped `[(tag, recipes, severity)]` | (b) | envelope transport of `--resolve-tag-conflicts` |
| 2 | `...test_bridge_invokes_resolve_tag_conflicts_with_ordered_recipes` | argv `--resolve-tag-conflicts --catalog-dir <dir> --recipe beta --recipe solo` | (b) | — |
| 3 | `TagConflictParityTests.test_go_and_python_shapes_order_and_severity_agree` | `_python_check_tag_conflicts(catalog, ENABLED)` equals `EXPECTED_SHAPE` | (a) | parity against the Python authority |
| 3 | ″ | the graded shape `EXPECTED_SHAPE` over the 7-recipe fixture: `vcs`→[alpha,beta] fatal, `flow`→[alpha,beta] fatal, `color`→[pair-x,pair-y] warning, `self-tag`→[dup,dup-peer] warning, in first-seen tag order | (c) | piecewise: `catalog/recipes/worktree-flow/gate/shared/tagconflicts_test.go:19` `TestCheckTagConflicts` — `:26` warning, `:31` one-sided `conflicts_with` fatal, `:56` a recipe listing the same tag twice is not a conflict; `:84` `TestCheckTagConflictsTagOrderIsFirstSeen` — first-seen tag order. The exact 7-recipe / 4-group composition was UNCOVERED → one case added to the `TestCheckTagConflicts` table (L3) |
| 4 | `...test_go_reads_the_toml_id_not_the_directory_name` | `[recipe].id` wins over the catalog directory name (`dir-only` → `real-id`; `shared`→[other,real-id] warning) | (c) | `gate/tagconflicts_test.go:22` `TestLoadRecipeTagMetadata` — `dir-a` → id `alpha`, `dir-b` → id `beta`, and the `no-id`/`no-recipe-table`/`missing` recipes contribute nothing |
| 5 | `TagConflictFailOpenFallbackTests.test_missing_binary_falls_back_to_python_with_one_warning` | one warning, "temporary", "no verified worktree-gate binary", `EXPECTED_SHAPE` | (a) | — |
| 6 | `...test_binary_that_fails_falls_back_with_one_warning` | one warning, `"exited 2"`, non-empty conflicts | (a) | — |
| 7 | `...test_unreadable_binary_output_falls_back_with_one_warning` | one warning, `"was not JSON"` | (a) | — |
| 8 | `...test_malformed_envelope_falls_back_with_one_warning` | one warning, `"did not match the tag-conflict envelope"` | (a) | — |
| 9 | `...test_fallback_path_is_marked_temporary_in_the_source` | the constant's value and the `TEMPORARY` marker in the fallback docstring | (a) | — |

UNCOVERED: 1 (the 7-recipe composition of method 3), closed in L3.

### tests/test_classify_bridge.py — 11 methods: 6 (a), 1 (b), 4 (c)

| # | method | assertion | class | surviving coverage |
|---|---|---|---|---|
| 1 | `ClassifyParityTests.test_each_ownership_state_round_trips_through_the_bridge` | 5 states through the bridge: `missing`, `untracked`, `user_modified`, `managed_current`, `managed_stale` | (c) | `catalog/recipes/worktree-flow/gate/shared/classify_test.go:15` `TestClassifyManagedOverrideSemantics` — `:26` missing, `:33` untracked, `:47` user_modified, `:62` managed_current, `:70` managed_stale (same inputs: `catalog` bytes, `sha256("catalog")`) |
| 2 | `...test_managed_state_uses_catalog_src_rendering_via_the_bridge` | a `Path` `catalog_src` is rendered into the would-write bytes, then classified `managed_current` | rendering: (a) — Jinja substitution in the Python adapter, dies with `lib/`; classification: (c) | `shared/classify_test.go:62` |
| 3 | `...test_missing_destination_wins_over_a_managed_entry` | absent dest + a managed entry → `missing` | (c) | `shared/classify_test.go:26` |
| 4 | `ClassifyEnvelopeContractTests.test_bridge_invokes_plan_classify_with_json_stdin` | argv `--plan-classify`, stdin `{dest, managed_entry, would_write}` | (b) | — |
| 5 | `...test_absent_would_write_is_sent_as_null` | absent would-write serializes as JSON `null`; state `managed_current` | transport: (b); state: (c) | `shared/classify_test.go:54` "nil would_write on a tracked match is managed_current" |
| 6 | `...test_bad_shape_envelope_falls_back_to_python` | a JSON object with `state: "not_a_state"` is not authority → one fallback warning, Python result | (a) | — |
| 7 | `ClassifyFailOpenFallbackTests.test_missing_binary_falls_back_to_python_with_one_warning` | one warning, "temporary", "no verified worktree-gate binary", `user_modified` | (a) | — |
| 8 | `...test_binary_that_fails_falls_back_with_one_warning` | one warning, `"exited 2"` | (a) | — |
| 9 | `...test_unreadable_binary_output_falls_back_with_one_warning` | one warning on non-JSON stdout | (a) | — |
| 10 | `...test_subprocess_failure_falls_back_with_one_warning` | one warning, `"did not run"` on `TimeoutExpired` | (a) | — |
| 11 | `...test_fallback_path_is_marked_temporary_in_the_source` | the constant's value and the `TEMPORARY` marker in the fallback docstring | (a) | — |

UNCOVERED: none.

### tests/test_recipe_config_write_bridge.py — 10 methods: 5 (a), 2 (b), 3 (c)

| # | method | assertion | class | surviving coverage |
|---|---|---|---|---|
| 1 | `GoConfigWriteAuthorityTests.test_go_bridge_applies_values_without_the_python_writer` | `default_list = "To Do"  # keep-me` → `default_list = "In Progress"  # keep-me`, the `[recipes.other]` block untouched, no error | (c) | `internal/config/recipeconfig_test.go:298` `TestUpdateRecipeConfig/"plain write replaces the value and keeps its comment"` (rewritten line keeps its comment, sibling line survives) |
| 1 | ″ | `stderr == ""` on the Go path | (b) | — |
| 1 | ″ | `GO_RECIPE_CONFIG_BRIDGE_TIMEOUT_SECONDS == 60` | (a) | — |
| 2 | `...test_go_bridge_noop_writes_nothing_without_the_python_writer` | writing the current value leaves the bytes untouched | (c) | `internal/config/recipeconfig_test.go:317` "equal value is a no-op: changed=false, bytes untouched" |
| 3 | `...test_go_bridge_refusal_fails_closed_with_the_exact_string` | `cannot combine a whole-table and a dotted update for key(s): reconcile`, manifest untouched; no fallback warning | refusal + bytes: (c); "no fallback" is (a) | `internal/config/recipeconfig_test.go:381` "whole-table and dotted update for one root are refused" (same refusal string, manifest unchanged) |
| 4 | `...test_empty_values_short_circuits_before_any_authority` | `update_recipe_config(values={})` returns before any authority, bytes untouched | (a) — the Python short-circuit; the core no-op is covered by `internal/config/recipeconfig_test.go:330` "empty values never touch the file (even a missing one)" | — |
| 5 | `ConfigWriteRefusalEnvelopeTests.test_exit_two_error_envelope_raises_without_fallback` | exit-2 `{"error": "boom refusal"}` → `RecipeConfigWriteError("boom refusal")`, manifest untouched, no fallback warning | (b) | — |
| 6 | `ConfigWriteFallbackTests.test_infrastructure_failures_fall_back_with_one_warning` | 8 infra cases (missing binary, resolution raises, OSError, timeout, invalid UTF-8, exit-2 without envelope, non-JSON, wrong envelope) → one fallback warning naming the fault, Python writer output | (a) | — |
| 7 | `...test_fallback_warning_matches_the_bridge_family_format` | the warning line's exact prefix/suffix | (a) | — |
| 8 | `...test_unserializable_values_fall_back_silently_to_the_python_typeerror` | `json.dumps` `TypeError` is not a bridge failure: no warning, the writer's own `toml_value` `TypeError` surfaces | (a) | — |
| 9 | `...test_real_gate_hang_hits_the_bridge_timeout_and_falls_back` | a hanging gate is killed at the patched 1s timeout, one warning, Python writer applies | (a) | — |
| 10 | `...test_fallback_bytes_equal_the_pure_python_writer` | fallback bytes equal `_update_recipe_config_python`'s bytes | (a) | — |

UNCOVERED: none.

Totals: 35 methods = 19 (a) + 6 (b) + 10 (c); UNCOVERED (c) = 1, closed by one table case in
`catalog/recipes/worktree-flow/gate/shared/tagconflicts_test.go`.

## Tasks

| ID | Specs | Route | Commit |
|---|---|---|---|
| T1 | S1–S3 | inline | |
| T2 | S4, S5 | inline | |
| T3 | S6, S7 | inline | |

## Log

- L1 (Robert): "ok vamos con tu recomendación si, la idea es deshacernos de lo innecesario" —
  approving option (1): retire the Python bridge tests, verifying the (c) coverage in Go, and drop the
  bridge flags in PR4b.
- L2 — Baseline before deletion (S4): `dist/worktree-gate-current` built from the gate module
  (`scripts/build-gate.sh`, rc=0), then
  `python3 -m unittest tests.test_resolved_config_bridge tests.test_tag_conflict_bridge
  tests.test_classify_bridge tests.test_recipe_config_write_bridge -v` →
  `Ran 35 tests in 4.472s — OK`, rc=0, 0 skipped (`logs/02-baseline-four-files.log`,
  `logs/02-baseline-four-files.rc`). All 35 methods ran against the real gate binary, so the baseline
  is a live bridge run, not a skip.
- L3 — (c) gap closed test-first (S3), the only UNCOVERED item in the table: `TestCheckTagConflicts`
  gained the case `four tag groups keep first-seen order and mixed severities` — the retired Python
  fixture verbatim (alpha `vcs,flow` `conflicts_with=beta`, beta, pair-x/pair-y `color`, solo
  `solo-tag`, dup/dup-peer `self-tag` twice each). RED first, with the expectation deliberately written
  as alphabetical tag order:
  `go -C catalog/recipes/worktree-flow/gate test ./shared/ -run 'TestCheckTagConflicts$' -count=1` →
  `--- FAIL: TestCheckTagConflicts/four_tag_groups_keep_first-seen_order_and_mixed_severities`,
  rc=1 (`logs/03-red-tag-composition.log`). The failure message printed the actual grading —
  `vcs` fatal, `flow` fatal, `color` warning, `self-tag` warning — which is exactly the Python
  `EXPECTED_SHAPE`; the expectation was then replaced with that value (not with anything inferred from
  the Go code). GREEN: same command plus `-v` over `TestCheckTagConflicts|TestTagConflict` →
  `--- PASS` for every case, `ok ai-specs.dev/worktree-gate/shared`, rc=0
  (`logs/04-green-tag-composition.log`). No production code changed, so nothing else could move.
- L4 — Deletion (S5): plain `rm` of the four files, never `git rm`, so the deletions stay unstaged.
  `git status --short` shows exactly: `M catalog/recipes/worktree-flow/gate/shared/tagconflicts_test.go`,
  `D tests/test_classify_bridge.py`, `D tests/test_recipe_config_write_bridge.py`,
  `D tests/test_resolved_config_bridge.py`, `D tests/test_tag_conflict_bridge.py` and the new
  `?? odd/tasks/go-20-u2e-t5-pr4a-s5-bridge1.md`. No `lib/` file, no golden, no other Python test and
  no production code touched. Method count of the deleted files (`git show HEAD:<file> | grep -c
  "^    def test_"`): 5 + 9 + 11 + 10 = 35, matching the baseline's `Ran 35 tests`.
- L5 — (S6) `gofmt -l catalog/recipes/worktree-flow/gate/shared` → no output (`logs/05-gofmt.log`,
  rc=0); `go -C catalog/recipes/worktree-flow/gate vet ./shared/` rc=0 (`logs/06-vet.log`);
  `go -C catalog/recipes/worktree-flow/gate test ./shared/ -count=1` rc=0, 0.177s
  (`logs/07-test-shared.log`). No production Go package was touched, so no sibling-command
  differential applies (the diff changes zero non-test lines).
- L6 — (S7) `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 (`logs/08-build.log`).
  `./tests/validate.sh` rc=0 in 642s (`logs/09-validate.log`): `gofmt` clean, gate module + root module
  Go suites green, `parity summary: gate-absent failing=0, gate-present failing=0 — PASS`, and the
  Python discover leg `Ran 2412 tests in 642.280s — OK (skipped=1, expected failures=3)`.
- L7 — Count reconciliation (S7): the base count is 2447, not the 2345 quoted in the handoff (that
  figure predates the PRs merged between the S4 slice and this branch's base c952464). The observed
  drop is `2447 − 2412 = 35`, identical to the deleted files' 35 methods and to the pre-deletion live
  baseline — so no surviving test was lost and no test was silently skipped (the baseline had 0
  skips; the single skip in the full suite is elsewhere and pre-existing).
- L8 — Finding for the parent: the four files are also named in historical `odd/tasks/*.md` records
  (`go-classify-override.md`, `go-resolved-config.md`, `go-07-s13-resolvedconfig.md`,
  `go-toml-writer.md`, `advisory-hardening.md`, `go-primitive-conflicts.md`, `go-07-s14b-*.md`,
  `go-07-s13-mergeconfig.md`). Those are immutable history and were left untouched; no live build,
  script, harness or test references them (`grep` over the tree excluding `odd/tasks` and the
  gitignored `tests/__pycache__` bytecode matches only the new Go comment that names the retired
  fixture).
