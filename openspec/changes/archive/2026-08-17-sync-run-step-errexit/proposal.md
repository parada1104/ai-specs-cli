# Proposal: restore errexit after `run_step` cleanup in sync

## Tracker

- card_id: `#84`
- url: https://trello.com/c/ZAQB1dXA/84-sync-restore-errexit-after-cleanup-in-runstep

## Depth

**Standard.** A scoped two-file fix in a well-understood area — but on the
hottest path in the CLI (every sync, every project, five harnesses, plus
fan-out), so it gets written requirements rather than a tasks-only pass.

## Why

`run_step` in `lib/sync.sh` and `lib/sync-agent.sh` disables errexit to capture
the wrapped command's exit status, then restores it **before** printing the
captured output. A failure in that printing — a `cat` hitting SIGPIPE on an
early-closed stdout, or a full disk — therefore aborts the script from inside
the helper: both temporary files leak, and the wrapped command's exit status is
replaced by `cat`'s.

Reproduced before fixing, identically in both files:

```
AssertionError: 1 != 5 : aborted from inside the helper on the failing cat,
losing the wrapped command's status
```

This is the same defect shape corrected in `lib/upgrade.sh` under card #80
(judgment-day finding S4), carried here as an explicit follow-up.

## Why it is not a copy-paste of the upgrade fix

In `upgrade.sh` every `run_step` call sits in an `if !` context, so bash has
already suspended errexit for the invocation and the internal restore is
cosmetic.

Here the call sites are **bare** — `sync.sh:177, 179, 181, 183, 218` and
`sync-agent.sh:260, 262, 342, 492` — so the internal `set -e` is load-bearing:
it is what makes a failing step abort the sync at all. Two consequences shaped
the fix:

- The restore **cannot be removed**. `set` options are shell-global rather than
  function-local (verified directly), so dropping it would silently disable
  errexit for the remainder of the script.
- The exposed path is the **common** one, not an exotic corner. The single
  guarded call site (`sync.sh:226`) was never at risk.

## What changes

1. Move the errexit restore in both `run_step` helpers to after the temporary
   files are removed, in both the success and failure branches.
2. Detect a `mktemp` failure and name it, instead of letting it surface later
   as the wrapped command's abort message. The step still runs, with its output
   unfiltered — compact mode cannot apply when nothing is captured, and the
   warning says so.
3. Keep the two helpers identical in shape — they are intentional twins.
4. **Scope extension after judgment day**: apply the same correction to the
   hand-rolled capture block at `lib/sync.sh:210-234`.

### Scope extension: the recipe-materialize block

Both judges independently found that `lib/sync.sh:210-234` — a hand-rolled
capture around `recipe-materialize.py`, not a `run_step` call — carries the
identical defect: errexit restored at line 219, before its own `cat` (227-228)
and `print_step_output` (232-233) calls.

It is **pre-existing**: this change's diff touched 0 lines of it, and the same
code is present verbatim on `development`.

One judge found a detail the author had missed: the `trap … EXIT` registered at
line 213 covers only `RECIPE_MCP_TEMP`, `RESOLVED_CONFIG_TEMP` and
`RESOLVED_HOOKS_TEMP`. `RECIPE_OUT_FILE` and `RECIPE_ERR_FILE` are **not in the
trap**, so unlike every other temp file in this script they have no cleanup
safety net if that path aborts.

It is folded in here rather than deferred because it is the same defect, in the
same file, in a block that exists precisely because it predates `run_step`.
Leaving one instance fixed and its neighbour broken would be worse than either
outcome. This proposal was updated **before** the code, and the change re-enters
judgment day against the new target.

## Success criteria

1. A failure inside the helper's own output handling does not abort the script
   from inside `run_step`; the wrapped command's exit status survives and the
   temporary files are cleaned up.
2. Errexit is enabled after `run_step` returns, on both the success and failure
   paths, so a bare failing step still aborts the sync with the command's own
   status.
3. A `mktemp` failure names itself and the step still runs; when its output
   cannot be captured, the warning states that the output is unfiltered.
4. The compact/verbose output contract and every existing sync behavior are
   unchanged.
5. The recipe-materialize capture block restores errexit only after its own
   cleanup, and its two temporary files are covered by a cleanup safety net
   rather than relying on the abort path never being taken.

## Non-goals

- Changing which steps abort a sync, or any output format or message.
- `lib/upgrade.sh` — corrected separately under card #80.
- Any change to `print_step_output`.

## Affected areas

| Area | Impact | Description |
|---|---|---|
| `lib/sync.sh` | Modified | `run_step` errexit restore point, mktemp guard |
| `lib/sync-agent.sh` | Modified | Same, kept identical |
| `tests/test_sync_run_step_errexit.py` | New | 10 cases, each run against both helpers |
