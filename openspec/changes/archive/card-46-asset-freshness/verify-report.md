```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:8b8ad32657a0d76ed8c3638610c88e2fc495eeec6f874204575c0a1c3a75d13a
verdict: pass_with_warnings
blockers: 0
critical_findings: 0
requirements: 4/4
scenarios: 18/18
test_command: ./tests/validate.sh
test_exit_code: 0
test_output_hash: sha256:17a1a35238e90b1b79df78e5b93fa4ac64face60942b06f6bfa26a3d7b7346d8
build_command: python3 -m py_compile lib/_internal/*.py tests/*.py && bash -n lib/*.sh bin/ai-specs tests/*.sh && go -C catalog/recipes/worktree-flow/gate build ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report

**Change**: card-46-asset-freshness
**Round**: 2 of 3 (re-verification after Judgment Day corrections)
**Version**: worktree-flow (recipe version unchanged by design)
**Mode**: Strict TDD (cached capability `sdd/ai-specs-cli/testing-capabilities`: Strict TDD Mode enabled, runner `./tests/run.sh`; full validation `./tests/validate.sh`)
**Artifact set**: Full (proposal + delta spec + design + tasks + apply-progress + judgment-ledger) — all dimensions verified.
**evidence_revision definition**: SHA-256 over the four execution evidence digests in fixed order (build output digest, Go test output digest, Go vet output digest, validate.sh output digest), concatenated as lowercase hex with no separators.

### Completeness
| Metric | Value |
|--------|-------|
| Tasks total | 53 |
| Tasks complete | 53 |
| Tasks incomplete | 0 |

All 53 tasks checked in `tasks.md` (planning guard 0.1–0.4, phases 1–7, acceptance checklist). No core or cleanup task outstanding. Apply-progress and the round-1 verify-report are treated as historical input; every claim below was independently re-executed in this worktree on the post-Judgment-Day candidate.

### Judgment Day Correction Verification

The Judgment Day ledger records three bounded corrections after the round-1 verification. Each was independently confirmed present in source AND proven by a runtime-passing regression test in this round.

| Finding | Correction | Implementation evidence | Runtime evidence (this round) |
|---|---|---|---|
| JD-B-001 (CRITICAL): newline-containing pathnames could make an unmerged branch appear tree-equivalent and eligible for deletion | NUL-delimited `git diff --name-only -z` + process-substitution `read -r -d ''` loop in `candidate_has_combined_tree_equivalence` | `catalog/recipes/worktree-flow/templates/worktree-cleanup.sh:255-278` | `test_preserves_newline_pathname_worktree` — ✅ passed individually |
| JD-B-002 (CRITICAL, correction 1): unverified prior legacy bytes could be executed through the launcher fallback | Atomic `.verified` provenance sidecar written after verified materialization (`_write_legacy_verification`); launcher `_legacy_verified` recomputes on-disk sha256 and fails closed on missing/stale/mismatched receipt | `lib/_internal/recipe-materialize.py:867-896`, `catalog/recipes/worktree-flow/hooks/worktree-gate.sh:194-211,226-241` | `test_legacy_gate_materialization_writes_verified_sidecar`, `test_legacy_fallback_fails_closed_without_valid_provenance`, `test_legacy_fallback_under_derived_root` — ✅ passed individually |
| JD-B-002 (CRITICAL, correction 2): a failed refresh could leave an old matching `.verified` receipt beside restored old bytes, letting the launcher execute stale legacy bytes | `_invalidate_legacy_verification` removes the sidecar in the failure path after target/lock rollback | `lib/_internal/recipe-materialize.py:899-912,974-981` | `test_failed_legacy_refresh_invalidates_stale_receipt` — ✅ passed individually |

Spec conformance of the corrections: JD-B-001 tightens the complete-equivalence proof required by REQ-1 (a quoted literal resolving to no entry must never prove merge). JD-B-002 corrections implement the REQ-3 fail-closed rules ("MUST NOT accept or execute an unverified asset"; "no partial temporary file or unverified asset MUST become executable") for the governed legacy fallback and extend the REQ-4 `<binary>.verified` receipt contract to the legacy hook. No correction changed cleanup or gate decision policy, candidate order, no-fetch behavior, or generic recipe policy. Round-3 Judgment Day disposition: **approved, 0 severe findings, 0 contradictions**.

### Build & Tests Execution
**Build**: ✅ Passed
```text
python3 -m py_compile lib/_internal/*.py tests/*.py && bash -n lib/*.sh bin/ai-specs tests/*.sh && go -C catalog/recipes/worktree-flow/gate build ./...
exit 0 | output sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855 (empty on success)
```
Additional syntax checks on the three corrected governed shell assets (`bash -n` on `templates/worktree-cleanup.sh`, `hooks/worktree-gate.sh`, `hooks/worktree-gate-legacy.sh`) → exit 0.

**Tests** (configured final validation): ✅ 1683 passed / 0 failed / 116 skipped
```text
./tests/validate.sh
Ran 1683 tests in 464.143s
OK (skipped=116)
exit 0 | output sha256:17a1a35238e90b1b79df78e5b93fa4ac64face60942b06f6bfa26a3d7b7346d8
```
(+4 tests vs round 1: the JD-B-001 newline-path regression test and the three JD-B-002 sidecar/invalidation/fail-closed tests.)

**Focused suites** (all re-executed in this verification run):
```text
python3 -m unittest tests.test_worktree_cleanup tests.test_worktree_gate_hook tests.test_worktree_gate_dist_config -q
                                                                         → Ran 235 tests ... OK (skipped=94)
python3 -m unittest tests.test_gate_binary_dist tests.test_worktree_gate_release_phase4 -q
                                                                         → Ran 29 tests ... OK
python3 -m unittest tests.test_doctor_worktree_gate tests.test_override_ownership -q
                                                                         → Ran 27 tests ... OK
python3 -m unittest tests.test_recipe_materialize tests.test_worktree_flow_recipe -q
                                                                         → Ran 90 tests ... OK
python3 -m unittest tests.test_cli_version tests.test_lock tests.test_doctor -q
                                                                         → Ran 107 tests ... OK
python3 -m unittest tests.test_sync_pipeline -q                          → Ran 91 tests ... OK (130.101s)
go -C catalog/recipes/worktree-flow/gate test ./...                      → ok ai-specs.dev/worktree-gate | exit 0 | sha256:57d0456b92a446749da6aee45ae51db3a82b989437b25657e4943f7c550dec23
go -C catalog/recipes/worktree-flow/gate vet ./...                       → exit 0, no output
go version                                                               → go1.24.13 darwin/arm64
```
The JD-targeted tests were additionally executed individually with `-v` and observed `ok` (not skipped): `test_preserves_newline_pathname_worktree`, `test_failed_legacy_refresh_invalidates_stale_receipt`, `test_legacy_gate_materialization_writes_verified_sidecar`, `test_legacy_gate_user_modification_is_force_replaced_and_recorded`, `test_legacy_fallback_fails_closed_without_valid_provenance`, `test_legacy_fallback_under_derived_root`.

**Coverage**: ➖ Python coverage not configured for this repository (deferred, not a blocking signal per testing-foundation). Go gate module: informational only; no Go source changed in this change.

### Spec Compliance Matrix
| Requirement | Scenario | Test | Result |
|-------------|----------|------|--------|
| REQ-1 Positive Base Candidate Resolution for Merge Detection | Multi-commit regular merge is eligible | `tests/test_worktree_cleanup.py > test_removes_multi_commit_regular_merge` | ✅ COMPLIANT |
| REQ-1 | Multi-commit squash merge is eligible | `tests/test_worktree_cleanup.py > test_removes_multi_commit_squash_merge` | ✅ COMPLIANT |
| REQ-1 | Partial squash is preserved | `tests/test_worktree_cleanup.py > test_preserves_partial_multi_commit_squash` | ✅ COMPLIANT |
| REQ-1 | Reverted change is preserved | `tests/test_worktree_cleanup.py > test_preserves_reverted_multi_commit_squash` | ✅ COMPLIANT |
| REQ-2 Conservative Skip for Dirty Worktrees | Detached worktree is preserved | `tests/test_worktree_cleanup.py > test_preserves_detached_worktree_under_configured_directory` | ✅ COMPLIANT |
| REQ-2 | Main worktree is never removed | `tests/test_worktree_cleanup.py > test_never_reports_main_worktree_as_removable` | ✅ COMPLIANT |
| REQ-2 | Topology-protected worktree is preserved | `tests/test_worktree_cleanup.py > test_uninitialized_submodule_skipped`, `test_explicit_topology_standalone_skips_submodule_enumeration`, `test_submodule_scope_flag_limits_to_one_module` | ✅ COMPLIANT |
| REQ-3 Forced Latest-Canonical Refresh for Governed Worktree-Flow Assets | Stale cleanup override forces verified replacement | `tests/test_recipe_materialize.py > test_worktree_flow_managed_stale_cleanup_is_force_replaced` | ✅ COMPLIANT |
| REQ-3 | Unknown cleanup override forces canonical ownership replacement | `tests/test_recipe_materialize.py > test_divergent_worktree_override_is_force_replaced_and_sync_succeeds` | ✅ COMPLIANT |
| REQ-3 | Customized gate is force-replaced by ordinary sync | `tests/test_sync_pipeline.py > test_ordinary_sync_preserves_customized_gate_refresh_flag_updates`; `tests/test_worktree_gate_dist_config.py > test_legacy_gate_user_modification_is_force_replaced_and_recorded` | ✅ COMPLIANT |
| REQ-3 | Current worktree-flow assets remain idempotent | `tests/test_recipe_materialize.py > test_identical_override_no_stale_warn`; `tests/test_doctor_worktree_gate.py > test_healthy_binary_reports_ok` | ✅ COMPLIANT |
| REQ-3 | Failed canonical verification fails closed | `tests/test_gate_binary_dist.py > test_digest_mismatch_never_installs_and_records`, `test_cache_version_mismatch_is_not_executed_before_reacquisition`, `test_cache_selftest_failure_forces_reacquisition`, `test_verification_receipt_failure_leaves_cache_unselected` | ✅ COMPLIANT |
| REQ-3 | Failed replacement rolls back governed state | `tests/test_recipe_materialize.py > test_failed_worktree_lock_update_rolls_back_target_and_backup`; `tests/test_worktree_gate_dist_config.py > test_failed_legacy_refresh_invalidates_stale_receipt` (JD-B-002 correction 2) | ✅ COMPLIANT |
| REQ-4 Current Gate Asset and Release Freshness | Stale cache binary is not accepted as current | `tests/test_gate_binary_dist.py > test_stale_executable_cache_is_revalidated_and_reacquired`; `tests/test_worktree_gate_dist_config.py > test_unverified_cache_binary_is_not_executed` | ✅ COMPLIANT |
| REQ-4 | Committed release digest remains authoritative | `tests/test_worktree_gate_release_phase4.py > test_committed_digests_match_locally_built_assets`, `test_every_target_has_a_committed_digest_entry`, `test_release_workflow_pins_canonical_toolchain_without_broken_cache` | ✅ COMPLIANT |
| REQ-4 | Doctor exposes actionable freshness evidence | `tests/test_doctor_worktree_gate.py > test_unverified_cache_evidence_reports_error`, `test_digest_mismatch_record_reports_error`; `tests/test_doctor.py > test_stale_worktree_override_is_error_and_sync_remains_repair_path`; `tests/test_override_ownership.py > test_doctor_reports_worktree_flow_cleanup_as_force_refreshable_error` | ✅ COMPLIANT |
| REQ-3 (orphan scenario) | Canonical preflight precedes project writes | `tests/test_worktree_flow_recipe.py > test_worktree_flow_freshness_preflight_is_read_only`, `test_sync_runs_worktree_flow_preflight_before_project_writes` | ✅ COMPLIANT |
| REQ-4 (orphan scenario) | Version and lock drift is distinguishable | `tests/test_cli_version.py > test_version_lock_drift_is_reported_without_rewriting_metadata` | ✅ COMPLIANT |

**Compliance summary**: 18/18 scenarios compliant — every scenario has at least one covering test that passed at runtime in this verification run. The last two scenarios sit directly under `## ADDED Requirements` without an explicit `### Requirement:` header; they are mapped to the semantically owning requirement above. Requirement count = 4 (two MODIFIED, two ADDED).

### Correctness (Static Evidence)
| Requirement | Status | Notes |
|------------|--------|-------|
| REQ-1 Complete multi-commit merge proof | ✅ Implemented | `candidate_has_combined_patch_equivalence` + `candidate_has_combined_tree_equivalence` added after the existing per-commit `git cherry` proof. JD-B-001 NUL-safe pathname consumption present (`--name-only -z`, `read -r -d ''`, process substitution). Candidate order, no-fetch, ancestry-first preserved; partial and reverted changes still fail both proofs. |
| REQ-2 Conservative preservation | ✅ Implemented | Dirty/detached/main/topology preservation branches unchanged; cleanup suite green including all negative fixtures. |
| REQ-3 Forced latest-canonical refresh | ✅ Implemented | `lib/_internal/recipe-materialize.py`: `force_worktree_asset` (cleanup override), worktree-flow branch of `materialize_hook_script`, lock-backed `materialize_legacy_gate` with `.verified` sidecar + `_invalidate_legacy_verification` on failed refresh, transactional `_replace_governed_asset` (snapshot target+lock → cache-only immutable backup → atomic replace → readback verify → lock record → full rollback on failure), `_forced_replacement_message`. Generic recipe policies untouched. |
| REQ-3 Preflight before writes | ✅ Implemented | `preflight_worktree_flow` (`--preflight`) verifies canonical Bash sources read-only; `lib/sync.sh` runs it before the first project write (ordering asserted by test); materialization repeats classification/verification at each governed write. |
| REQ-4 Gate cache revalidation | ✅ Implemented | `lib/_internal/gate_binary.py`: cache hit revalidated (trust-root digest → version → self-test), `.verified` atomic receipt, quarantine of rejected candidates; download path verifies digest/version/self-test before `os.replace`. |
| REQ-4 Launcher hot-path rejection | ✅ Implemented | `catalog/recipes/worktree-flow/hooks/worktree-gate.sh` `_cache_candidate_verified` requires `status=verified`, `version=<stamped>`, 64-hex digest, `selftest=passed` before `exec`; legacy fallback `_legacy_verified` recomputes on-disk sha256 and fails closed; no-digest hot path preserved. |
| REQ-4 Doctor evidence | ✅ Implemented | `lib/_internal/doctor.py` classifies governed assets read-only and emits ERROR with state/digests and "ordinary sync will force the latest verified replacement"; no mutation (asserted by tests). |

### Coherence (Design)
| Decision | Followed? | Notes |
|----------|-----------|-------|
| Keep cleanup in Bash; correct only after a failing RED fixture | ✅ Yes | RED proved the multi-commit squash failure; fixes extend the existing decision points conservatively; no Go port, no new merge heuristic |
| Two deliberately separate seams (cleanup vs freshness) | ✅ Yes | Cleanup diff touches only `templates/worktree-cleanup.sh`; freshness spans materializer/gate_binary/doctor/sync/launcher |
| Freshness scoped to worktree-flow; generic template policies unchanged | ✅ Yes | Forced path gated on `recipe_id == "worktree-flow"` + exact targets; generic `auto`/`confirm`/`never-force` tests green |
| Reuse lock/provenance classifier and existing cache-only backup | ✅ Yes | `classify_managed_override`, `set_managed_override`, `set_gate_baseline`, gate backup path reused; no parallel manifest invented |
| Preflight before sync writes + repeat check at write boundary | ✅ Yes | `lib/sync.sh` ordering + `materialize_*` reclassification |
| Go gate decision policy unchanged | ✅ Yes | No Go source changed (zero diff under `catalog/recipes/worktree-flow/gate/`); go test/vet/build green |
| Doctor read-only, sync is the repair path | ✅ Yes | Test asserts target bytes unchanged after doctor |
| Version/lock drift reported, lock not rewritten | ✅ Yes | `evaluate_cli_version` emits WARN naming both versions; dogfood lock stays visible |
| Release sums/toolchain/pin/asset names unchanged | ✅ Yes | No changes to `scripts/build-gate.sh`, `scripts/verify-gate-sums.sh`, or the release workflow; release parity tests green |

Design deviations: none found.

### TDD Compliance
| Check | Result | Details |
|-------|--------|---------|
| TDD Evidence reported | ✅ | apply-progress carries `### RED` / `### GREEN` sections with exact commands and observed outcomes; Judgment Day correction rounds add focused-test evidence in the ledger; not the structured per-task `TDD Cycle Evidence` table prescribed by strict-tdd-verify.md (see WARNING 1) |
| All tasks have tests | ✅ | Every task area maps to a test module changed in this diff (cleanup, materialize, gate dist/config/hook, doctor, override ownership, sync pipeline, cli version) |
| RED confirmed (tests exist) | ✅ | RED-named fixtures exist and were re-executed green this round; JD corrections each carry a dedicated regression test |
| GREEN confirmed (tests pass) | ✅ | Every GREEN-claimed suite re-executed green in this verification run (see Build & Tests Execution) |
| Triangulation adequate | ✅ | Positive + negative per behavior: complete vs partial vs reverted squash; stale/user-modified/unknown/current states; digest/version/self-test mismatch axes; receipt-present vs receipt-missing vs receipt-stale after failed refresh |
| Safety Net for modified files | ⚠️ | No explicit safety-net table in apply-progress; the full-suite RED run demonstrates existing tests were exercised at modification time |

**TDD Compliance**: 5/6 checks fully passed; safety-net recording is the partial check (non-blocking, see WARNING 2).

### Test Layer Distribution
| Layer | Tests | Files | Tools |
|-------|-------|-------|-------|
| Unit | 235 + 29 + 27 + 90 + 107 | `test_worktree_cleanup`, `test_worktree_gate_hook`, `test_worktree_gate_dist_config`, `test_gate_binary_dist`, `test_worktree_gate_release_phase4`, `test_doctor_worktree_gate`, `test_override_ownership`, `test_recipe_materialize`, `test_worktree_flow_recipe`, `test_cli_version`, `test_lock`, `test_doctor` | python unittest + real temp Git repos / mocked network |
| Integration | 91 | `test_sync_pipeline` | python unittest + subprocess (`ai-specs init`/`sync` in temp projects) |
| Go unit/parity | full module | `catalog/recipes/worktree-flow/gate` | go 1.24.13 (`go test` ok, `go vet` clean) |
| E2E | — | not configured | — |
| **Total executed** | **1683 full + focused re-runs** | 12 changed test files | |

### Changed File Coverage
Python: coverage analysis skipped — no coverage tool configured in this repository (informational, not a failure).
Go gate module: no source changes in this diff (informational only).

### Assertion Quality
✅ All assertions verify real behavior. Audit of the changed test files (including the JD regression tests) found:
- No tautologies (`assert True`, `assertEqual(True, True)` class).
- No type-only assertions standing alone; assertions pin stable output strings (`would remove`, `skipped <name> (unmerged)`), file bytes, receipt presence/absence, lock state, subprocess exit codes, and digest values.
- No ghost loops over possibly-empty collections; fixtures build real commit graphs and assert exact expected output plus worktree/branch existence.
- The JD regression tests are behavioral: `test_preserves_newline_pathname_worktree` creates a real Git worktree with a newline-containing filename and asserts preservation + `unmerged` output; `test_failed_legacy_refresh_invalidates_stale_receipt` injects a real refresh failure and asserts target rollback AND sidecar absence.
- Mock usage bounded: `mock.patch.object` injection points only; no mock-heavy tests.

### Quality Metrics
**Linter**: ➖ Not configured (Python)
**Type Checker**: ➖ Not configured (Python)
**Go vet**: ✅ exit 0, no output
**gofmt**: ✅ `validate.sh` gofmt step passed (`gofmt -l` empty)
**Whitespace**: ✅ `git diff --check` exit 0

### Issues Found
**CRITICAL**: None

**WARNING**:
1. apply-progress reports TDD cycle evidence as RED/GREEN prose sections rather than the structured per-task `TDD Cycle Evidence` table prescribed by `strict-tdd-verify.md`. The substantive evidence exists and was independently re-verified by execution, so this is a format/record-keeping deviation, not missing evidence.
2. No explicit per-file safety-net record in apply-progress for the modified test files. The full-suite RED run shows existing tests were exercised during modification; a record-keeping gap, not a regression signal.
3. Environment-gated skips: 116 tests skipped in the full suite (pre-existing, documented) and 94 in the cleanup/hook/dist-config combo. Every changed-behavior test in this change — including all six JD-targeted tests — ran and passed in this round; none of the change's covering tests is among the skips.
4. Judgment Day informational carry-over (from ledger, non-blocking, approved disposition): the legacy `.verified` receipt stores a normalized digest while the launcher verifies raw bytes, so a CRLF-mangled legacy target would fail closed rather than execute; and a launcher delivered without its sidecar fails closed until the next sync. Both are fail-closed directions and are not defects.

**SUGGESTION**:
1. `gate_binary.verify_cached_binary` carries duplicated digest-reason branches after its early returns (dead code); harmless, could be tidied in a future pass.
2. At archive time, give the two trailing delta scenarios explicit `### Requirement:` headers (or fold them into REQ-3/REQ-4) so machine counting of scenarios is heading-uniform.

### Verdict
**PASS WITH WARNINGS**

All 53 tasks complete; 4/4 requirements and 18/18 scenarios covered by tests that passed at runtime in this verification round; the three Judgment Day corrections are present in source and each proven by a dedicated runtime-passing regression test; configured final validation `./tests/validate.sh` green (1683 tests, exit 0, skipped=116 documented); Go test/vet/build green; design coherent with no deviations; gofmt and `git diff --check` clean. No new failing check was introduced by the corrections; no critical finding exists. The four warnings are record-keeping format gaps, environment-gated skips, and informational fail-closed carry-overs from Judgment Day — none blocks archive, none indicates a behavioral defect.
