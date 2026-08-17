# Judgment ledger: upgrade-experience

**Target (immutable):** `dfdcb37` on `change/upgrade-ux-release-notices`, base
`development` (`7224f8a`). 18 files, +2472 / −17.

**Round:** 1. Two blind read-only judges, identical scope, launched in parallel.
No refuter — two-judge agreement is the corroboration mechanism.

## Counts

| | |
|---|---|
| Confirmed (both judges) | 3 |
| Suspect (one judge) | 6 |
| Contradictions | 0 |
| INFO (WARNING/SUGGESTION, not merge-blocking) | retained below |

## CONFIRMED — both judges

### C1 — `sparse-checkout init --cone` destroys the tree before `set` runs
`lib/_internal/narrow-checkout.sh:95-104` — Judge A: CRITICAL, Judge B: CRITICAL.

`init --cone` immediately narrows the working tree to root-level files. If the
following `sparse-checkout set` fails, `lib/`, `bin/` and `catalog/` are already
gone. The recovery `sparse-checkout disable … || true` discards its own exit
status, and the script prints "restoring the full checkout" and exits 0
regardless of whether recovery worked.

**Independently verified by the orchestrator**, not accepted on the judges'
reasoning: cloning the real repo and running `init --cone` alone took the tree
from **20 top-level entries to 8**. `lib/`, `bin/` and `catalog/` were absent
between the two git calls.

Consequence: contradicts the file's own stated invariant. A failed narrowing can
break the CLI, and `upgrade.sh`'s post-upgrade symlink check would then abort
with exit 5 — narrowing blocking an upgrade, which the design forbids.

### C2 — an already-narrowed install never reconciles a changed `KEEP_DIRS`
`lib/_internal/narrow-checkout.sh:60-64` — Judge A: WARNING, Judge B: CRITICAL.

The short-circuit tests only `core.sparseCheckout == true` and never re-applies
`sparse-checkout set` against the current allowlist. A future release that adds
a top-level runtime directory would never materialize it on existing installs.

Severity is disputed between judges. Treated as severe here: the blast radius is
delayed and silent, and it lands on users who upgraded successfully.

### C3 — clone fallback retries into the same destination
`install.sh:92-95` — Judge A: WARNING, Judge B: WARNING.

Both judges reasoned that a partial-clone attempt failing *after* creating the
destination would make the plain-clone retry fail with "destination path already
exists".

**Partially refuted by the orchestrator.** Both judges reasoned inferentially;
neither tested it. Direct testing shows `git clone` *removes* the destination on
failure — verified for a bad ref and for an unreachable remote, in both cases
the directory was absent afterwards. The hypothesized scenario does not occur
for clean failures. A residual window remains for an interrupted clone
(SIGINT/SIGKILL mid-transfer), where git may not clean up. Worth a cheap
defensive guard; not the defect as described.

## SUSPECT — single judge, verified by the orchestrator

### S1 — test allowlist coverage is incomplete (Judge A)
`tests/test_narrow_checkout.py:25`. **Confirmed factually.**
`RUNTIME = (lib, bin, catalog, bundled-skills, templates)` — 5 entries — while
production `KEEP_DIRS` has 8 (`bundled-commands`, `scripts`, `docs` untested).
The suite does not prove the full production allowlist survives narrowing.

### S2 — duplicate `## [0.12.4]` heading in the real CHANGELOG (Judge B)
**Confirmed factually**: lines 369 and 378 of `CHANGELOG.md` are identical
headings. `parse_sections` does not de-duplicate, so a user crossing 0.12.4
would see that version rendered twice. The duplicate predates this change; the
new parser is what makes it visible.

## SUSPECT — single judge, not independently verified

| ID | Finding | Judge |
|---|---|---|
| S3 | `changelog.py:29-32` — separator class accepts em dash and hyphen but not en dash (U+2013); such a heading silently drops the whole section | B |
| S4 | `upgrade.sh:76-88` — `set -e` re-enabled inside `run_step`; a failing `cat` would abort mid-function, leak both mktemp files, and return bash's status instead of `rc`. Copied from the pre-existing `lib/sync.sh` pattern | B |
| S5 | `changelog.py:33,35` — no fenced-code-block awareness; a `##`-prefixed line inside a code fence would truncate a section early | B |
| S6 | `upgrade.sh:76-77` — `mktemp` runs before the internal `set +e`, so a TMPDIR failure surfaces as a misleading "Failed to fetch" message. Copied from `lib/sync.sh` | A |
| S7 | `changelog.py:298-304` — `main()` reimplements `crossed_notices()` instead of calling it; risks divergence | A |

