# Verify Report: plan-build-depth-artifacts-verify

## Verify evidence

- Verdict: PASS
- Command: `./tests/validate.sh`
- Exit: 0
- Date: 2026-08-07
- Commit: b83fc047
- ready_for_archive: true

## Success-criteria mapping

- Criterion 1: PASS — Skill and guardian agree on Light/Standard/Full minima, including Light `proposal.md`.
- Criterion 2: PASS — Standard explore criteria and the recorded skip path are documented; missing `explore.md` is not a machine gate.
- Criterion 3: PASS — Light advisory, Standard dedicated-report enforcement, and Full strict PASS plus ready marker are documented and tested.
- Criterion 4: PASS — Verify enforcement is documented at pre-archive and pre-merge, with no bypass flag.
- Criterion 5: PASS — #59 classifier, conflict, annotation, standalone `Depth:` contract, brief rules 1/7, and seven-rule topology are preserved by comparison checks and focused tests.
- Criterion 6: PASS — Canonical PR minimum scenario requires Standard proposal/tasks/spec and includes the Light-without-proposal blocker.
- Criterion 7: PASS — Recipe surfaces are versioned consistently at 1.6.0 while #59's 1.5.0 changelog entry remains unchanged.
- Criterion 8: PASS — Grandfathering paragraph is present; historical archive preservation and stale-PR owning-agent handling are documented.
- Criterion 9: PASS — Focused guardian (30/30), focused recipe (25/25), and repository validation (1344/1344) pass.
- Criterion 10: PASS — Vocabulary hygiene and recipe materialization checks pass.

## Verification notes

The report's `Commit` field identifies the consolidated snapshot used for the
focused test and guardian runs (b83fc047); the archive-tail delivery commit is
reported separately in the archive run evidence.

Archive-tail was executed on 2026-08-07 on this review branch
(`change/plan-build-depth-artifacts-verify`):

- The canonical sync was completed (archive-time fallback, parent-approved) —
  see `sync-report.md`; all six delta requirements are byte-equal in
  `openspec/specs/plan-build-flow/spec.md`, and the 21 non-delta requirements
  are preserved.
- The active change folder was moved to
  `openspec/changes/archive/plan-build-depth-artifacts-verify/`.
- The pre-merge guardian passed on the archived tree:
  `python3 lib/_internal/premerge_guardian.py plan-build-depth-artifacts-verify
  --root . --tier full --stage pre-merge` → `premerge-guardian: OK (full)`.
- Task 5.2 was checked (stale-checkbox reconciliation, see `archive-report.md`);
  no `- [ ]` implementation task boxes remain in `tasks.md`.

`ready_for_archive: true` is now backed by completed archival, not only artifact
readiness. The branch was committed and pushed so PR #185 reflects the archived
state; no merge was performed (#59/#62 untouched).
