# [Go 07.S15] Frozen judgment ledger

Status: FINAL BOUNDED REMEDIATION ROUND IMPLEMENTED AND VERIFIED (F1–F5,
verifier muue9ehz-7-1l8i VERDICT PASS: validate.sh EXIT:0, parity PASS 28
fixtures × both gate modes, 56 zero-delta lines). TERMINAL DISPOSITION:
JUDGMENT ESCALATED for delivery (both re-judgment rounds complete, correction
budget exhausted, no severe) — the escalated state remains ON 587f64f. A
SEPARATE, DISTINCT human MAINTAINER-CLOSURE authorization (2026-10-04) permits
a narrow maintenance round (errno subclasses, manifest-path contract pin,
description truth, docs reconciliation) — it is NOT a new JD approval, NOT a
native receipt, and does NOT reopen or reset the exhausted judgment.
SUBSEQUENT human acceptance (2026-10-05): the manifest fatal-read/parse
portable diagnostic framing was explicitly accepted and recorded as a narrow
TOLERANT contract entry (manifest fatal-error framing); the arg-contract
fixture descriptions were corrected to laude2 (the TOML escape \u001c
consumes the C, so the separator entry strips to laude2, not claude2). The
ESCALATED disposition remains ON 587f64f for the JD history; these are
distinct maintenance acceptances, not JD outcomes. This
document records the review/judgment facts of the slice; it is frozen history,
not a task list.

## Commit identity

| Stage | Commit | Content |
| --- | --- | --- |
| Original frozen candidate | `d8f8c1a` | feat(sync): implement native sync-agent fan-out |
| Final corrected delta | `5a6d365` | fix(sync): preserve residual sync-agent parity |
| Base chain | `0db6c2f` → `d8f8c1a` → `5a6d365` | epic tip (S9) onward |

## Native review disposition (factual, no approval claimed)

- Native review was NEVER completed for any candidate of this slice: two START
  attempts (batch 2 era) failed with `consent-binding-stale` /
  `native_invocation_attempted:false` / `lineage_created:false`; NO lineage
  exists; nothing was approved, acknowledged, burned, abandoned or reset.
- The human's standing disposition: "No new native retry loop" — review
  authority is exercised through the parent-owned JD dual and the independent
  verifier fallback instead.
- Verification evidence: verifier runs (batch 1: mut5btdc-6; batch 2 final:
  mutzpuv2-6-936v, VERDICT PASS) — tooling runs, NOT native approval.

## Re-judgment findings (bounded scope: prior findings + delta)

- Judge A (`muu1bnkr-c-n4o4`) and Judge B (`muu1bng3-b-uv3e`): BOTH scoped
  judges CONFIRM the following (no severity agreement is fabricated here; the
  human authorized this final bounded round on their joint confirmation):
  1. T15 fixture data absent: the arg-contract fixture DESCRIPTION claimed
     U+001C-prefix and embedded-newline agent entries, but the manifest did
     not contain them (false coverage claim).
  2. removeRf loses the ReadDir/child-failure rc: a ReadDir error is printed
     but the function can still return 0 when the final remove succeeds;
     unreadable-dir handling must match the measured rm status/tree/error
     ordering.
  3. MCP stderr classification gap: the unsized-type death diagnostic
     ("object of unsized type") was invented without meaning and the
     IsADirectoryError/read-error class was not named; docs staleness.
- Original JD-A-001..A004 (first re-judgment round) were FIXED and verified
  (see odd/tasks/go-07-s15.md, batch evidence); A005 was resolved by human
  decision (narrow TOLERANT, locale ordering).

## Human decisions recorded

- A005 locale ordering: narrow TOLERANT accepted (contract §10).
- Resolver-plan traceback framing: stable portable Go diagnostics; pinned
  local-Python traceback constants rejected; narrow TOLERANT accepted
  (contract, resolver-plan diagnostic framing).
- This final bounded round: fix ONLY F1–F4, verify (F5), then HOLD for parent
  commit. Explicit follow-ups (NOT fixed): NUL/Bash-version behavior,
  malformed-resolver non-string/missing-targets. execNested deletion stays S16.
- Both judges saw no severe. NO native approval exists for any candidate of
  this slice; delivery is the parent's serial action onto epic 2451ee2.
