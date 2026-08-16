# Tasks: multi-commit cleanup with forced latest worktree-flow freshness

Depth: full

## Review Workload Forecast

| Field | Value |
|---|---|
| Estimated changed lines | 900-1,500 including tests and docs; confirm after RED evidence |
| 400-line budget risk | High; split cleanup proof from freshness/distribution work if implementation exceeds the configured review slice |
| Chained PRs recommended | Yes if the implementation crosses the review budget; do not mix cleanup algorithm changes with asset-distribution changes without an explicit exception |
| Delivery strategy | ask-on-risk; honor the user-selected force-latest policy and preserve version evidence |
| Decision needed before apply | Yes: authorize this Full plan and resolve the design decision points before production edits |

## Planning and Scope Guard

- [x] 0.1 Confirm the edit root remains `/Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/card-46-asset-freshness` on `change/card-46-asset-freshness` before apply.
- [x] 0.2 Keep all implementation edits under the authorized worktree; do not modify production code, tests, generated assets, consumer projects, or the active `worktree-gate-go` artifacts during this planning attempt.
- [x] 0.3 Record the current path map and stale card wording in the implementation handoff: catalog cleanup template versus materialized override; Go gate source versus cleanup source.
- [x] 0.4 Resolve the seven design decision points from RED fixtures and current source before changing behavior.

## Phase 1 - Cleanup RED and Source Proof

- [x] 1.1 Add a failing real-Git fixture for a clean branch with at least two commits merged by a regular merge; assert removal eligibility and stable output.
- [x] 1.2 Add a failing real-Git fixture for a clean branch with at least two commits squash-merged into the base; assert removal eligibility even though the branch tip is not an ancestor.
- [x] 1.3 Add a failing real-Git partial-squash fixture where one branch commit is not represented in the base; assert `skipped <name> (unmerged)` and branch/worktree preservation.
- [x] 1.4 Add a failing reverted-change fixture whose branch change was later undone in the base; assert preservation as unmerged rather than removal.
- [x] 1.5 Add or strengthen preservation fixtures for dirty worktrees, the main worktree, detached worktrees, branch-ahead/unmerged worktrees, and active/incomplete merge state where the current test helpers support it.
- [x] 1.6 Add topology-protection fixtures for uninitialized modules, explicit standalone behavior, and an out-of-scope initialized submodule; assert no cross-module removal.
- [x] 1.7 Run the focused cleanup tests and capture the exact failing decision point before touching `worktree-cleanup.sh`. Do not substitute a new merge algorithm in the test.

## Phase 2 - Minimal Cleanup GREEN

- [x] 2.2 If Phase 1 proves a failure, correct only the smallest source-derived step in `catalog/recipes/worktree-flow/templates/worktree-cleanup.sh`; preserve candidate order, no-fetch behavior, ancestry-first evaluation, `git cherry` semantics, and stable output.
- [x] 2.3 Verify clean regular/fast-forward/rebase/squash removal and every genuine unmerged/partial/reverted/dirty/main/detached/topology-protected preservation case.
- [x] 2.4 Confirm the dogfood/generated cleanup copy is treated as a materialized target, not a second source of truth. Do not hand-edit it in this change.

## Phase 3 - Freshness RED and Materialization Boundary

- [x] 3.1 Add RED tests for worktree-flow cleanup asset states: managed current, managed stale, user-modified, unknown/untracked, and missing target.
- [x] 3.2 Add RED tests proving ordinary sync/materialization force-replaces stale, user-modified, and unknown cleanup bytes with the latest canonical bytes, records the resulting provenance, reports the replacement, and keeps the old bytes recoverable where supported.
- [x] 3.3 Add RED tests proving the forced policy is scoped to governed worktree-flow assets while unrelated recipe template policies retain their existing behavior.
- [x] 3.4 Add RED coverage for the current direct-write `materialize_legacy_gate` path so an existing user-modified or unknown legacy gate is replaced through equivalent provenance, backup, atomic-write, and rollback handling.
- [x] 3.5 Add RED coverage for a read-only canonical-asset preflight before mutating sync steps, and for the materializer's second state/verification check immediately before a governed replacement.
- [x] 3.6 Define the failure envelope: asset path, state, recorded/current/desired digest where available, CLI/version evidence, replacement/backup result, and the exact retry command.
- [x] 3.7 Add RED coverage for failed digest/version/self-test verification and failed backup, atomic replacement, or lock update; assert fail-closed behavior, no unverified execution, and consistent target/lock rollback.

## Phase 4 - Freshness GREEN and Forced Replacement

