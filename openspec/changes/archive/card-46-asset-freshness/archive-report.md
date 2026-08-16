# Archive Report: card-46-asset-freshness

**Change**: card-46-asset-freshness
**Archived**: 2026-08-15
**Artifact store**: hybrid (OpenSpec + Engram)
**Directory**: `openspec/changes/archive/2026-08-15-card-46-asset-freshness/`
**Status**: `success` — SDD cycle closed.

## Final State (at close)

This report describes the state of the change AT CLOSE, per the Final-State Authority
hierarchy. Intermediate snapshots (`apply-progress.md`, round-1 verify) are historical
input and are not echoed as current facts.

- **Verification**: round 2 re-verification rerun after all Judgment Day corrections.
  Verdict **PASS WITH WARNINGS**; **CRITICAL: None**; `blockers: 0`;
  **4/4 requirements**, **18/18 scenarios** covered by runtime-passing tests.
- **Tests**: `./tests/validate.sh` green — **1683 passed / 0 failed / 116 skipped**
  (documented, pre-existing). Go test/vet/build green; gofmt and `git diff --check` clean.
- **Evidence revision**: `sha256:8b8ad32657a0d76ed8c3638610c88e2fc495eeec6f874204575c0a1c3a75d13a`
  (final round-2 report, byte-identical in OpenSpec and Engram #2214).
- **Tasks**: 53/53 complete, 0 unchecked. No stale-checkbox reconciliation was required
  and none was performed — the persisted tasks artifact already reflected full completion.
- **Implementation included** (final): NUL-safe newline pathname handling in cleanup
  (`candidate_has_combined_tree_equivalence`); fail-closed legacy fallback with atomic
  `.verified` digest receipt; stale receipt invalidation after a failed governed refresh;
  dedicated regression tests for all three.

## Review Authority / Native Review Receipt Gate

- Structured status reported no discovered native review receipt: `reviewGate` was
  **structurally absent** (no `transaction`/`ledger`/`receipt`/`gate-context` artifacts).
  Archive therefore proceeded under ordinary repository policy — a present, non-`allow`
  `reviewGate` was never found, so nothing blocked.
- **Judgment Day** is a separate review method (not the native delivery receipt). It
  reached terminal `JUDGMENT: APPROVED ✅` (final scoped target
  `sha256:62a5ce896f8cf3e7b4f4d8f58283d37f3d58766d72a214003ed54e846afe0447`; both final
  judges reported no severe findings; 2/2 correction rounds and 2/2 scoped re-judgments
  used). It does NOT create a delivery receipt. The only remaining JD item is a
  theoretical/pre-existing suggestion and is NOT a current blocker — it is not reported
  as one here.

## Tasks Completion Gate

Per `openspec/changes/archive/2026-08-15-card-46-asset-freshness/tasks.md` at close:
53 tasks checked `[x]`, 0 `[ ]`. The gate passed; no reconciliation override was needed.

## Specs Synced (Delta → Main)

| Domain | Action | Details |
|--------|--------|---------|
| worktree-flow | Updated (merge into existing main spec) | 2 requirements MODIFIED (Positive Base Candidate Resolution for Merge Detection; Conservative Skip for Dirty Worktrees), 2 added (Forced Latest-Canonical Refresh for Governed Worktree-Flow Assets; Current Gate Asset and Release Freshness). All other requirements preserved byte-for-byte. |

- Main spec: `openspec/specs/worktree-flow/spec.md` (1007 lines after merge; 21 requirements).
- The two trailing delta scenarios ("Canonical preflight precedes project writes" →
  REQ-3; "Version and lock drift is distinguishable" → REQ-4) were folded as `#### Scenario:`
  under their semantically owning requirement, per verify-report SUGGESTION #2, keeping
  scenario counting heading-uniform (0 orphan `### Scenario:` headings remain). This is an
  intentional, verify-endorsed structure normalization; the scenario bytes are preserved.
- The stale dangling `## MODIFIED Requirements` tail in the old main spec was removed.

## Merge Fidelity

- Delta-sourced blocks (REQ-1, REQ-2, REQ-3, REQ-4) are **byte-identical** to their
  delta source (verified by programmatic comparison). Unchanged main requirements are
  preserved byte-for-byte, with a single run of two blank lines normalized to one
  (cosmetic whitespace only; no text altered).

## Archive Contents

- proposal.md ✓
- specs/worktree-flow/spec.md ✓
- design.md ✓
- tasks.md ✓ (53/53)
- apply-progress.md ✓
- verify-report.md ✓
- judgment-ledger.md ✓
- archive-report.md ✓ (this file, additive — excluded from the `diff -r` readback)

Source change folder `openspec/changes/card-46-asset-freshness/` is gone from the active
changes directory.

## Mechanical Readback Evidence

Mandatory recursive `diff -r` (pre-move snapshot vs archived tree): **empty output**, exit 0
(PASS). The change folder was moved with a native shell `mv` (untracked), never through a
model Read/Write path.

## Engram Traceability (observation IDs read/cited)

- #2214 — `sdd/card-46-asset-freshness/verify-report` (final round-2 report,
  evidence revision `sha256:8b8ad…`; read in full)
- #2215 — round-1 verify (historical intermediate; not final state)
- #2216 — Judgment Day approved card 46
- #2223 — Judgment Day correction ledger (round-1 ledger + authorized corrections)
- Filesystem artifacts were read directly from the change folder for merge/validation.

## Intentional Explicit Decisions

- No partial archive (all required artifacts present).
- No intentional-with-warnings override from the orchestrator was needed; the four
  verify WARNINGs are record-keeping format gaps, environment-gated skips, and
  informational fail-closed carry-overs — none blocks archive, none indicates a defect.
- Trailing-scenario folding and blank-line normalization are explicit, recorded here.

## SDD Cycle Complete

The change was fully planned, implemented, verified, and archived.
Ready for the next change.
