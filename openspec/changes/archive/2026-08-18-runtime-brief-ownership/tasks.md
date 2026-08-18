# Tasks: runtime-brief ownership

Depth: full

Tracker: card #81 — https://trello.com/c/tR60h8lX

Discipline: red-green-refactor. Failing test first, then the minimum
implementation. `./tests/validate.sh` must pass before the PR.

## WU0 — Confirm the exposure before changing anything

The report that triggered this was wrong about the file. Prove the premise.

- [x] Reproduce: a hand-written `AGENTS.md` with no marker IS overwritten today
      by `init`, and by `sync`
- [x] Confirm `sync-agent` renders without the preserve flag, and that this is
      reachable only for `TARGET_PATH != SOURCE_ROOT`
- [x] Confirm a pre-existing regular `CLAUDE.md` is still refused, not
      clobbered — this change must not alter that

## WU1 — Classification for the brief

- [x] RED: `missing`, `untracked`, `user_modified`, `managed_current`,
      `managed_stale` each map to the intended write decision
- [x] RED: an unreadable lock and an unreadable target both resolve to preserve
- [x] RED: the marker preserves regardless of state
- [x] GREEN: decide inside `agents-render.py` using
      `classify_managed_override`; no second classifier

## WU2 — Baseline recording

- [x] RED: a successful write records the new bytes as the baseline
- [x] RED: a preserved target records nothing
- [x] GREEN: a brief-baseline recorder in `lock.py`, beside `set_gate_baseline`

## WU3 — Migration

The highest-risk work unit. Every existing project currently has no baseline.

- [x] RED: bytes identical to would-write adopt silently, with no extra output
- [x] RED: bytes that differ are preserved, never adopted
- [x] RED: an ordinary up-to-date project sees **no** behavior change and no
      new output
- [x] RED: explicit adoption records the current bytes and hands over management
- [x] GREEN: implement adoption plus the explicit opt-in
- [x] Measure how many of this repo's own fixtures land in each state — focused fixtures cover all five states; 0 fixture leaks

## WU4 — One guard, three entry points

- [x] RED: `init`, `sync`, `sync-agent` produce identical decisions for
      identical inputs
- [x] RED: the `sync-agent` fan-out path is covered specifically
- [x] GREEN: route all three through the same decision

## WU5 — Reporting

- [x] RED: a skipped write names the state and both remedies
- [x] RED: `doctor` reports the brief's ownership state
- [x] GREEN: implement both messages
- [x] Check the wording against the failure it describes: would a user who has
      never read this spec know what to do?

## WU6 — Documentation

- [x] Guidance for repositories that already have agent documentation
- [x] Correct the record on `CLAUDE.md`: it is a symlink and is already
      refused, so the widely-shared mitigation was aimed at the wrong file
- [x] Document the adopt path and the marker as the two explicit exits

## WU7 — Close out

- [x] `./tests/validate.sh` green
- [x] End-to-end: a scratch repo with a hand-written `AGENTS.md` survives
      `init` + `sync` + `sync-agent`
- [x] End-to-end: an ordinary project still updates its brief with no prompt
- [x] Every new assertion checked for falsifiability — revert the fix, confirm
      RED, restore
- [x] `verify-report.md` with the canonical evidence block and success-criteria
      mapping
- [ ] Archive on the review branch before merge

## Deliberately excluded

- `CLAUDE.md` and other per-agent slots — symlinks, already refused
- Merging user content with generated content
- Removing the marker or `[brief].render`
- A force flag for `user_modified` (see design D3)
