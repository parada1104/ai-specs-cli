# Tracker ledger host strangler

## Objective
Remove the newly introduced Python `tracker_ledger_host.py` compatibility host from the active ledger lifecycle without creating another Python file, while preserving the Go grader and checkpoint behavior.

## Audit finding
`lib/_internal/tracker_ledger_host.py` was introduced by `dc5aa90` and is 321 lines. It contains no Go grading predicate, but duplicates existing shell-host acquisition and JSON mapping. Its one policy-bearing responsibility (`resolve_ledger_mode`) is duplicated in Python and shell; the long-term destination is Go.

## Scope
- Implement the smallest safe removal shape: reuse the existing `tracker-card-gate.sh` acquisition/JSON bridge for `pre-merge` and `archive-close`, update all skill/command/docs callsites, and delete the new Python host plus host-only tests.
- Preserve checkpoint names, evidence acquisition, mode semantics, ask/no-TTY behavior, fail-open binary resolution, exit contracts, and the distinction between `archive-close` and OpenSpec archive.
- Do not add a Python replacement or move provider logic into Go.
- Record the deeper follow-up: move effective `ledger_mode`/legacy `gate_mode` resolution from duplicated shell/Python logic into Go, with trust-root regeneration as a separate release-grade slice.

## Constraints
- Go remains the sole authoritative grader and state/predicate owner.
- Existing `ledger_bridge.py`, `gate_binary.py`, and parser-only `trello_link.py` remain only as thin acquisition/verification bridges.
- No provider writes.
- No changes to the unrelated brief or ledger-follow-up worktrees.
- TDD runner: `./tests/validate.sh`; focused host/recipe/merge tests must be run first.

## Tracker

- **card_id**: `AImzsLWw`
- **url**: https://trello.com/c/AImzsLWw/37-feat-worktree-flow-recipe-modes-always-ask-off
- **note**: Follow-up debt from the shipped tracker-ledger and worktree gate changes; Trello MCP unavailable for a new card.

## Tasks

- [x] T1 — Add RED coverage for the shell-hosted pre-merge/archive-close path and preserve ask/no-TTY/fail-open contracts. RED: focused suite failed with 23 failures/1 error before the direct host implementation.
- [x] T2 — Replace Python host callsites, remove the new host and host-only tests, and reconcile docs/skills/recipes. GREEN: direct-host replacement suite passed (263 tests).
- [x] T3 — Run focused and full validation; record the deeper Go-owned mode-resolution follow-up separately. Go packages and full validation passed (2102 tests, 142 skipped).
- [ ] T4 — Reconcile the canonical tracker-ledger spec, bridge docstring, and guardian contract tests with the retired Python host removal.
- [ ] T5 — Rerun focused/full validation and update evidence.

## Acceptance criteria

- No new Python host file is required for pre-merge/archive-close.
- All lifecycle callsites invoke the existing bridge path and preserve checkpoint semantics.
- No authoritative grading or state logic moves into shell/Python.
- Focused tests and `./tests/validate.sh` pass.
- The Go mode-resolution migration remains explicitly tracked as a separate follow-up, not silently omitted.

## Size exception authorization

The user explicitly chose one PR for this branch despite the 1260-line authored diff. This exception is for delivery shape only; all focused/full verification and the deletion/callsite/doc contracts remain required.

## Progress

- Worktree: `.worktrees/tracker-ledger-host-strangler`
- Branch: `refactor/tracker-ledger-host-strangler`
- Base: `development` at `03e227c`
- Current state: Shape-S implementation complete and verified; canonical spec/doc/test residue remains before closure; no commit yet.
- Evidence: focused replacement suite — 263 tests OK; Go gate packages OK; `./tests/validate.sh` — 2102 tests OK, 142 skipped.
- Known residue to reconcile: `openspec/specs/tracker-ledger/spec.md`, `lib/_internal/ledger_bridge.py`, and `tests/test_premerge_guardian.py` still name the retired Python host. A9 `ledger_mode`/legacy `gate_mode` resolution remains duplicated in shell and should move into Go in a separate release-grade slice with trust-root regeneration.

## Next step
Update the canonical contract and its assertions to the shell bridge, then rerun validation.
