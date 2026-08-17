# Verify Report: sync-run-step-errexit

## Verdict

**PASS** — the defect was reproduced before the fix, both helpers were
corrected identically, and the full suite is green on the CLI's hottest path.

## Verify evidence

- Verdict: PASS
- Command: `./tests/validate.sh`
- Exit: 0
- Date: 2026-08-17
- Commit: cd814e7

## Verification summary

- `./tests/validate.sh` — exit 0; **1797 tests**, 0 failures (1787 before this
  change).
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

## Test-design correction

`test_mktemp_failure_names_itself` originally pointed `TMPDIR` at a
nonexistent path. macOS `mktemp` **ignores `TMPDIR`** (it uses the Darwin
confstr temp dir), so the test passed without ever exercising the guard. It now
shadows `mktemp` directly, which is portable and actually falsifiable.

## Scope discipline

No change to which steps abort a sync, to the compact/verbose output contract,
or to any message format. `lib/upgrade.sh` was corrected separately under card
#80 and is untouched here.
