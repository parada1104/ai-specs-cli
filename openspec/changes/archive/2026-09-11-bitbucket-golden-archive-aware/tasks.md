# Tasks: `bitbucket-golden-archive-aware`

## Planning classification

- **Requested depth:** none stated; the user asked for the fix directly, as its own change.
- **Signal depth:** Light — test-only, one guard plus one small helper, no production code, no CLI,
  schema, recipe, or documentation surface, no cross-cutting behavior.
- **Decided depth:** Light.
- **Tier minimum:** `proposal.md` plus this `tasks.md`.
- **Strict TDD:** `openspec/config.yaml` declares `strict_tdd: true` and runner `./tests/run.sh`; write the
  RED test first, then the smallest change that makes it pass.
- **Repository rule:** all work happens in the `bitbucket-golden-archive-aware` worktree. Do not modify
  `ai-specs/` project state, the `jinna-mcp-recipe` change, or any other branch.

## Phase 1 — Archive-aware resolution

### [x] 1.1 Add RED tests for the archive-aware resolver

- **Files:** `tests/test_change_paths.py` (new).
- **Test:** resolves an active change folder; resolves a date-prefixed archive entry; resolves the legacy
  undated `archive/<slug>/` entry; prefers the active folder when both exist; falls back to the
  active-shaped path when neither exists; a missing `archive/` root is not an error.
- **Test:** the guard's target for `bitbucket-bb-cli-alignment` resolves to the archived folder and the
  resolved `apply-progress.md` exists.
- **Evidence:** focused tests fail before implementation (`tests/_change_paths.py` does not exist).

### [x] 1.2 Implement `tests/_change_paths.py` and adopt it in the guard

- **Files:** `tests/_change_paths.py` (new), `tests/test_bitbucket_pr_flow_recipe.py`.
- **Implement:** `change_dir(root, slug)` and `change_artifact(root, slug, *parts)` resolving active →
  date-prefixed archive → legacy undated archive, following the `tests/_cache_paths.py` convention.
- **Implement:** the bitbucket guard resolves its artifact through the helper and keeps its existing
  assertion unchanged (no absolute host, home, or Homebrew path in the artifact).
- **Acceptance:** 1.1 passes; the guard passes against the real archived artifact; the guard's protection
  is provably intact — it still fails when the resolved artifact contains an absolute host path, pinned
  by a temporary-copy case in the new test module.

### [x] 1.3 Run the repository validation

- **Run:** `./tests/validate.sh` from the change worktree (it wraps `./tests/run.sh`, so the full
  discovery was not run twice).
- **Result:** exit code 0, `Ran 1826 tests in 563.246s — OK (skipped=133)`. The pre-change baseline was
  `FAILED (failures=1)` — the single failure this change removes. `tests/test_change_paths.py` passes
  8/8 and the guard resolves to `archive/2026-09-07-bitbucket-bb-cli-alignment/apply-progress.md`.
- **Count note — do not reuse the `Ran 1900 tests` figure as this change's gate:** that summary belongs
  to the `jinna-mcp-recipe` worktree, whose branch adds ~74 tests. Discovery on this machine is 1821 on
  `development`, 1829 here (1821 + the 8 new tests), and 1903 in the jinna worktree; the runner reports
  1826 here because three discovered cases are not executed as tests. The gate for this change is a
  green suite and exit 0, not a borrowed absolute count.

## Review workload

- Estimated changed lines: ~95 total (resolver ~35, new test module ~45, guard adoption ~5, artifacts
  excluded from the code diff).
- Comfortably inside the 400-line budget: single PR, no delivery decision required, no chained PRs.

## Judgment Day (informational rows) — 2026-09-11

Two blind judges (`jd-judge-a`, `jd-judge-b`) ran read-only on the frozen candidate
(`sha256:7718de7c0b814adf0444150df6a69f00b7b32bc2eb876e8764f394e077c9c015`) as an explicitly requested
dual review. Manifests were identical before and after: the judges wrote nothing.

**No CRITICAL rows were found**, so no fix batch was scheduled under the Judgment Day contract —
WARNING and SUGGESTION rows are one-time informational rows. The user authorized addressing those worth
fixing as part of this change, which is an ordinary author-authorized improvement, not a
Judgment Day-governed fix batch.

| Row | Severity | Disposition |
|---|---|---|
| JD-B-001 | WARNING | **Fixed.** The "protection intact" test was vacuous: it asserted the guard's regex against content the test itself authored and never invoked the guard. Replaced by a pair that invokes the real guard method with a patched `ROOT` — it must raise on a dirty artifact and pass on a clean one. Mutation-proven: deleting the guard's `assertIsNone` makes the dirty artifact pass, so the tripwire test would go red. |
| JD-B-004 | WARNING | **Fixed.** `test_real_bitbucket_guard_target_is_archived_and_clean` asserted `"archive" in target.parts` against live repository bookkeeping, so restoring the change folder (a permitted operation) would have reddened the suite — the same permanent-failure mode this change removes. It now asserts only that the target resolves, exists and is clean. |
| JD-A-003 / JD-B-002 | SUGGESTION / WARNING | **Fixed.** Both judges converged: `proposal.md` quoted `Ran 1900 tests` as this change's baseline evidence, but that figure belongs to the `jinna-mcp-recipe` worktree. The Evidence section now states the real relationship and warns against borrowing counts across worktrees. |
| JD-A-001 / JD-B-003 | SUGGESTION / WARNING | **Fixed.** "Most recent dated entry wins" had no coverage: no test created two dated entries. Added `test_newest_dated_archive_entry_wins` with three of them. |
| JD-A-002 / JD-B-006 | SUGGESTION | **Fixed.** The date prefix was validated by shape only, so `2026-99-99-<slug>` sorted above every real date and would shadow the genuine archive entry. The resolver now requires a real ISO calendar date via `date.fromisoformat`, pinned by `test_invalid_calendar_date_prefix_is_ignored` (RED before the fix: the impossible date won). |
| JD-B-005 | SUGGESTION | **Fixed.** The documented no-raise contract for a root without `openspec/`, or without `changes/`, was untested; two cases now cover it. |

## Tracker

`card_id` and `url` are recorded in the `## Tracker` section of `proposal.md`
(card `6aa3cb78e51fefeffe5b11fc`, list In Progress).
