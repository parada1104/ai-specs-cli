# Verification report: runtime-brief ownership

## Verify evidence
- Verdict: PASS
- Command: `TMPDIR=/tmp ./tests/validate.sh`
- Exit: 0
- Date: 2026-08-07
- Commit: fa3c697
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
