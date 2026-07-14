# Verify Report: CLI version pinning

**Change**: `cli-version-pinning`  
**Branch**: `feat/cli-version-pinning`  
**Date**: 2026-06-23  
**Validation**: `./tests/validate.sh` — **PASS** (733 tests)

## cli-version-contract

| Requirement | Scenario | Status | Evidence |
|-------------|----------|--------|----------|
| Tool section in manifest | Exact pin declared | COMPLIANT | `tests/test_cli_version.py::CliVersionPolicyParseTests::test_exact_pin` |
| Tool section in manifest | Min version inferred policy | COMPLIANT | `tests/test_cli_version.py::CliVersionPolicyParseTests::test_min_inferred_policy` |
| Tool section in manifest | Conflicting version fields rejected | COMPLIANT | `tests/test_cli_version.py::CliVersionPolicyParseTests::test_conflicting_fields_rejected` |
| Installed CLI version resolution | Version read from AI_SPECS_HOME | COMPLIANT | `tests/test_cli_version.py::CliVersionInstalledTests::test_read_installed_version` |
| Installed CLI version resolution | Missing VERSION file | COMPLIANT | `tests/test_cli_version.py::CliVersionInstalledTests::test_missing_version_file` |
| Semver comparison | Patch ordering | COMPLIANT | `tests/test_cli_version.py::CliVersionCompareTests::test_patch_ordering` |
| Semver comparison | Pre-release lower than release | COMPLIANT | `tests/test_cli_version.py::CliVersionCompareTests::test_prerelease_lower_than_release` |
| Lock file meta section | Meta written after sync | COMPLIANT | `lib/sync.sh` stamp-meta; `tests/test_lock.py::test_meta_section_written_and_ignored_on_skill_load` |
| Lock file meta section | Legacy lock without meta remains valid | COMPLIANT | `tests/test_cli_version.py::CliVersionLockMetaTests::test_read_lock_meta_absent` |
| Sync version gate | Exact pin mismatch blocks sync | COMPLIANT | `tests/test_sync_pipeline.py::CliVersionSyncGateTests::test_exact_pin_mismatch_aborts_sync` |
| Sync version gate | Min version satisfied allows sync | COMPLIANT | `tests/test_cli_version.py::CliVersionCheckPolicyTests::test_min_satisfied` + sync gate integration |
| Sync version gate | No tool section skips gate | COMPLIANT | Existing sync tests unchanged; `check-sync` returns 0 when no `[tool]` |
| Escape hatch flag | Ignore flag bypasses exact pin | COMPLIANT | `tests/test_sync_pipeline.py::CliVersionSyncGateTests::test_ignore_cli_version_flag_proceeds_with_warning` |
| Changelog for migrations | Changelog exists at repo root | COMPLIANT | `CHANGELOG.md` |

## project-doctor (delta)

| Requirement | Scenario | Status | Evidence |
|-------------|----------|--------|----------|
| CLI version diagnostics | All version sources present and aligned | COMPLIANT | `tests/test_doctor.py::CliVersionDoctorTests::test_exact_pin_aligned_reports_ok` |
| CLI version diagnostics | No pin configured with last sync recorded | COMPLIANT | `tests/test_doctor.py::CliVersionDoctorTests::test_no_pin_stale_last_sync_reports_warn` |
| CLI version diagnostics | Exact pin mismatch is ERROR | COMPLIANT | `tests/test_doctor.py::CliVersionDoctorTests::test_exact_pin_mismatch_reports_error` |
| CLI version diagnostics | Doctor remains read-only | COMPLIANT | `tests/test_doctor.py::CliVersionDoctorTests::test_doctor_cli_version_is_read_only` |

## Regression notes

- Sync idempotency preserved: `synced_at` is not bumped when `cli_version` unchanged (`tests/test_external_dirs.py::ResyncIdempotencyTests`).
- Manifest contract docs updated for `[tool]` field table rows.

## Gaps / deferred

- `ai-specs migrate` command — out of scope per proposal.
- Min-version violation doctor scenario — covered by policy unit tests; dedicated doctor integration test for `min_version` not added (behavior mirrors exact pin path in `evaluate_cli_version`).
