# Archive Report: gentle-ai-compatibility-boundary

- **Archived**: 2026-08-14
- **Store mode**: hybrid (OpenSpec filesystem + Engram)
- **Archived to**: `openspec/changes/archive/2026-08-14-gentle-ai-compatibility-boundary/`
- **Engram topic**: `sdd/gentle-ai-compatibility-boundary/archive-report`
- **Branch**: `change/gentle-ai-compatibility-boundary` (worktree `gentle-ai-compatibility-boundary`)
- **reviewGate**: structurally absent — no review was discovered for this candidate; archived under ordinary repository policy.

## Final State (at close)

The change shipped complete. Final verification verdict is **PASS WITH WARNINGS** with **zero CRITICAL** findings and **zero blockers** (per the persisted `verify-report.md`, observation `#2161`).

- 13/13 implementation tasks complete.
- 12/12 requirements, 54/54 spec scenarios have passing runtime covering tests.
- `./tests/validate.sh` ran **1657 tests, 116 skipped, exit 0**; build/syntax checks exit 0.
- Final verify report hash `sha256:49d3364862fd8ba8258b6f44be3ea2cc0408e166005e7718d3f1fa32c3a78bcd`; evidence revision / test hash `sha256:9530a2e481f598d3d9077d96271b291f0ac2ccfda3703fc2207514000362eefe`.

### Verification remediation

During verification a blocker was found and remediated after apply:
- Added `tests/test_override_ownership.py::OverrideOwnershipTests.test_gate_provenance_policy_is_documented`.
- Added one minimal sentence to `catalog/recipes/worktree-flow/README.md`.

The prior FAIL (1 CRITICAL — UNTESTED override-ownership "Policy documentation" scenario) is resolved by that remediation test, which passes at runtime against the actual documentation surfaces. This remediation is recorded as final state; the prior FAIL / stale 1656-test count are not current state.

### Native apply budget rescope

Native apply budget was explicitly rescaled by the maintainer to the actual 1788-line candidate. The remediation added 23 lines, so the final candidate is about **1811 changed lines**, fully covered by the approved single-PR `size:exception`. Delivery strategy remained `exception-ok`.

### Non-blocking warnings carried to archive (documented, not resolved)

- Main-spec promotion during apply (canonical main specs were promoted in `openspec/specs/{worktree-flow,plan-build-flow,override-ownership}/spec.md` during apply).
- Size exception / diff forecast.
- Provenance-model test replacement.
- Sync wrapper forwarding.
- Doctor header-only documentation.
- Intentional `trello-mcp-workflow/README.md` gate-policy alignment.

None of these are CRITICAL or blocking.

## Spec Sync (parity-only)

Canonical main specs were already promoted during apply. Archive performed an **idempotent parity-only sync** — verified that every delta requirement heading is already present in the promoted main spec, and did **not** duplicate requirements.

| Domain | Action | Delta requirements all present in main |
|--------|--------|------------------------------------------|
| `override-ownership` | Parity-only (no re-merge) | 4/4 (Doctor alignment, Gate hook provenance and refresh backup, Policy documentation, Update policy by category) |
| `plan-build-flow` | Parity-only (no re-merge) | 3/3 (Orchestrator-absence degradation, Pre-merge merge guardian, Topology-aware planning artifact root) |
| `worktree-flow` | Parity-only (no re-merge) | 5/5 (Explicit fan-out target semantics, Repo Topology Configuration, Request context owner and planning root separation, Submodule Worktree Creation Contract, Topology-aware gate_scope worktree protection) |

Main specs are the source of truth and already reflect the new behavior.

## Archive Move (mechanical, byte-identical)

Moved `openspec/changes/gentle-ai-compatibility-boundary/` → `openspec/changes/archive/2026-08-14-gentle-ai-compatibility-boundary/` using a pre-move recursive snapshot + shell `mv` (folder untracked) + mandatory `diff -r` readback.

`diff -r` result: **EMPTY** (byte-identical, PASS). Verbatim output was empty; the archived tree is byte-identical to the pre-move snapshot. The `archive-report.md` is additive-only and was written after the readback, so it is excluded from the comparison by construction.

Archived contents: `exploration.md`, `proposal.md`, `specs/{override-ownership,plan-build-flow,worktree-flow}/spec.md`, `design.md`, `tasks.md` (13/13 checked, 0 unchecked), `apply-progress.md`, `verify-report.md`.

## Engram Traceability (observation IDs read)

- exploration `#2138`
- proposal `#2140`
- spec `#2141`
- design `#2142`
- tasks `#2143`
- apply-progress `#2153`
- verify-report `#2161`

## Later-base-advance handoff fact (for the merge step)

The local `development` branch advanced after this feature branch was created: current `development` includes commit `a6519f0 fix(sync): support Bash 3.2 runtimes (#200)`, while this branch is based at `147cf94`. Archive did NOT rebase or merge branches; this fact is recorded for the later orchestrator merge handoff.
