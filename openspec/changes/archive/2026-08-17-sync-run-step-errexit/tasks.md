# Tasks: sync-run-step-errexit

Depth: standard

Tracker: card #84 — https://trello.com/c/ZAQB1dXA

Follow-up to card #80. Same defect shape as the one corrected in
`lib/upgrade.sh` (judgment-day finding S4), but **not** a copy-paste — see
below.

## Why this needs its own care

In `upgrade.sh` every `run_step` call sits in an `if !` context, so bash has
already suspended errexit for the call and the internal restore is cosmetic.

In `sync.sh` and `sync-agent.sh` the call sites are **bare** (`sync.sh:177,
179, 181, 183, 218`; `sync-agent.sh:260, 262, 342, 492`). There the internal
`set -e` is load-bearing: it is what makes a failing step abort the sync.

Verified: `set` options are shell-global, not function-local. Dropping the
restore would silently disable errexit for the rest of the script. The restore
must be **moved past the cleanup**, never removed.

Also narrower than the upgrade.sh case: `sync.sh`'s failure branch already runs
`rm -f` before `return $rc`, so the ordinary failure path does not leak. Only a
failing `cat` triggers the defect.

## WU1 — Lock the errexit contract

- [x] RED: errexit is still enabled after a successful `run_step`
- [x] RED: errexit is still enabled after a handled failing `run_step`
- [x] RED: a bare failing `run_step` still aborts and preserves the status
- [x] RED: `if ! run_step` observes the wrapped command's real exit status
- [x] RED: no temporary files survive either path
- [x] Run against **both** `lib/sync.sh` and `lib/sync-agent.sh` via the
      existing `_extract_bash_functions` harness

## WU2 — Apply the fix

- [x] GREEN: move the errexit restore in `lib/sync.sh` `run_step` to after the
      temp-file cleanup
- [x] GREEN: same in `lib/sync-agent.sh` `run_step`
- [x] GREEN: name a `mktemp` failure instead of letting it surface as the
      wrapped command's abort message; the step still runs
- [x] Keep both helpers byte-comparable in shape — they are intentionally twins

## WU4 — Scope extension: the recipe-materialize capture block

Added **after** judgment day round one, with this plan updated before any code.
Both judges confirmed `lib/sync.sh:210-234` carries the identical defect, and
it is pre-existing (0 lines of it were touched by WU2).

- [x] RED: a failing `cat` while printing the block's captured output does not
      abort before cleanup, and the step's own status survives
- [x] RED: neither `RECIPE_OUT_FILE` nor `RECIPE_ERR_FILE` is stranded on any
      exit path — the `trap … EXIT` at line 213 currently omits both
- [x] GREEN: restore errexit only after the block's own cleanup
- [x] GREEN: bring both temp files under a cleanup safety net
- [x] Re-enter judgment day against the new target

## WU3 — Prove no regression on the hottest path

- [x] `./tests/validate.sh` green
- [x] Existing sync verbosity/fan-out suites unchanged
- [x] `verify-report.md` with the canonical evidence block
- [x] Archive the change folder on the review branch before merge

## Deliberately excluded

- Any change to which steps abort a sync, or to sync's output format
- `lib/upgrade.sh` — already corrected under card #80

## Discovered, not fixed (out of scope)

- `lib/_internal/recipe-materialize.py:1251` creates `ai-specs-recipe-mcp-*`
  via `mkstemp` and only prints the path (1258) — no cleanup. Accounts for the
  1 remaining leaked temp file per sync (down from 3).
- `lib/sync-agent.sh:134` and `:191` register EXIT traps with bare `$VAR`
  references, the same `set -u` clobber class hardened in `lib/sync.sh`.
