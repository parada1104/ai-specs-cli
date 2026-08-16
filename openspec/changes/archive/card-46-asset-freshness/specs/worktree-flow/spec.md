# Delta for worktree-flow

## MODIFIED Requirements

### Requirement: Positive Base Candidate Resolution for Merge Detection

The system MUST treat a worktree branch as removable only when an ordered local
base candidate proves either ancestry or complete patch-id equivalence for the
branch's unique commits. Complete patch-id equivalence MUST account for every
commit on a multi-commit branch. A base that represents only a subset of the
branch changes MUST remain unmerged. A later revert of the branch changes MUST
not be treated as proof that the branch is merged.

The existing candidate order remains authoritative: exact `--base`, configured
upstream, configured remote-tracking ref, and the conditional `origin/<base>`
fallback only when the configured remote-tracking ref does not resolve. The
cleanup MUST use only local refs and MUST NOT fetch. The implementation MUST
preserve the current ancestry-first and `git cherry` patch-id decision points;
this requirement does not authorize a new merge heuristic.

#### Scenario: Multi-commit regular merge is eligible

- GIVEN a clean feature worktree whose branch contains at least two commits
- AND the complete branch tip is integrated into the selected base by a regular
  merge or fast-forward
- WHEN `worktree-cleanup.sh --base <base> --dry-run` runs
- THEN it MUST report `would remove <name>`
- AND the branch and worktree MUST be removable in normal mode

#### Scenario: Multi-commit squash merge is eligible

- GIVEN a clean feature worktree whose branch contains at least two commits
- AND the feature changes are integrated into the selected base as one or more
  new squash commits
- AND the original branch tip is not an ancestor of the base
- WHEN cleanup evaluates the branch
- THEN complete patch-id equivalence MUST prove the branch as merged
- AND dry-run MUST report `would remove <name>`

#### Scenario: Partial squash is preserved

- GIVEN a feature branch contains at least two commits
- AND the selected base represents only a strict subset of those changes
- WHEN cleanup evaluates the branch
- THEN it MUST report `skipped <name> (unmerged)`
- AND it MUST preserve both the worktree and branch

#### Scenario: Reverted change is preserved

- GIVEN a feature branch's changes were integrated into the base and later
  reverted so the branch's complete patch is no longer present
- WHEN cleanup evaluates the branch
- THEN it MUST report `skipped <name> (unmerged)`
- AND it MUST preserve both the worktree and branch

### Requirement: Conservative Skip for Dirty Worktrees

The system MUST preserve dirty, main, detached, unmerged, and topology-protected
worktrees before any removal. Dirty status MUST be checked before merge proof.
The main worktree MUST never be removed even when its branch is fully merged.
Detached worktrees under the configured directory MUST be reported as detached
and preserved. Under topology-aware cleanup, uninitialized modules, explicit
out-of-scope modules, and unproven relationships MUST not become removal
candidates.

#### Scenario: Detached worktree is preserved

- GIVEN a detached worktree exists under the configured worktree directory
- WHEN cleanup runs
- THEN it MUST report `skipped <name> (detached)`
- AND it MUST not remove the worktree or any branch

#### Scenario: Main worktree is never removed

- GIVEN the main repository worktree is on a protected or integration branch
- WHEN cleanup runs from the repository root
- THEN it MUST not report the main worktree as removable
- AND it MUST leave the main worktree unchanged

#### Scenario: Topology-protected worktree is preserved

- GIVEN a worktree belongs to an uninitialized, unproven, or explicitly
  out-of-scope submodule topology
- WHEN cleanup runs from the superproject or with a different module scope
- THEN it MUST not scan that worktree as an eligible candidate
- AND it MUST preserve the worktree and branch

## ADDED Requirements

### Requirement: Forced Latest-Canonical Refresh for Governed Worktree-Flow Assets

