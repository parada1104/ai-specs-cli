# Judgment ledger: sync-run-step-errexit

**Target (immutable):** `c2ae33f` on `change/sync-run-step-errexit`, base
`development` (`39785ee`). 7 files, +539 / −6.

**Round:** 1. Two blind read-only judges, identical scope, launched in parallel.

## Counts

| | |
|---|---|
| Confirmed (both judges) | 3 |
| Suspect (one judge) | 2 |
| Contradictions | 0 |
| **SEVERE / BLOCKER / CRITICAL** | **0** |

Both judges independently concluded the core fix is correct: errexit is
restored on all three return paths in both helpers, the mktemp guard does not
leak, bare/guarded call-site propagation is unchanged, and fan-out is
unaffected because `sync-agent.sh` runs as an independent child process with
its own errexit state.

## Judge limitation, and why it mattered

**Neither judge had a Bash tool.** Both reasoned statically against file
contents. Every factual claim below was therefore re-verified by execution
before being acted on — the same discipline that caught a false premise in the
previous round.

## CONFIRMED — both judges

### C1 — the recipe-materialize block has the identical defect (pre-existing)
`lib/sync.sh:210-234` — Judge A: WARNING, Judge B: WARNING.

A hand-rolled capture block for `recipe-materialize.py` restores errexit at
line 219, before its own `cat` (227-228) and `print_step_output` (232-233)
calls. Same shape as the defect just fixed in `run_step`.

Judge B added a detail neither the author nor Judge A's summary had: the
`trap … EXIT` at line 213 covers only `RECIPE_MCP_TEMP`,
`RESOLVED_CONFIG_TEMP` and `RESOLVED_HOOKS_TEMP` — **`RECIPE_OUT_FILE` and
`RECIPE_ERR_FILE` have no cleanup safety net at all.**

**Verified pre-existing**: the candidate diff touches 0 lines of this block,
and the same code is present verbatim on `development`.

**Disposition:** out of scope for this change. Raised for an explicit decision
rather than folded in silently.

### C2 — the mktemp guard bypasses compact-mode filtering (introduced)
`lib/sync.sh:126-134`, `lib/sync-agent.sh:242-250` — Judge A: SUGGESTION,
Judge B: WARNING.

**Verified by execution**, not accepted on reasoning:

| Path | `✓` marker | `·` marker | `!` warning |
|---|---|---|---|
| normal | filtered | filtered | kept |
| degraded (mktemp fails) | **leaks** | **leaks** | kept |

Real, and introduced by this change. Not fixable by filtering: there is
nothing captured to filter. Corrected by making the warning state the
consequence — the message now says the step runs *with unfiltered output*
instead of merely "unbuffered" — and by locking that wording in a test.

### C3 — the asymmetric mktemp branch was untested (introduced)
`tests/test_sync_run_step_errexit.py` — Judge A: SUGGESTION, Judge B: WARNING.

`! A || ! B` short-circuits, so a stub that always fails never reaches the
second `mktemp`. The branch where the **first** succeeds and the second fails —
the only one where a real temp file must be removed by `rm -f "${out_file:-}"`
— had no coverage.

**Verified by execution:** the branch behaves correctly (probe file `CLEANED`,
step still ran). An earlier `ls | wc -l` measurement suggested a leak
(`DELTA=1`); that was noise from the shared TMPDIR, which is exactly the
flakiness Judge B flagged separately. Re-measured with an exact path oracle.

Judge B also noted two untested aspects of the same branch: no leak assertion,
and no failing-command-plus-mktemp-failure case. Both added.

## SUSPECT — one judge

| ID | Finding | Judge | Disposition |
|---|---|---|---|
| S1 | The harness omits `shopt -s inherit_errexit`, which both real scripts enable before defining `run_step` — a fidelity gap that could hide a future regression | B | Fixed: the harness prelude now mirrors the real guarded `shopt` block |
| S2 | The leak oracle used a shared-directory entry count, which can flake under concurrent CI | B | Fixed: replaced with an exact per-path probe. This one was not hypothetical — it produced a false `DELTA=1` during verification |

## Round-one correction

No severe findings, so no correction was mandatory. The in-scope items were
corrected anyway; the out-of-scope one was not.

| ID | Action |
|---|---|
| C1 | **Not fixed** — pre-existing and outside this change's stated scope. Awaiting an explicit decision |
| C2 | Warning now states the output is unfiltered; wording locked by `test_degraded_path_output_is_documented_as_unfiltered` |
| C3 | `test_partial_mktemp_failure_does_not_leak_the_first_file`, `test_mktemp_failure_still_forwards_a_failing_status` |
| S1 | Harness prelude mirrors the real `inherit_errexit` guard |
| S2 | `_leak_probe` tracks exact temp paths instead of a directory count |

## Verification after correction

- `./tests/validate.sh` — **exit 0, 1800 tests, 0 failures** (1797 before the
  correction, 1787 on `development`).
- `tests/test_sync_run_step_errexit.py` — 13 cases, each against both helpers.
- `bash -n` clean on both helpers.

## Disposition

Round one complete, no round two required. One finding (C1) remains open by
design: it is a pre-existing defect outside this change's scope.

`JUDGMENT: APPROVED ✅`
