# Proposal: multi-commit worktree cleanup and forced asset freshness

- **Change slug**: `card-46-asset-freshness`
- **Depth**: Full
- **Baseline**: `1db6e210d9d85466cb2de4fcc305e3e6b973f7a0` (`development` lineage)
- **Worktree**: `/Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/card-46-asset-freshness`
- **Branch**: `change/card-46-asset-freshness`

## Executive Summary

Plan card #46 around the current Bash `worktree-cleanup.sh` implementation. The
cleanup script remains the implementation of record for removing merged
worktrees. Its existing ordered base-ref search, ancestry check, and
`git cherry` patch-id check must prove the complete branch change set before a
worktree or branch can be removed. A multi-commit squash merge is an explicit
regression case, not a reason to port cleanup into the Go `worktree-gate`.

The same change must establish an authoritative latest-canonical refresh policy
for the worktree-flow assets that make cleanup and gate behavior trustworthy.
The existing lock-backed classifier remains evidence about the bytes on disk,
but stale, unknown, and user-modified governed assets are replaced by ordinary
sync/materialization with the latest verified canonical bytes. Replacements are
atomic, report the prior and new state, and use the existing immutable
cache-only backup and rollback mechanisms where available. A failed digest,
version, self-test, backup, write, or lock update fails closed; an unverified
asset is never accepted or executed. The explicit refresh flag remains a
supported retry/diagnostic path, not a prerequisite for canonical replacement.

## Why

The card reports that `worktree-cleanup.sh` fails to detect squash merges for
branches with multiple commits. The current catalog template already handles
regular, fast-forward, rebase, and single-commit squash examples. Its
`candidate_has_patch_equivalence` function uses `git rev-list` and `git cherry`
over the branch tip, so the first implementation task is to prove the actual
multi-commit behavior and identify the smallest source correction only if the
hermetic fixture fails. No replacement heuristic may be designed from the
card wording alone.

The current source also contains freshness behavior that is not hard enough for
this safety-sensitive recipe:

| Surface | Current source and behavior | Planning consequence |
|---|---|---|
| Cleanup source | `catalog/recipes/worktree-flow/templates/worktree-cleanup.sh` owns candidate enumeration, merge proof, skip decisions, and deletion. | Keep cleanup in Bash and preserve its stable output contract. |
| Cleanup materialization | `recipe.toml` materializes the template to `ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh` with `condition = "not_exists"`; the default template policy is `auto`. | Keep classification for evidence, but route this exact governed target through forced latest-canonical replacement during ordinary sync, with atomic write, reporting, and backup/rollback where supported. |
| Gate migration | `catalog/recipes/worktree-flow/gate/` is the Go source for `worktree-gate`; `hooks/worktree-gate.sh` is a stamped launcher and `hooks/worktree-gate-legacy.sh` is the rollback reference. | Do not move cleanup logic into Go. Apply freshness work only to the gate distribution/materialization seams. |
| Lock/provenance | `ai-specs/.ai-specs.lock` stores `[managed.*]` hashes, source, kind, and policy. `util.classify_managed_override` distinguishes missing, current, stale, user-modified, and untracked states. | Reuse these facts and do not invent an unrelated manifest. |
| Gate refresh | `recipe-materialize.py` currently leaves a modified or unknown launcher untouched during ordinary sync and supports `ai-specs sync --refresh-gates` with a cache-only immutable backup. | Make ordinary sync and the explicit flag use the same forced replacement transaction, and give the materialized legacy gate equivalent provenance, backup, and rollback treatment. |
| Gate binary | `gate_binary.py` verifies downloaded assets against `catalog/recipes/worktree-flow/bin/SHA256SUMS`, but an existing executable cache hit is currently accepted without re-checking current digest/version/self-test. | Revalidate every cache candidate at acquisition/materialization/doctor boundaries, force re-acquisition for stale or unknown bytes, and reject execution until digest, version, and self-test checks pass. |
| Release trust root | `scripts/build-gate.sh`, `scripts/verify-gate-sums.sh`, and `.github/workflows/release-worktree-gate.yml` build the four-target matrix and compare canonical digest entries. | Keep release checks tied to the existing `VERSION`, toolchain pin, asset names, and committed sums. |
| Version evidence | Repository `VERSION` is `0.22.0`; this worktree's dogfood lock records `[meta].cli_version = "0.21.0"`. | Surface a stale version/lock relationship as actionable freshness evidence; do not silently rewrite the lock during planning. |
| Doctor | `doctor.py` reports gate implementation health, stale template overrides, and gate provenance, but these paths currently warn in several stale/unknown cases. | Make the diagnostic use the same target, version, digest, replacement, and failure evidence as materialization while remaining read-only and scoped to worktree-flow. |

## Current Path Map

The card's shorthand must be resolved against current repository paths before
implementation:

- **Catalog cleanup source**: `catalog/recipes/worktree-flow/templates/worktree-cleanup.sh`.
- **Dogfood/generated cleanup target**: `ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh`.
- **Materializer and lock**: `lib/_internal/recipe-materialize.py`, `lib/_internal/util.py`, and `lib/_internal/lock.py`.
- **Go gate source**: `catalog/recipes/worktree-flow/gate/`.
- **Go gate launcher and frozen Bash fallback**: `catalog/recipes/worktree-flow/hooks/worktree-gate.sh` and `catalog/recipes/worktree-flow/hooks/worktree-gate-legacy.sh`.
- **Gate cache and trust root**: `lib/_internal/gate_binary.py` and `catalog/recipes/worktree-flow/bin/SHA256SUMS`.
- **Release checks**: `scripts/build-gate.sh`, `scripts/verify-gate-sums.sh`, and `.github/workflows/release-worktree-gate.yml`.

