# Judgment ledger: runtime-brief-ownership

**Target (immutable):** `2778f36` on `change/runtime-brief-ownership`, base
`development` (`b3bd5f4`). 20 files, +1248 / −37.

**Round:** 1. Two blind read-only judges, identical scope, launched in parallel.

## Counts

| | |
|---|---|
| Confirmed by both judges | 2 (1 critical, 1 warning) |
| Suspect (one judge) | 5 |
| Contradictions | 0 |

## What the judges confirmed is CORRECT

`make_relative_symlink` is unchanged, so `CLAUDE.md` keeps refusing to replace a
pre-existing regular file. That is the file the original field report was wrong
about, and this change had to leave it alone. It did.

No second classifier was introduced in the write path: the decision calls
`classify_managed_override` (design D1).

## CONFIRMED — both judges

### C1 — the documented remedy did nothing (CRITICAL, both)

The preserve message, `doctor` guidance, and `docs/ai/troubleshooting.md` all
tell the user to run `ai-specs sync --adopt-brief`. `_brief_decision`'s
`user_modified` branch never inspected the flag — only `untracked` did. Running
the exact command the tool printed exited 0, changed nothing, and reprinted the
same warning on every future sync.

Judge B found the sharper framing: the shipped message contradicted the shipped
code. Design D3 forbids *automatic* updates of `user_modified`; D6 states
`--adopt-brief` is safe **because the user issues it**. So the message was right
and the code was wrong.

**Fixed**: `--adopt-brief` now adopts a `user_modified` brief, keeping the
user's bytes and recording them. It never overwrites.

### C2 — doctor contradicted sync (WARNING, both)

`doctor` called `classify_brief`, which returns the raw classification and does
not apply the exact-match short-circuit that lives in `_brief_decision`. For the
common post-upgrade case — no baseline, bytes already identical to our output —
doctor reported `WARN untracked; preserving existing file` and recommended
`--adopt-brief`, while a plain `ai-specs sync` would silently adopt with no
message at all.

**Fixed**: a shared `brief_effective_state()` now answers "the state sync would
act on", and doctor asks that. One decision asked twice, never two decisions.

## SUSPECT — one judge

### S1 — an interrupted write left an ordinary brief stuck forever (Judge B, CRITICAL)

**Verified by the coordinator.** `output_path.write_bytes()` runs *before*
`set_brief_baseline()`. A crash between them leaves disk ahead of the lock, so
the next sync computes a disk hash that mismatches the recorded one and
classifies a never-hand-edited brief as `user_modified` — preserved forever.
Combined with C1, there was no working recovery at all.

**Fixed**: content byte-identical to what we would write is provably ours, so
the baseline is re-recorded. This self-heals and can never adopt foreign
content.

### S2 — the adopt gate disagreed with the classifier on line endings (Judge A: WARNING, Judge B: SUGGESTION)

**Verified by the coordinator**:

```
classifier (sha256_bytes, normalizes CRLF): SAME
adopt gate (raw ==)                       : DIFFERENT
```

A CRLF checkout of our own output was called divergent and preserved — hitting
exactly the no-regression cohort the migration rule exists to protect.

**Fixed**: the adopt gate compares through `normalized_bytes`, the same way the
classifier hashes.

### S3 — `preserve_if_marker` reads as a dead parameter (Judge A)

It is inert **by design**: D5 makes the marker unconditional, so the flag cannot
turn preservation off. Behaviour left unchanged; the module docstring now says
so, because a parameter that looks dead invites someone to "fix" it into
breaking D5.

### S4 — `brief_ownership_state()` duplicated the marker check (Judge A)

No callers, but it was a second definition of ownership — the very thing D1
forbids. Now delegates to `brief_effective_state`.

### S5 — two modified tests do not exercise the path they claim (Judge B)

`test_sync_without_marker_preserves_untracked_agents_md` and
`test_sync_default_render_true_preserves_divergent_untracked_brief` call
`ai-specs init` first, which records a baseline, so they drive `user_modified`
rather than the true no-baseline `untracked` path. They pass because the remedy
text is state-agnostic.

**Recorded, deliberately not fixed this round.** The true first-sight path is
covered directly by the new ownership suite, and rewriting those two end-to-end
fixtures is a larger change than a correction round should carry. Flagged here
so the next person does not mistake their names for coverage.

## Verification after correction

- `./tests/validate.sh` — **exit 0, 1864 tests, 0 failures**, with no `TMPDIR`
  override. The worker reported its own run with `TMPDIR=/tmp` forced; the
  coordinator re-ran without it rather than inherit an environment condition
  that could not be reproduced.
- New ownership suite: 33 tests. Every judgment-day fix has a test that failed
  first.

## Disposition

Round one complete. One finding (S5) is recorded with its reason; nothing else
remains open.

`JUDGMENT: APPROVED ✅`
