# Apply Progress: premerge-guardian-dated-openspec

## Scope

- Issue: `#216`
- Trello card: `#79`
- Branch: `change/premerge-guardian-dated-openspec`
- Worktree: `.worktrees/premerge-guardian-dated-openspec`

## TDD Cycle Evidence

| Step | Command | Result |
|---|---|---|
| RED | `./tests/run.sh` | The run exceeded the tool's 120-second timeout before the suite summary; visible progress showed three staged dated-archive failures (`test_passes_when_archived_with_canonical_dated_openspec_name`, `test_multiple_dated_archives_block_as_ambiguous`, and `test_dated_and_undated_archives_block_as_ambiguous`). The tool exposed no exit code, so none is claimed. |
| GREEN | `python3 -m unittest tests.test_premerge_guardian` | Exit 0; 40 tests passed. |
| Full validation | `./tests/validate.sh` | First run: 1678 tests, 116 skipped, 2 stale recipe-surface expectation failures, exit 1. After correcting those expectations, the final run passed: 1678 tests, 116 skipped, exit 0. |

The RED command was run once from the target worktree before production edits.
The captured output reached the existing pre-merge guardian test activity, but
the timeout prevented a complete suite result.

## Implementation Status

- Archive resolver implemented with exact dated/legacy resolution and fail-closed blockers.
- Live recipe and specification contract updates implemented.
- Historical `openspec/changes/archive/**` directories remain out of scope.

## Verification Snapshot

- Date: `2026-08-16`
- Revision: `8e3f3b8` (`HEAD`; implementation remains uncommitted)
- `python3 -m unittest tests.test_plan_build_flow_recipe` — exit 0; 30 tests passed.
- `python3 -m unittest tests.test_premerge_guardian` — exit 0; 40 tests passed.
- `python3 -m unittest tests.test_premerge_guardian tests.test_plan_build_flow_recipe` — exit 0; 70 tests passed.
- `./tests/validate.sh` — final run exit 0; 1678 tests passed, 116 skipped.
- `python3 -m py_compile lib/_internal/premerge_guardian.py tests/test_premerge_guardian.py tests/test_plan_build_flow_recipe.py` — exit 0.
- `git diff --check` — exit 0.
- `git diff --name-only -- openspec/changes/archive/` — no output; historical archive subtree unchanged.
- Final focused reruns after resolver cleanup: guardian 40/40, recipe surface
  30/30, and combined focused suite 70/70 passed; syntax and diff checks
  remained clean.

## Changes Applied

- Added exact direct-child archive resolution in `lib/_internal/premerge_guardian.py`.
- Preserved the staged dated, legacy, ambiguity, invalid-date, and active-folder
  regression cases; added one focused dated near-match case.
- Updated the live skill, recipe brief, recipe README, catalog docs, and active
  OpenSpec delta to state the dated provider contract and legacy fallback.
