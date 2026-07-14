# Tasks: CLI version pinning

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 400–600 |
| 400-line budget risk | Medium |
| Chained PRs recommended | No |
| Suggested split | Single PR to `development` |
| Delivery strategy | Full SDD domain_change |
| Classification | domain_change |

## Phase 1: Specs & scaffolding (RED prep)

- [x] 1.1 Create `lib/_internal/cli_version.py` module skeleton with `read_installed_version`, `parse_tool_policy`, `compare_versions`, `check_policy`, `read_lock_meta`, CLI entrypoints `check-sync` and `stamp-meta`.
- [x] 1.2 Add `tests/test_cli_version.py` with RED tests:
  - semver compare (patch, pre-release, build metadata)
  - parse_tool_policy (exact, min, inferred, conflicting fields)
  - check_policy outcomes
  - read_lock_meta absent/present

## Phase 2: Lock meta (TDD)

- [x] 2.1 RED: `tests/test_lock_meta.py` — write_lock emits `[meta]` when provided; load ignores meta for hash ops.
- [x] 2.2 GREEN: Extend `lib/_internal/lock.py` — `write_lock` accepts optional meta dict; update LOCK_HEADER comment.
- [x] 2.3 GREEN: `cli_version.stamp_lock_meta()` reads VERSION, writes meta via write_lock.
- [x] 2.4 GREEN: Call stamp from `refresh-bundled.py` after write_lock.
- [x] 2.5 GREEN: Call stamp from `sync.sh` on successful completion (before exit 0).

## Phase 3: Sync gate (TDD)

- [x] 3.1 RED: `tests/test_sync_pipeline.py` — exact pin mismatch aborts sync with exit 1; `--ignore-cli-version` proceeds with warning.
- [x] 3.2 GREEN: Wire `cli_version.py check-sync` at start of `lib/sync.sh`.
- [x] 3.3 GREEN: Pass `--ignore-cli-version` flag through sync.sh argument parser.

## Phase 4: Doctor (TDD)

- [x] 4.1 RED: Extend `tests/test_doctor.py` — scenarios from project-doctor delta spec (OK, WARN, ERROR, read-only).
- [x] 4.2 GREEN: Add `Doctor._check_cli_version()` in `lib/_internal/doctor.py`.

## Phase 5: Docs & template

- [x] 5.1 Add `CHANGELOG.md` at repo root (Keep a Changelog; Unreleased + 0.12.2 baseline).
- [x] 5.2 Update `docs/ai-specs-toml.md` — `[tool]` section + field table.
- [x] 5.3 Update `docs/ai/troubleshooting.md` — CLI version mismatch section.
- [x] 5.4 Update `templates/ai-specs.toml.tmpl` — commented `[tool]` example.
- [x] 5.5 Update `README.md` — mention `[tool]` pinning and `--ignore-cli-version`.
- [ ] 5.6 Update `bin/ai-specs` sync help if needed.

## Phase 6: Verify

- [x] 6.1 Run `./tests/run.sh` — all green.
- [x] 6.2 Run `./tests/validate.sh` — all green.
- [x] 6.3 Write `verify-report.md` mapping every spec scenario to evidence.
- [x] 6.4 Write `apply-progress.md` with RED/GREEN notes per phase.

## Phase 7: Dogfood (optional follow-up in same PR)

- [ ] 7.1 Add `[tool].version = "0.12.2"` to `ai-specs/ai-specs.toml` in dogfood manifest (if bumping VERSION to 0.12.3, pin that version instead).

## Implementation order

Phase 1 → 2 → 3 → 4 → 5 → 6. Phases 2–4 are strictly TDD (RED before GREEN).
Docs (Phase 5) after behavior is green.

## Commit plan (suggested)

1. `docs(openspec): add cli-version-pinning change artifacts`
2. `feat(cli): add cli_version module with semver policy`
3. `feat(lock): stamp meta.cli_version on sync`
4. `feat(sync): enforce [tool] version policy with escape hatch`
5. `feat(doctor): report CLI version state`
6. `docs: CHANGELOG and ai-specs-toml [tool] reference`