## Invariants judges actively confirmed as UPHELD

- **No injection path for notice content.** Judge A traced `changelog.py` stdout
  → bash command substitution → `printf '%s\n'` and confirmed no `eval` and no
  unquoted expansion. `_emit_notices` / `_emit_summary` use Python `print()`
  only, never a shell.
- **No runtime-read directory missing from `KEEP_DIRS`.** Judge A cross-checked
  the allowlist against the `bin/ai-specs` dispatch table and the repository's
  top-level tracked directories. None found — which matches the orchestrator's
  independent WU0 end-to-end run.

## Round-one correction (human-approved: confirmed **and** suspect)

Every finding was addressed. RED reproduction first in each case.

| ID | Fix | Evidence |
|---|---|---|
| C1 | Dropped the `init --cone` + `set` split entirely; the allowlist is applied in one `sparse-checkout set --cone` call, so a failure leaves the tree untouched instead of half-pruned. Added post-apply verification that every `KEEP_DIRS` entry present at HEAD exists on disk, and the recovery no longer discards its own exit status — a failed recovery says so and prints the exact repair command instead of claiming success. | `test_collapsed_tree_is_never_reported_as_success` |
| C2 | The short-circuit now compares the *current* sparse list against `KEEP_DIRS` instead of testing `core.sparseCheckout == true`, so an install narrowed by an older allowlist reconciles on the next upgrade. | `test_a_stale_allowlist_is_reconciled`, `test_reconciliation_leaves_the_tree_clean` |
| C3 | Defensive cleanup before the plain-clone retry, with a non-empty guard. Scoped to the branch where `$AI_SPECS_HOME` did not exist beforehand, so only residue from the failed clone is removed. | reasoned; the `elif [ -e ... ]` above the branch makes it unreachable for a pre-existing path |
| S1 | The test allowlist is now parsed out of `narrow-checkout.sh` itself, so coverage cannot drift from production. | `test_allowlist_covers_every_production_keep_dir` |
| S2 | Parser collapses duplicate version headings, **and** the duplicated `## [0.12.4]` heading was removed from `CHANGELOG.md` (its content is preserved verbatim inside the single remaining section). | `test_duplicate_version_headings_are_collapsed`, `test_real_changelog_has_no_duplicate_versions_after_parsing` |
| S3 | Separator class accepts en dash (U+2013) alongside em dash and hyphen. | `test_en_dash_separator_is_accepted`, `test_all_dash_separators_are_accepted` |
| S4 | `run_step` no longer re-enables errexit before the `cat` calls; it is restored only after the temp files are removed, so a failing `cat` cannot abort mid-function, leak temp files, or mask the wrapped command's `rc`. | reasoned; covered indirectly by the 70 upgrade tests |
| S5 | Fenced-code-block tracking added to both section boundaries and notice extraction. | `test_heading_inside_a_code_fence_is_not_a_section_boundary`, `test_notice_is_not_truncated_by_a_fenced_heading` |
| S6 | `mktemp` failure is detected and named ("cannot create temporary files (check TMPDIR)") instead of surfacing as the wrapped command's abort message; the step still runs, unbuffered. | reasoned |
| S7 | `main()` delegates to a shared `_notices_for()` helper; `crossed_notices()` uses the same helper. | `test_cli_notices_branch_uses_the_shared_helper`, `test_cli_and_helper_agree` |

### Correction on C3

Judge A and Judge B agreed on the *finding* but their shared premise was wrong.
Both reasoned that a failed clone leaves the destination behind. Direct testing
shows `git clone` removes it — verified for a bad ref and an unreachable remote.
The guard was still added, because an interrupted transfer is a real residual
window, but the described failure does not occur for clean failures.

Recorded because it generalizes: **two judges agreeing is corroboration of
attention, not of fact.** Factual premises still need testing.

## Verification after correction

- `./tests/validate.sh` — **exit 0, 1787 tests, 0 failures** (1773 before the
  correction round, 1684 on `development`).
- All 70 upgrade-related tests pass.
- Real output re-rendered after the parser and CHANGELOG changes; unchanged and
  correct.

## Disposition

Round one complete, no round two required. No finding remains open.

`JUDGMENT: APPROVED ✅`