- [x] 4.1 Reuse `util.classify_managed_override` and existing `[managed.*]` fields for cleanup and gate evidence; add no parallel ownership manifest and do not change generic recipe policy semantics.
- [x] 4.2 Change the worktree-flow cleanup target path to force the latest verified canonical bytes over stale/unknown/user-modified bytes during ordinary sync, using atomic replacement, provenance update after success, replacement reporting, and backup/rollback where supported. Keep missing-target and managed-current paths idempotent.
- [x] 4.3 Route the materialized legacy Bash gate through equivalent provenance handling, forced canonical replacement, immutable backup, atomic write, verification, and rollback.
- [x] 4.4 Make ordinary sync and `ai-specs sync --refresh-gates` converge on the same forced launcher/gate refresh transaction, including the existing cache-only immutable backup and atomic lock update. The flag remains an explicit retry, not the only replacement path.
- [x] 4.5 Remove the cleanup override's remove-then-sync requirement: ordinary worktree-flow sync must replace the governed target and report the prior bytes/backup location instead of blocking on customization.
- [x] 4.6 Ensure failed verification, backup, replacement, or lock update fails closed, never installs or executes unverified bytes, and leaves the target/lock transaction consistent. Preserve unrelated generic template and lock behavior.
- [x] 4.7 Make `sync.sh` and every current direct materializer entry point perform the same worktree-flow preflight and governed replacement checks before/at the write boundary, without running a live consumer sync in this planning work.
- [x] 4.8 Report each forced replacement with target, prior state/digest, desired state/digest, verification result, and recovery location when available.

## Phase 5 - Gate Asset, Version, and Release Freshness

- [x] 5.1 Add RED tests for an executable version-keyed cache binary whose digest, `--version`, or self-test does not match the current accepted asset; it must trigger forced re-acquisition and must not be treated as current or executed.
- [x] 5.2 Extend `gate_binary.py` only through its existing platform, cache, `SHA256SUMS`, self-test, mismatch-record, and acquisition seams. Revalidate ordinary cache hits, verify local builds before acceptance, preserve atomic temporary installation, and never execute a digest/version/self-test mismatch.
- [x] 5.3 Define and test forced re-acquisition for stale/unknown/mismatched cache bytes during ordinary sync and the explicit gate-refresh path; a cache hit must not silently accept old bytes.
- [x] 5.4 Make launcher resolution reject a cache candidate without current verified evidence before `exec`, while preserving Go gate policy, renderer path, exit codes, and the existing bounded diagnostic verification contract.
- [x] 5.5 Add version/lock evidence tests using the existing `VERSION`, stamped launcher version, version-keyed cache, `SHA256SUMS`, and `.ai-specs.lock [meta].cli_version` semantics. Do not promote unrelated global CLI lock warnings without evidence.
- [x] 5.6 Keep `scripts/build-gate.sh`, `scripts/verify-gate-sums.sh`, release assets, and `.github/workflows/release-worktree-gate.yml` aligned on exact Go toolchain, build flags, target names, tag/version, and committed trust-root entries; reject any mismatched release asset.

## Phase 6 - Doctor, Documentation, and Spec

- [x] 6.1 Make `doctor.py` report non-current or unverified worktree-flow assets with the same target, digest/version/self-test, replacement, and recovery evidence as materialization, without mutating files or seeding provenance; ordinary sync remains the repair path.
- [x] 6.2 Preserve existing doctor semantics for explicit `gate_impl=bash`, valid Go gate, fallback, missing binary, and recorded digest mismatch unless the new freshness evidence requires a narrower worktree-asset error. Do not turn generic recipe warnings into this policy.
- [x] 6.3 Update current-path documentation in `catalog/recipes/worktree-flow/README.md`, `docs/runtime-hooks.md`, and `docs/recipes-catalog.md`; distinguish cleanup Bash ownership from Go gate ownership and document explicit refresh commands.
- [x] 6.4 Add the canonical delta under `specs/worktree-flow/spec.md` for complete multi-commit cleanup proof, preservation cases, forced latest-asset freshness, and release/doctor evidence.
- [x] 6.5 Keep stale card paths and pre-migration wording out of new artifacts; reference `catalog/recipes/worktree-flow/templates/worktree-cleanup.sh` as cleanup source of truth.

## Phase 7 - Verification

- [x] 7.1 Run focused cleanup, materialization, ownership, sync-pipeline, gate-distribution, doctor, release, lock, and CLI-version tests using temporary fixtures only.
- [x] 7.2 Run the configured final validation command: `./tests/validate.sh`.
- [x] 7.3 When the repository-supported Go toolchain is available, run `go -C catalog/recipes/worktree-flow/gate test ./...` and `go -C catalog/recipes/worktree-flow/gate vet ./...`; otherwise record the supported skip and do not claim Go verification.
- [x] 7.4 Confirm no consumer sync, live project mutation, generated asset refresh, commit, push, PR, or Gentle AI invocation occurred during this planning work.
- [x] 7.5 Record RED/GREEN evidence and the final validation result in the implementation/verification artifacts after authorization; planning artifacts alone do not authorize production edits.

## Acceptance Checklist

- [x] Multi-commit regular and complete squash merges remove only clean eligible worktrees.
- [x] Partial, reverted, unmerged, dirty, main, detached, and topology-protected cases are preserved with stable evidence.
- [x] Cleanup remains Bash and its source path is unambiguous.
- [x] Stale, user-modified, and unknown worktree-flow materializations are force-replaced by the latest verified canonical bytes during ordinary sync, with actionable replacement evidence.
- [x] Existing immutable backup/rollback behavior is retained or extended for governed worktree-flow targets where supported; failed verification or replacement fails closed without accepting or executing unverified bytes.
- [x] Version, lock, provenance, digest, release, materialization, and doctor checks agree on the same current asset state.
- [x] Go gate policy and hot-path parity remain unchanged.
- [x] `./tests/validate.sh` passes after authorized implementation, with conditional Go checks recorded when supported.

## Artifact Path

`openspec/changes/card-46-asset-freshness/tasks.md`
