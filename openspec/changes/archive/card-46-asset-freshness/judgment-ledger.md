# Judgment Ledger: card-46-asset-freshness

**Mode**: judgment_day  
**Artifact store**: hybrid (OpenSpec + Engram)  
**Round budget**: 3 total rounds (initial sweep plus at most two scoped re-judgments)  
**Fix budget**: 2 bounded correction rounds

## Round 1 — Initial Blind Sweeps

### Frozen target

- **Target**: `sha256:816e0e77f5840fe741b0418b806577502b0b6703831dd91c549373a50020be25`
- **Base**: `1db6e210d9d85466cb2de4fcc305e3e6b973f7a0`
- **Candidate paths**: 28

### Judge A (`jd-judge-a`)

| ID | Severity | Location | Causal disposition | Claim |
|---|---|---|---|---|
| JD-A-001 | WARNING | `lib/_internal/recipe-materialize.py:1296` | introduced | The new sync preflight invokes gate acquisition without the materialization seam's warning-only exception boundary, so some network/cache I/O exceptions abort sync before consumer writes. |
| JD-A-002 | SUGGESTION | `tests/test_gate_binary_dist.py:296` | unknown | The acquisition-never-raises test returns before exercising the download exception path because the fixture has no `SHA256SUMS`. |
| JD-A-003 | SUGGESTION | `lib/_internal/gate_binary.py:286` | unknown | Duplicate digest-reason branches are unreachable after earlier returns. |
| JD-A-004 | SUGGESTION | `lib/_internal/doctor.py:1153` | introduced | Missing `VERSION` fallback differs between doctor launcher rendering and materialization, producing non-convergent freshness evidence in a partial install. |
| JD-A-005 | SUGGESTION | `tests/test_worktree_flow_recipe.py:143` | unknown | The read-only preflight test may mutate repository-local cache evidence while running in offline/local-build conditions. |
| JD-A-006 | SUGGESTION | `lib/_internal/recipe-materialize.py:721` | introduced | The generic materializer docstring still describes preserve-and-warn behavior while the worktree-flow branch force-replaces governed assets. |

### Judge B (`jd-judge-b`)

| ID | Severity | Location | Causal disposition | Claim |
|---|---|---|---|---|
| JD-B-001 | CRITICAL | `catalog/recipes/worktree-flow/templates/worktree-cleanup.sh:255` | introduced | Non-NUL Git path output can make a genuinely unmerged branch with a newline-containing pathname appear tree-equivalent and eligible for deletion. |
| JD-B-002 | CRITICAL | `lib/_internal/recipe-materialize.py:908`; `catalog/recipes/worktree-flow/hooks/worktree-gate.sh:174-183` | introduced | A failed legacy-gate replacement can leave prior bytes executable through the launcher fallback without current provenance validation. |
| JD-B-003 | WARNING | `lib/_internal/recipe-materialize.py:557` | introduced | The direct materialization path can force-install malformed worktree-flow bytes while only checking byte readback, not syntax/source verification. |
| JD-B-004 | WARNING | `lib/_internal/gate_binary.py:232` | introduced | Malformed non-UTF-8 cache receipts or trust-root files can escape decoding and terminate doctor rather than produce actionable verification evidence. |

### Round 1 disposition

The judges contradicted on severe findings: Judge A found no critical finding; Judge B found JD-B-001 and JD-B-002. Per the human decision, both Judge B critical candidates were authorized for bounded correction. No other finding was authorized for correction. Judge A's warning and all suggestions, plus Judge B's warnings, remain outside the fix scope.

## Correction 1 — Authorized findings JD-B-001 and JD-B-002

### JD-B-001 fix work unit

- Replaced newline-unsafe `git diff --name-only` consumption with NUL-delimited `git diff --name-only -z` and a process-substitution read loop in `candidate_has_combined_tree_equivalence`.
- Preserved Bash 3.2 compatibility, conservative empty-path behavior, and the existing ancestry/patch-id proofs.
- Added `test_preserves_newline_pathname_worktree` as a real-Git regression test.

### JD-B-002 fix work unit

- Added an atomic `.verified` legacy-hook sidecar containing the catalog-byte digest after successful materialization.
- The launcher recomputes the legacy-hook digest and refuses to execute missing, stale, or mismatched provenance, failing closed instead.
- Preserved valid managed fallback behavior and transactional rollback; added launcher and materialization regression tests.

### Correction evidence

- Focused correction tests: `python3 -m unittest tests.test_worktree_cleanup tests.test_worktree_gate_hook tests.test_worktree_gate_dist_config -q` — 234 tests passed, 94 skipped.
- Broader correction tests: 318 tests passed, 98 skipped.
- Full Python suite: `python3 -m unittest discover -s tests -p 'test_*.py'` — 1682 tests passed, 116 skipped.
- Go gate tests passed; Bash syntax and Python compilation passed.
- `git diff --check` passed.
- No commit, push, archive, PR, consumer sync, or review lifecycle was run.

## Round 2 — Scoped Re-judgment

**Scoped target**: `sha256:14113aa8f91b2f0552c0775cfa4373000127ab9d25b9d2779fa6de366fb28f2d`

### Judge A (`jd-judge-a`)

- JD-B-001: fixed; no severe finding in the scoped delta.
- JD-B-002: the unverified-fallback execution path is fixed; no severe finding in the scoped delta.
- Additional WARNING: the receipt digest uses normalized hashing while the launcher verifies raw bytes, so CRLF materialization can fail closed.
- Additional WARNING: delivery of the corrected launcher without its sidecar causes a fail-closed enforcement gap until sync runs.

### Judge B (`jd-judge-b`)

- JD-B-001: fixed; no severe finding in the scoped delta.
- JD-B-002: **CRITICAL remains**. A failed refresh can leave an old matching `.verified` receipt beside restored old bytes; the launcher then accepts and executes stale legacy bytes because the receipt is not invalidated or generation-bound to the current catalog.

### Round 2 disposition

The judges still contradict on JD-B-002: Judge A considers it fixed, while Judge B reports a deterministic remaining CRITICAL. Per Judgment Day rules, the parent cannot classify this as approved. Human decision is required before any final bounded correction or escalation. No third-round correction has started.

**Terminal state**: `blocked_by_contradiction`

## Round 3 — Final Scoped Re-judgment

**Scoped target**: `sha256:62a5ce896f8cf3e7b4f4d8f58283d37f3d58766d72a214003ed54e846afe0447`

### Final correction

- Added `_invalidate_legacy_verification` to remove the stale legacy `.verified` sidecar when a governed legacy refresh fails after target/lock rollback.
- Added `test_failed_legacy_refresh_invalidates_stale_receipt`, which proves the prior target is restored while the stale receipt is absent.
- Valid managed-current and successful-refresh paths continue to write a fresh receipt.

### Judge A (`jd-judge-a`)

- JD-B-001: fixed; no severe finding.
- JD-B-002: fixed; no severe finding.
- One theoretical SUGGESTION about a process-kill window was classified pre-existing, not candidate-caused.

### Judge B (`jd-judge-b`)

- JD-B-001: fixed; no severe finding.
- JD-B-002: fixed; no severe finding.
- No additional findings.

### Final Judgment Day disposition

- Confirmed severe findings: 0
- Suspect severe findings: 0
- Contradictions: 0
- Informational findings: 1 pre-existing/theoretical suggestion; not blocking
- Correction rounds used: 2 of 2
- Scoped re-judgments used: 2 of 2

**Terminal state**: `approved`

**JUDGMENT: APPROVED ✅**
