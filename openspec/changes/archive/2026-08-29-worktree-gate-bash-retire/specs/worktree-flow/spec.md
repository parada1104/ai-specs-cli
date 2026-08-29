# Delta for worktree-flow

Retires the legacy Bash worktree gate after the Go gate became the default
(v0.22.0 shipped `gate_impl = "auto"` with no field regression). The frozen
Bash reference leaves the catalog, the `bash` value leaves the `gate_impl`
enum, and the launcher's legacy fallback step is removed; the launcher fails
open with exactly one stderr warning when no gate binary resolves.

## MODIFIED Requirements

### Requirement: Forced Latest-Canonical Refresh for Governed Worktree-Flow Assets

The worktree-flow cleanup override and generated Go launcher MUST be classified
using the existing lock-backed provenance and current would-write bytes before
replacement or execution. A managed-current asset MAY be used without rewriting
after its current bytes remain verified. A missing asset MAY be materialized and
recorded. A managed-stale, user-modified, or unknown/untracked governed asset
MUST be force-replaced by the latest verified canonical bytes during ordinary
sync/materialization.

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

(Previously: this requirement classified and force-refreshed the materialized
legacy Bash gate as a third governed asset alongside the launcher and cleanup
override; the legacy gate is retired and is no longer classified, materialized,
or refreshed.)

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

#### Scenario: Customized launcher is force-replaced by ordinary sync

- GIVEN a materialized `worktree-gate.sh` launcher differs from its recorded
  baseline or has no baseline
- WHEN ordinary sync or `ai-specs sync --refresh-gates` runs
- THEN the pre-refresh bytes MUST be saved through the existing cache-only
  immutable backup mechanism where that mechanism applies
- AND the launcher MUST be atomically replaced with verified canonical bytes
- AND its baseline/lock evidence MUST be updated only after replacement succeeds
- AND the operation MUST report the replacement rather than block on the local
  customization