The worktree-flow cleanup override, generated Go launcher, and materialized
legacy gate MUST be classified using the existing lock-backed provenance and
current would-write bytes before replacement or execution. A managed-current
asset MAY be used without rewriting after its current bytes remain verified. A
missing asset MAY be materialized and recorded. A managed-stale,
user-modified, or unknown/untracked governed asset MUST be force-replaced by
the latest verified canonical bytes during ordinary sync/materialization.

The operation MUST use an existing immutable cache-only backup and rollback
mechanism where supported, write the replacement atomically, verify the
installed bytes, and update provenance only after replacement succeeds. It MUST
report the exact project-relative target, prior classified state, observed and
desired digests when available, relevant recipe/source, replacement result, and
backup/recovery location when one exists. Unknown or user-modified bytes are
recoverable evidence, not a reason to block or defer the canonical update.

If canonical verification, backup, replacement, rollback, or lock update fails,
the operation MUST fail closed, leave the target and lock consistent, and MUST
NOT accept or execute an unverified asset. `ai-specs sync --refresh-gates` MUST
use the same forced replacement transaction as ordinary sync; it is an
explicit retry/diagnostic path, not the only replacement path. Doctor MUST use
the same read-only classification and verification evidence without mutating
the project.

This requirement applies only to worktree-flow assets. It MUST NOT change
generic template ownership policies for unrelated recipes.

#### Scenario: Stale cleanup override forces verified replacement

- GIVEN the materialized cleanup override matches its recorded managed digest
- AND the catalog would-write bytes have changed
- WHEN ordinary sync or materialization runs
- THEN the materializer MUST back up the prior bytes where the existing cache
  mechanism supports it
- AND it MUST atomically replace the override with the verified catalog bytes
- AND it MUST update the managed lock entry only after the replacement verifies
- AND the operation MUST report the prior state/digest, desired digest, and
  replacement/backup result

#### Scenario: Unknown cleanup override forces canonical ownership replacement

- GIVEN a cleanup override exists with no managed lock entry
- AND its bytes diverge from the current catalog would-write bytes
- WHEN ordinary sync runs
- THEN ordinary sync MUST replace it with the verified catalog bytes
- AND it MUST seed the managed entry from the installed canonical bytes only
  after successful replacement
- AND the result MUST identify unknown provenance, the observed digest, and the
  replacement/backup result
- AND doctor MUST remain read-only and report that ordinary sync will perform
  the forced replacement

#### Scenario: Customized gate is force-replaced by ordinary sync

- GIVEN a materialized `worktree-gate.sh` or legacy gate differs from its
  recorded baseline or has no baseline
- WHEN ordinary sync or `ai-specs sync --refresh-gates` runs
- THEN the pre-refresh bytes MUST be saved through the existing cache-only
  immutable backup mechanism where that mechanism applies
- AND the gate or legacy fallback MUST be atomically replaced with verified
  canonical bytes
- AND its baseline/lock evidence MUST be updated only after replacement succeeds
- AND the operation MUST report the replacement rather than block on the local
  customization

#### Scenario: Current worktree-flow assets remain idempotent

- GIVEN the cleanup override and gate assets match their current recorded
  provenance and expected bytes
- WHEN ordinary sync or doctor runs
- THEN no freshness warning or hard failure MUST be emitted
- AND no asset MUST be rewritten

#### Scenario: Failed canonical verification fails closed

- GIVEN a worktree-flow asset or version-keyed Go cache candidate is stale,
  mismatched, or unknown
- AND the latest canonical bytes fail digest, version, or self-test verification
- WHEN ordinary sync, materialization, or acquisition evaluates it
- THEN no unverified bytes MUST be accepted or executed
- AND the operation MUST report the target/cache path and expected/observed
  verification evidence
- AND any prior bytes MUST remain recoverable or quarantined without being
  selected as the current verified asset

#### Scenario: Failed replacement rolls back governed state

- GIVEN a stale, user-modified, or unknown governed asset is selected for forced
  canonical replacement
