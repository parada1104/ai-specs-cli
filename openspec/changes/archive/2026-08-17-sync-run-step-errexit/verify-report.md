# Verify Report: sync-run-step-errexit

## Verdict

**PASS** — the defect was reproduced before the fix, both helpers were
corrected identically, and the full suite is green on the CLI's hottest path.

## Verify evidence

- Verdict: PASS
- Command: `./tests/validate.sh`
- Exit: 0
- Date: 2026-08-17
- Commit: 319e42f

## Scope extension (WU4) — the recipe-materialize capture block

Added after judgment day round one, with the plan committed **before** any
code (`6a5acfa`).

Measured against the original block shape, all variables defined, `cat` forced
to fail:

| | exit status | stranded temp files |
|---|---|---|
| original | **1** — `cat`'s, not the step's | **2** |
| fixed | **3** — the step's own | **0** |

So both effects the judges predicted are real in production shape, not test
artifacts. The fix restores errexit only after the block's own cleanup, and
brings `RECIPE_OUT_FILE` / `RECIPE_ERR_FILE` into the `trap … EXIT` that
previously covered only the other three temporaries.

**Additional hardening found while verifying**: the EXIT trap referenced its
variables bare. Under `set -u` a trap that names an unset variable dies
mid-cleanup and **replaces the script's exit status with its own** — observed
directly, turning a clean `exit 3` into `exit 1`. Every name in the trap is now
`:-` expanded, and `test_exit_trap_cannot_clobber_the_exit_status` locks it.

### Real-world leak measurement

Per `ai-specs sync`, counting the exact files created and not cleaned (a
set difference, not a directory count — see below):

| | leaked per sync |
|---|---|
| `development` | **3** |
| this branch | **1** |

The leak is **pre-existing**; this change reduces it. The remaining one is
traced to `lib/_internal/recipe-materialize.py:1251`, which creates
`ai-specs-recipe-mcp-*` with `mkstemp` and only prints the path (line 1258)
with no cleanup. Python side, different file, outside this change's scope —
recorded, not fixed.

## Verification summary

- `./tests/validate.sh` — exit 0; **1808 tests**, 0 failures (1787 on
  `development`), after two judgment rounds.
- `tests/test_sync_run_step_errexit.py` — 10 tests, each run against **both**
  `lib/sync.sh` and `lib/sync-agent.sh`.
- `bash -n` clean on both helpers.
- Real end-to-end `ai-specs init` + `ai-specs sync` against a scratch project
  using the patched helpers — both exit 0, output format unchanged.

## RED evidence

The defect reproduced exactly as predicted, in both files:

```
AssertionError: 1 != 5 : aborted from inside the helper on the failing cat,
losing the wrapped command's status
```

A `cat` failure inside `run_step` aborted the script with cat's status (1)
instead of the wrapped command's (5), skipping the temp-file cleanup.

## What the investigation corrected about the original diagnosis

Three refinements came out of reproducing this rather than assuming it:

1. **`[[ -s f ]] && cat f` does not trip errexit when `[[` fails.** A non-final
   command in an `&&` list is exempt. Only a failing `cat` — the final command
   — triggers it. An empty capture file is therefore harmless, which is why
   this never fired in normal use.

2. **A guarded call site is immune.** `if ! run_step …` suspends errexit for
   the entire invocation, so `lib/sync.sh:226` was never exposed. The defect is
   reachable only from **bare** call sites: 5 of 6 in `sync.sh`, all 4 in
   `sync-agent.sh`. That inverts the original expectation — the exposed path is
   the common one.

3. **The restore cannot simply be dropped.** `set` options are shell-global,
   not function-local; verified directly. Removing it would leave errexit
   disabled for the remainder of the script. The fix moves the restore past the
   cleanup, it does not remove it.

## Test-design corrections

Two fixtures were wrong before they were right. Both would have given false
confidence:

1. **The counter stub returned the same path twice.** `VAR="$(mktemp)"` runs
   the stub inside a command-substitution subshell, so an incremented counter
   never escapes. Both capture files got the same name, which made the cleanup
   assertions vacuous — they passed against a block that was in fact stranding
   files. Replaced with `command mktemp "$PROBE_DIR/tempXXXXXX"`.

2. **A shared-directory count is not a leak oracle.** Measuring
   `ls "$TMPDIR" | wc -l` deltas produced `+1` on one helper and `−1` on its
   twin — a negative leak, which is impossible, proving the oracle was reading
   noise. Every leak measurement here now uses an exact set difference of file
   paths. A judge independently flagged the same weakness.

## Earlier test-design correction

`test_mktemp_failure_names_itself` originally pointed `TMPDIR` at a
nonexistent path. macOS `mktemp` **ignores `TMPDIR`** (it uses the Darwin
confstr temp dir), so the test passed without ever exercising the guard. It now
shadows `mktemp` directly, which is portable and actually falsifiable.

## Scope discipline

No change to which steps abort a sync, to the compact/verbose output contract,
or to any message format. `lib/upgrade.sh` was corrected separately under card
#80 and is untouched here.