(Previously: this scenario covered "a materialized `worktree-gate.sh` or legacy
gate"; the legacy gate is no longer a governed asset.)

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

#### Scenario: Canonical preflight precedes project writes

- GIVEN worktree-flow is enabled in a project manifest
- AND the catalog cleanup template, launcher, and supported gate trust-root
  inputs are available
- WHEN ordinary `ai-specs sync` starts
- THEN a read-only worktree-flow freshness preflight MUST verify those canonical
  inputs before the first consumer-project write
- AND materialization MUST repeat classification and verification immediately
  before each governed replacement
- AND the preflight MUST NOT create or rewrite the project's materialized assets
  or lock

(Previously: the preflight inputs included the legacy gate; that input no
longer exists.)

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
evidence MUST remain consistent with the committed trust root. A successful
cache acquisition MUST leave an atomic `<binary>.verified` receipt containing
the accepted version, digest, and passing self-test; the launcher MUST reject a
cache executable without a current receipt. Sync and doctor MUST revalidate the
trust-root digest before running version or self-test commands, so stale bytes
are never executed merely for diagnostics.

(Previously: the launcher retained the legacy Bash fallback as a distinct
governed asset that MUST not be used to bless an unverified Go cache file; the
Bash fallback is retired and the Go binary is the only gate implementation.)

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

- GIVEN a worktree-flow cleanup, launcher, or cached Go asset is stale, unknown,
  or digest-invalid
- WHEN `ai-specs doctor` runs
- THEN it MUST report an ERROR naming the asset state and evidence
- AND it MUST state that ordinary sync will force the latest verified
  replacement, plus the explicit retry/re-acquisition action where applicable
- AND doctor MUST not mutate the project or lock
- AND the diagnostic MUST NOT turn the ordinary sync replacement into a
  preserve-and-defer requirement

(Previously: the freshness evidence targets included the legacy gate; that
target no longer exists.)

#### Scenario: Version and lock drift is distinguishable

- GIVEN the repository `VERSION`, stamped gate version, cache key, and
  `.ai-specs.lock [meta].cli_version` do not describe the same sync state
- WHEN the freshness checks run
- THEN the result MUST identify which version relationship is stale or unknown
- AND it MUST not silently rewrite the lock as part of reporting

## ADDED Requirements

### Requirement: Single Gate Implementation Without Bash Rollback

The worktree-flow gate MUST have exactly one implementation: the Go gate
binary. The `gate_impl` setting MUST accept only `auto` and `go`, and
configuration validation MUST reject `gate_impl = "bash"` with an actionable
error naming the removed value and the valid values. Doctor MUST NOT present an
explicit Bash configuration as a rollback lever and MUST NOT emit the legacy
rollback-lever diagnostic. Gate binary resolution MUST NOT branch on a Bash
implementation choice. The catalog MUST NOT ship a frozen legacy Bash gate
reference, and ordinary sync MUST NOT materialize or refresh any legacy Bash
gate; the lock-backed provenance machinery remains in force for the launcher
and cleanup assets only.

#### Scenario: Explicit bash configuration is rejected

- GIVEN a project manifest or recipe config stamps `gate_impl = "bash"`
- WHEN configuration validation runs during sync or doctor
- THEN validation MUST reject the value with an actionable error naming `bash`
  as removed and `auto | go` as the only valid values
- AND no Bash gate MUST be materialized or invoked

#### Scenario: Doctor no longer offers the Bash rollback lever

- GIVEN a project whose lock still carries an explicit `gate_impl = "bash"`
  stamp from an earlier CLI version
- WHEN `ai-specs doctor` runs
- THEN doctor MUST NOT report the Bash configuration as a rollback lever
- AND it MUST report the stamped value as retired and name the recovery action
  of re-running sync so the project is stamped with a supported value
- AND doctor MUST remain read-only

#### Scenario: Legacy gate is never materialized again

- GIVEN a project with worktree-flow enabled runs ordinary sync
- WHEN materialization runs
- THEN no legacy Bash gate file MUST be written into the project's materialized
  hooks
- AND the catalog MUST NOT provide a legacy Bash gate reference to materialize

#### Scenario: Docs and catalog no longer document a Bash rollback path

- GIVEN the runtime-hooks documentation, the recipe README, and the recipes
  catalog
- WHEN a reader looks up `gate_impl` or the launcher resolution chain
- THEN only `auto | go` MUST be documented as valid values
- AND no documentation or catalog entry MUST present the legacy Bash gate as a
  fallback or rollback path

### Requirement: Launcher Fail-Open Without Legacy Fallback

The launcher resolution order MUST be: the `WORKTREE_GATE_BIN` environment
override, then the project-local gate binary, then the version-keyed cache
binary. When no step resolves a verified gate binary, the launcher MUST fail
open: it MUST NOT block the original operation, and it MUST emit exactly one
stderr warning naming that the gate binary could not be resolved and how to
restore gate coverage (project-local materialization or the explicit
gate-refresh path). The launcher MUST NOT fall back to any Bash implementation.

#### Scenario: No binary resolves anywhere

- GIVEN `WORKTREE_GATE_BIN` is unset
- AND no project-local gate binary exists
- AND no verified version-keyed cache binary exists
- WHEN an operation invokes the launcher on a protected branch
- THEN the launcher MUST NOT block the original operation (fail open)
- AND it MUST print exactly one stderr warning naming the unresolved gate
  binary and the recovery action
- AND it MUST NOT attempt any Bash gate fallback

#### Scenario: Exactly one warning per invocation

- GIVEN all resolution steps fail for a single launcher invocation
- WHEN the launcher runs
- THEN stderr MUST contain exactly one gate-resolution warning
- AND the warning MUST NOT repeat once per failed resolution step

#### Scenario: Existing resolution order is preserved above the fail-open floor

- GIVEN `WORKTREE_GATE_BIN` points to an executable gate binary, or a
  project-local or verified cache binary resolves
- WHEN the launcher runs
- THEN the first resolved step in the order (environment override, project-local
  binary, version-keyed cache binary) MUST be used
- AND no fail-open warning MUST be emitted

### Requirement: Parity Corpus Asserts the Go Gate as the Only Implementation

The worktree-flow gate parity corpus MUST remain the executable specification
of gate behavior with the Go gate binary as the only implementation. The
Bash-reference half of the parity corpus (hermetic Bash fixtures, legacy-script
tokenizer passes, and release or workflow parity against the Bash gate) MUST be
removed or adjusted so every parity, tokenizer, hook, dist, and doctor suite
exercises and passes against the Go binary only. No suite MUST skip its Go
coverage because a Bash reference is missing, and no suite MUST compare gate
behavior against the retired legacy Bash script.

#### Scenario: Parity corpus passes with the Go binary only

- GIVEN the parity, tokenizer, hook, dist, and doctor test suites
- WHEN they run
- THEN every retained parity case MUST assert Go gate behavior
- AND no case MUST execute or compare against the retired legacy Bash gate
  script
- AND no suite MUST skip Go coverage because a Bash reference is unavailable

#### Scenario: Tokenizer behavior is pinned by the Go-only corpus

- GIVEN a shell command that mixes a move with output redirection (for example
  `mv a b 2>&1`)
- WHEN the parity corpus evaluates it against the Go gate
- THEN the corpus MUST pass
- AND the retired Bash tokenizer behavior MUST NOT be re-introduced as an
  expected result
