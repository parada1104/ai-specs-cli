# Verification report: runtime-brief ownership

## Verify evidence
- Verdict: PASS
- Command: `./tests/validate.sh`
- Exit: 0
- Date: 2026-08-18
- Commit: d4c5536
- ready_for_archive: true

## Evidence

- Focused affected suites: 199 tests passed, 0 failures, 0 errors.
- New ownership suite: 15 tests passed, covering missing, untracked, user-modified,
  managed-current, managed-stale, marker, unreadable lock, unreadable target,
  exact-match migration, explicit adoption, init, sync, sync-agent fan-out, and doctor.
- Shell syntax: `bash -n lib/init.sh lib/sync.sh lib/sync-agent.sh` passed.
- `git diff --check` passed.
- Full `./tests/validate.sh`: 1846 tests passed, 116 skipped, exit 0.

## Success-criteria mapping

- Criterion 1: PASS — untracked hand-written AGENTS.md survives init, sync, and sync-agent; output names state and both remedies.
- Criterion 2: PASS — user-modified generated brief remains byte-identical and is never overwritten.
- Criterion 3: PASS — managed-stale untouched brief updates silently and records its new baseline.
- Criterion 4: PASS — init, sync, and sync-agent fan-out call the same renderer decision path.
- Criterion 5: PASS — runtime-brief marker preserves the file unconditionally.
- Criterion 6: PASS — `[brief].render = false` entry-point behavior remains unchanged in affected tests.


## Coordinator verification

The worker reported its own run as `TMPDIR=/tmp ./tests/validate.sh`. That
override was re-run without, so no environment condition is inherited that
cannot be reproduced:

**`./tests/validate.sh` — exit 0, 1864 tests, 0 failures.**

The worker's three modified/new suites were also run separately under the
default macOS temp directory before trusting the claim: 15, 12 and 9 tests, all
OK. The override was not needed.

## Judgment day

Two blind judges, round one: 2 confirmed by both, 5 suspect, 0 contradictions.
All addressed except one recorded deferral (S5). See `judgment-ledger.md`.

The finding that mattered most: the preserve message, `doctor`, and the
troubleshooting doc all told the user to run `ai-specs sync --adopt-brief`, and
for a `user_modified` brief that flag was never inspected — the documented
remedy silently did nothing, forever. Combined with an interrupted-write window
that could push an ordinary never-edited brief into `user_modified`, a project
could get permanently stuck with no working recovery. Both are fixed, each with
a test that failed first.
