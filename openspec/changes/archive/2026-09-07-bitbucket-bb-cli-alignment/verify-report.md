# Verification Report: Bitbucket `bb-cli` Alignment

## Verify evidence

- Verdict: PASS
- Command: `./tests/validate.sh`
- Exit: 0
- Date: 2026-09-07
- Commit: effa753
- ready_for_archive: true

## Results

- Python unittest suite: 1,805 tests passed, 0 failures, 0 errors, 133 skipped.
- Go worktree-gate tests: passed.
- Python compilation checks: passed.
- Shell syntax checks: passed.
- Go formatting checks: passed.
- `git diff --check`: passed.

## Success-criteria mapping

- Criterion 1: PASS — Bitbucket recipe surfaces identify PHP `bb-cli` and remove the obsolete TypeScript identity.
- Criterion 2: PASS — install-plan tests cover Homebrew `bb-cli`, apt-only guidance, and the forbidden unrelated formula.
- Criterion 3: PASS — recipe invocations use the verified PHP verbs/options or explicit open-verification notes.
- Criterion 4: PASS — recipe version `1.3.0`, host floor `1.4.1`, version check, and renderer identity are covered by catalog tests.
- Criterion 5: PASS — agent-facing authentication guidance captures only `Username` and rejects ambiguous output without printing `AppPassword`.
- Criterion 6: PASS — positive PHP identity guard coverage blocks a different `bb` binary with installation guidance.
- Criterion 7: PASS — post-merge feature-branch cleanup and protected-head exclusions are covered by recipe and scenario tests.
- Criterion 8: PASS — apply-progress wording is path-safe and the misleading merge test name was corrected.

## Scope notes

- Live Bitbucket eval execution was not required: the repository has no Bitbucket-enabled dogfood directory, and the change intentionally does not execute real create or merge operations.
- The validation command generated only ignored Python cache directories; no additional tracked changes were produced.