- AND its backup, atomic write, verification, or lock update fails
- WHEN the replacement transaction runs
- THEN the operation MUST fail closed
- AND the prior target bytes and lock state MUST be restored or remain
  internally consistent
- AND no partial temporary file or unverified asset MUST become executable

### Requirement: Current Gate Asset and Release Freshness

The version-keyed Go gate cache MUST not treat an executable file as current
solely because it exists. For the current platform and CLI version, acceptance
MUST be based on the existing committed `SHA256SUMS` trust root plus the
current binary version and self-test checks. A missing, stale, mismatched, or
unknown cached asset MUST trigger forced re-acquisition during ordinary
acquisition/materialization or the explicit gate-refresh path. It MUST not be
executed as a verified current gate before those checks pass. If verification
or replacement fails, the operation MUST fail closed and the stale/unknown
candidate MUST remain unselected. The diagnostic MUST name the attempted
replacement or failure and its recovery evidence.

The normal launcher invocation MUST retain the existing no-digest hot-path
contract except for the bounded pre-exec rejection required to avoid executing
an unverified cache candidate. Release build flags, exact toolchain pin, asset
names, tag/version stamp, canonical digest comparison, and `ai-specs doctor`
evidence MUST remain consistent with the committed trust root. The legacy Bash
fallback remains a distinct governed asset and MUST not be used to bless an
unverified Go cache file. A successful cache acquisition MUST leave an atomic
`<binary>.verified` receipt containing the accepted version, digest, and passing
self-test; the launcher MUST reject a cache executable without a current
receipt. Sync and doctor MUST revalidate the trust-root digest before running
version or self-test commands, so stale bytes are never executed merely for
diagnostics.

#### Scenario: Stale cache binary is not accepted as current

- GIVEN the version-keyed cache contains an executable gate binary
- AND its observed digest, reported version, or self-test does not match the
  current accepted asset state
- WHEN acquisition or materialization evaluates the cache
- THEN it MUST force re-acquisition of the latest canonical asset
- AND it MUST report the cache path, expected/observed digest, version, and
  self-test evidence
- AND it MUST not execute the stale/unknown bytes before the replacement is
  verified
- AND if re-acquisition or replacement fails, it MUST fail closed and leave the
  candidate unselected

#### Scenario: Committed release digest remains authoritative

- GIVEN a release matrix artifact differs from the committed
  `catalog/recipes/worktree-flow/bin/SHA256SUMS` entry
- WHEN the release checksum gate runs
- THEN the release MUST fail with the existing regeneration guidance
- AND no mismatched artifact MUST become an accepted cache asset

#### Scenario: Doctor exposes actionable freshness evidence

- GIVEN a worktree-flow cleanup, launcher, legacy gate, or cached Go asset is
  stale, unknown, or digest-invalid
- WHEN `ai-specs doctor` runs
- THEN it MUST report an ERROR naming the asset state and evidence
- AND it MUST state that ordinary sync will force the latest verified
  replacement, plus the explicit retry/re-acquisition action where applicable
- AND doctor MUST not mutate the project or lock
- AND the diagnostic MUST NOT turn the ordinary sync replacement into a
  preserve-and-defer requirement

### Scenario: Canonical preflight precedes project writes

- GIVEN worktree-flow is enabled in a project manifest
- AND the catalog cleanup template, launcher, legacy gate, and supported gate
  trust-root inputs are available
- WHEN ordinary `ai-specs sync` starts
- THEN a read-only worktree-flow freshness preflight MUST verify those canonical
  inputs before the first consumer-project write
- AND materialization MUST repeat classification and verification immediately
  before each governed replacement
- AND the preflight MUST NOT create or rewrite the project's materialized assets
  or lock

#### Scenario: Version and lock drift is distinguishable

- GIVEN the repository `VERSION`, stamped gate version, cache key, and
  `.ai-specs.lock [meta].cli_version` do not describe the same sync state
- WHEN the freshness checks run
- THEN the result MUST identify which version relationship is stale or unknown
- AND it MUST not silently rewrite the lock as part of reporting