Older card or plan wording that treats `worktree-gate` as the cleanup
implementation, names a pre-Go launcher as the current hook, or treats a
consumer copy as the source of truth is stale. The active `worktree-gate-go`
artifacts describe the already-landed Go migration and must not be duplicated
as a new cleanup port.

## Goals

- Prove and, if required, minimally correct clean multi-commit regular and
  squash-merge cleanup using the current Bash decision points.
- Preserve clean eligible removal of worktrees and branches after regular,
  fast-forward, rebase, or complete squash integration.
- Preserve genuine unmerged, partially integrated, reverted, dirty, main,
  detached, uninitialized, out-of-scope, and topology-protected worktrees.
- Preserve ordered base candidate resolution and the no-fetch boundary.
- Make governed worktree-flow cleanup and gate assets converge to the latest
  verified canonical bytes during ordinary sync/materialization, with actionable
  replacement evidence.
- Keep stale, unknown, and user-modified bytes recoverable where the current
  cache-only backup mechanism supports it, but do not let them block canonical
  replacement. `--refresh-gates` remains an explicit retry path.
- Keep Go gate parity, launcher path stability, version stamps, lock
  provenance, digest trust, release checks, materialization, and doctor output
  aligned with the current architecture.

## Non-Goals

- Porting `worktree-cleanup.sh` to Go or changing the Go gate's write-policy
  heuristics.
- Replacing the current `git cherry`/patch-id mechanism with a new merge
  algorithm before a failing multi-commit fixture proves that a change is
  needed.
- Fetching refs or introducing network access into cleanup.
- Changing generic recipe ownership policy for unrelated recipes; forced
  replacement is limited to worktree-flow assets and gates governed by this
  change.
- Accepting or executing an unverified cleanup/gate asset.
- Changing all CLI version policy semantics merely because this worktree has a
  stale dogfood lock timestamp/version.
- Running consumer sync, changing production code/tests/generated assets, or
  invoking Gentle AI as part of this planning attempt.

## Safety Contract

Cleanup may remove a worktree and branch only after all of these hold:

1. The candidate is a linked worktree under the configured directory and is in
   the active topology/scope.
2. It is not the main worktree, detached, dirty, in an active merge, or from an
   uninitialized/unproven topology.
3. An ordered local base candidate proves either ancestry or complete
   patch-id equivalence for every unique branch commit.
4. The result is not merely a partial squash, a reverted change, or an
   unmerged branch.

Freshness may not weaken those conditions. For the governed worktree-flow
assets, stale, unknown, or user-modified bytes must not block ordinary sync:
ordinary sync/materialization must back them up where supported, atomically
replace them with the latest verified canonical bytes, update provenance only
after verification, and report what changed. A failed verification, backup,
replacement, or lock transaction fails closed and leaves no unverified asset
eligible for execution. The Go gate's decision policy remains unchanged; any
runtime fallback must be a separately verified canonical implementation, not a
way to accept a stale or unknown Go binary.

## Tracker

- **card_id**: `6a6199ea05db9d700a1e1797`
- **shortLink**: `wX8Z2O7t`
- **url**: https://trello.com/c/wX8Z2O7t/46-worktree-cleanupsh-detecci%C3%B3n-de-squash-merge-falla-con-branches-multi-commit

## Implementation Shape

1. Freeze the current source-derived decision points in RED tests, including a
   two-or-more-commit squash fixture and the required preservation cases.
2. Add only the smallest Bash correction if the fixture demonstrates a real
   failure; otherwise keep the algorithm and record the result as a regression
   proof.
3. Add a worktree-flow freshness preflight and materialization policy using the
   existing lock/provenance classifier. The preflight must verify the latest
   canonical bytes/assets before ordinary sync writes where the current sync
   pipeline permits it; the materializer must repeat the state and verification
   check immediately before replacing a target.
4. Extend forced latest-canonical replacement and freshness evidence to the
   stamped launcher, legacy gate materialized beside it, and version-keyed Go
   asset acceptance without changing the gate decision algorithm or cleanup
   decision algorithm.
5. Make doctor and release checks report the same target, version, provenance,
   digest, and remediation evidence used by materialization.
6. Update the canonical worktree-flow delta and user-facing documentation with
   current paths and explicit refresh commands.

## Verification Baseline

The configured repository validation command is `./tests/validate.sh`. The
existing Go module supports conditional checks at
`catalog/recipes/worktree-flow/gate/`, so implementation verification may also
run `go -C catalog/recipes/worktree-flow/gate test ./...` and
`go -C catalog/recipes/worktree-flow/gate vet ./...` when `go` is available.
No live consumer sync is part of this plan; use hermetic temporary fixtures and
the repository's existing tests.

## Risks

- `git cherry` can appear to prove a branch when a fixture does not represent a
  complete squash; tests must assert the exact commit set and expected output.
- A partial or reverted fixture that is accidentally treated as merged could
  delete user work, so negative cases are release-blocking.
- Forced replacement can destroy legitimate local customizations if the
  immutable backup or rollback path is incomplete; diagnostics must name the
  replacement, target, observed/expected evidence, and recovery location.
- The gate release digest is toolchain-sensitive; `VERSION`, the pinned Go
  release, build flags, published asset names, and `SHA256SUMS` must move
  together.
- The dogfood lock is intentionally not rewritten during this planning retry;
  the observed `0.21.0` versus `0.22.0` relationship must remain visible as a
  testable stale-state case.

## Artifact Path

`openspec/changes/card-46-asset-freshness/proposal.md`
