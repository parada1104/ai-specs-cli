# Archive Report: decouple-plan-build-readiness

**Archived at**: `openspec/changes/archive/decouple-plan-build-readiness/`
**Mode**: hybrid (openspec filesystem + Engram)
**Change**: decouple-plan-build-readiness (plan-build-flow recipe; two-layer contract: store = persistence preference, readiness = file-backed canonical tree)
**Date**: 2026-08-13

## Final State

This report describes the state of the change AT CLOSE. The change is fully planned, implemented, verified, and archived.

- **Tasks**: 8/8 complete. All `[x]` in `tasks.md` and in the persisted tasks observation #2118.
- **Verification**: `PASS WITH WARNINGS` (verify-report #2128), 0 blockers, 0 CRITICAL findings, 1/1 requirement and 7/7 scenarios compliant.
- **Test counts (final, from verify-report #2128)**: focused `python3 -m unittest tests.test_plan_build_flow_recipe tests.test_plan_build_gate_hook tests.test_premerge_guardian` → 93 tests OK (exit 0); eval offline `./tests/evals/run.sh` → 46 OK / 17 skipped; full `./tests/validate.sh` → 1616 OK (skipped=116), exit 0.
- **Enforcement untouched**: `catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh` and `lib/_internal/premerge_guardian.py` are byte-unchanged (not present in the tracked diff; confirmed via `git status` on this branch).
- **Tracked implementation diff**: 9 modified tracked files, 262 insertions + 16 deletions (278 changed lines). No commit, PR, push, or merge was created.
- **No post-verify fixes**: the final state equals the verified state.

## Spec Sync

The delta (`specs/plan-build-flow/spec.md`) is a single `MODIFIED Requirements` block for "Artifact store degradation and default" with 7 scenarios. Task 4.1 merged it into `openspec/specs/plan-build-flow/spec.md` during apply; apply-progress #2125 records the merge with the delta-only "(Previously:)" annotation stripped per archive convention.

Idempotent parity verified at archive time:

- Main spec contains exactly one `### Requirement: Artifact store degradation and default` (line 313, count = 1).
- Both normative paragraphs match the delta verbatim.
- All 7 delta scenarios are present and match (lines 329–378): Default store with Engram but no preflight; Store selection never changes readiness; Openspec store keeps file-backed enforcement; Engram memory-only cannot satisfy tier minima; Engram mirror cannot satisfy verify evidence; Both store mirrors but never replaces canonical files; No preflight and no Engram fall back to files.
- The delta's "(Previously: …)" change-rationale note is intentionally carried only in the delta (archived with the change), not in the canonical spec, per the apply-time convention recorded in #2125.
- No duplication and no reapply performed: the main spec was left in its final merged form with one correct requirement and all scenarios. Unrelated requirements in the main spec are preserved untouched.

No filesystem write to the main spec was required during archive because the merge had already been completed and validated in apply.

## Warnings and Disposition

Two WARNINGs were recorded in verify-report #2128. Both are historical, non-blocking, and neither is an unresolved blocker at close:

1. **Live runtime eval not executable** (environment-gated): the `required_transcript_any = ["both"]` live LLM-transcript leg of scenario `ac_delivery_contract_artifact_store` was not run live; 17 eval tests skipped. The scenario's `required_content` AGENTS.md needles were proven offline at runtime via materialization + sync. No normative scenario is unproven; the live-transcript leg remains environment-gated and does not block archive.
2. **Stale apply-progress workload count**: apply-progress #2125 reports 218 changed lines (205+13) while the current tracked diff is 262 insertions + 16 deletions (278). The delta is the task-4.1 main-spec merge (+61/−1) performed after that summary was written. Both figures are below the 400-line budget; the single-PR conclusion is unchanged. The persisted apply-progress count is stale, not wrong in outcome.

Two SUGGESTIONs (future integration fixture stubbing an Engram mirror; promoting the offline materialization needle check into `PlanBuildDeliveryContractHermetic`) are recorded in #2128 and remain informational for future work; neither affects this archive.

## Artifact Store

Hybrid: filesystem archive folder (`openspec/changes/archive/decouple-plan-build-readiness/`) is the openspec backend; Engram observation `sdd/decouple-plan-build-readiness/archive-report` is the memory backend.

## Observations Read (traceability)

| Artifact | Engram observation ID |
|----------|----------------------|
| proposal | #2113 |
| spec (delta) | #2114 |
| design | #2116 |
| tasks | #2118 |
| apply-progress | #2125 |
| verify-report | #2128 |
| archive-report | (this observation — see Engram topic `sdd/decouple-plan-build-readiness/archive-report`) |

`reviewGate` was structurally absent in the structured status for this candidate, so no review receipt was required; the change was archived under ordinary repository policy.

## Mechanical Copy Evidence

The archive move was performed with native shell commands (`cp -R` for the pre-move recursive snapshot, `mv` for the folder move; `git mv` was attempted first but the change folder is untracked, so `mv` was used). No artifact content passed through model Read/Write.

Mandatory `diff -r` readback (pre-move recursive snapshot vs. archived folder) — verbatim output:

```
=== diff -r READBACK (source snapshot vs archived folder) ===
DIFF_EXIT=0 (empty diff = PASS)
```

The `diff -r` output was empty (no differences), which is the only passing evidence of byte-identity. A non-empty diff or a skipped diff would have failed the phase; neither occurred. The `archive-report.md` file is additive-only and did not exist in the pre-move snapshot, so it is correctly excluded from the comparison.

## Verification Checklist

- [x] Main specs reflect final merged form (no reapply needed; parity verified)
- [x] Change folder moved to `openspec/changes/archive/decouple-plan-build-readiness/`
- [x] Archive contains all artifacts (proposal, specs, design, tasks, verify-report, exploration)
- [x] Archived `tasks.md` has no unchecked implementation tasks (8/8 `[x]`)
- [x] Active `openspec/changes/` no longer contains this change
- [x] Verbatim `diff -r` readback included above and is empty (PASS)
- [x] No CRITICAL verification issues (0 CRITICAL findings)
- [x] Archive report reflects final state per Final-State Authority hierarchy
