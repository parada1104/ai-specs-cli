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

## Round two — re-judgment after the scope extension

C1 was folded in (plan updated first, commit `6a5acfa`, code in `319e42f`), so
the target changed and the change re-entered judgment against `319e42f`.

### R2-C1 — the fix introduced a regression (both judges)
`lib/sync.sh:215-228` — Judge A: **CRITICAL**, Judge B: WARNING.

Moving the trap registration below all five `mktemp` calls left the first three
temporaries unprotected across two further fallible calls. Under errexit, a
failure at the fourth aborts before the trap exists.

**Verified by execution** — and only on the third attempt. The first two probes
used a counter inside the stub, which never increments across
`VAR="$(mktemp)"` because command substitution runs in a subshell, so no call
ever reached the failing branch and both shapes reported 0 stranded. With a
file-backed counter:

| trap placement | stranded |
|---|---|
| late (as written in `319e42f`) | **3** |
| registered up front | **0** |

A regression I introduced while fixing something else. Corrected by registering
the trap immediately after the first three temporaries and naming all five up
front — safe because every name is `:-` expanded, which was already required
for `set -u`.

**The new test is falsifiable**: reverting to the late-trap shape fails it with
`['tempPth8yb', 'temprmP8AU', 'tempv4Re2T'] != []`; restoring passes.

### R2-C2 — the suite could not have caught it (both judges)
`tests/test_sync_recipe_capture.py` — both WARNING.

`capture_block()` sliced from `RECIPE_OUT_FILE=`, excluding the three earlier
temporaries, so their trap references always expanded to empty strings. Neither
a late trap nor a typo in those three names could fail any assertion. The slice
now starts at the first temp file.

### R2-S1 — the anti-pattern was left live one test below (Judge B only)
`tests/test_sync_recipe_capture.py` — WARNING.

`test_errexit_survives_the_block` rebuilt its own prelude and reintroduced the
counter-in-a-subshell stub **documented as invalid in a comment 70 lines
above**, so both capture files resolved to the same path. Now driven through
`_harness` via a `trailer` parameter, with
`test_capture_files_are_distinct_paths` guarding the fixture itself.

Fixing it exposed a second fixture bug: the assertion used marker `REACHED`,
which is a substring of the block's own `REACHED_END`, so it could never pass.
Marker changed, and falsifiability confirmed — dropping the `set -e` restore
makes `ERREXIT_LEAKED` appear.

### R2-S2 — stale module docstring (Judge A only)
Rewritten to describe the current contract.

## Verification after round two

- `./tests/validate.sh` — **exit 0, 1808 tests, 0 failures**.
- Every new assertion checked for falsifiability by reverting the fix.

## Discovered, recorded, not fixed

- `lib/_internal/recipe-materialize.py:1251` creates `ai-specs-recipe-mcp-*`
  with `mkstemp`, prints the path at 1258, and never removes it — the one
  remaining leaked temp per sync (down from three).
- `lib/sync-agent.sh:134` and `:191` register EXIT traps with bare `$VAR`
  references, the same `set -u` clobber class hardened here.

## Disposition

Two rounds used; the budget is exhausted and no finding remains open.

`JUDGMENT: APPROVED ✅`
