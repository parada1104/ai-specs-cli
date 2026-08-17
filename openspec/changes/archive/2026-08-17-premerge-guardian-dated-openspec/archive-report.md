# Archive Report: premerge-guardian-dated-openspec

**Final state at close** — this report describes the state of the change when
archived, not intermediate snapshots from earlier in the cycle.

## Change

- **Change name**: `premerge-guardian-dated-openspec`
- **Archive path**: `openspec/changes/archive/2026-08-17-premerge-guardian-dated-openspec/`
- **Archive date**: `2026-08-17`
- **Depth**: standard
- **Artifact store**: openspec (filesystem)

## Delivery

- **PR**: #217 (open against `development`; NOT merged by this executor)
- **Commits**: `15f81d7` (implementation, fix plan-build accept dated openspec archives), `e13cd7c` (verify-report)

## Final verification (at close)

- **Judgment Day**: APPROVED, no severe findings.
- **Independent validation**: `./tests/validate.sh` — 1681 passed, 116 skipped.
- **Focused validation**: 73 passed.
- **py_compile and diff checks**: passed.
- **Historical `openspec/changes/archive/` subtree**: unchanged before this archive operation.

## Canonical spec sync

The delta `openspec/changes/premerge-guardian-dated-openspec/specs/plan-build-flow/spec.md`
was merged into the canonical `openspec/specs/plan-build-flow/spec.md`. The
affected requirements — *Pre-merge archive gate* and *Pre-merge merge guardian* —
now state:

- the dated OpenSpec archive path `openspec/changes/archive/YYYY-MM-DD-<slug>/`;
- the exact undated legacy fallback `openspec/changes/archive/<slug>/`;
- direct-child exact resolution of `openspec/changes/archive/`;
- fail-closed behavior for ambiguity, invalid date, and near-match candidates;
- symlink boundary behavior as implemented;
- unchanged existing gates (tier minima, staged verify, active-folder blocker,
  planning-root propagation, explore-at-standard, post-merge rejection).

All unrelated requirements and unchanged gates in the canonical spec were
preserved. No historical archive was rewritten.

## Guardian

- **Pre-archive guardian**: PASS — `premerge-guardian: OK (standard)` run at
  `--stage pre-archive` after `verify-report.md` was added.

## Archive readback

- `git mv` used to move the active change folder to the dated archive.
- Recursive `cp -R` snapshot taken before the move.
- Source folder confirmed absent after the move.
- `diff -r` (snapshot vs archived tree) returned an empty diff — byte-identical.

## Task completion

All implementation and acceptance checklist tasks in `tasks.md` are checked.
No unchecked implementation task remains in the archived `tasks.md`.

## Post-merge state

This executor did **not** merge PR #217 and did **not** push. The archive reflects
pre-merge state on the review branch at close. No post-merge state is claimed.
