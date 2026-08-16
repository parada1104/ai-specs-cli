# Design: multi-commit cleanup with forced latest worktree-flow freshness

- **Change slug**: `card-46-asset-freshness`
- **Depth**: Full
- **Reference cleanup implementation**: `catalog/recipes/worktree-flow/templates/worktree-cleanup.sh`
- **Reference gate implementation**: `catalog/recipes/worktree-flow/gate/`
- **Canonical specification**: `openspec/specs/worktree-flow/spec.md`

## Design Summary

This change has two deliberately separate seams:

1. **Cleanup seam**: keep the Bash template authoritative and verify complete
   merge evidence for multi-commit branches using the functions already in the
   script.
2. **Freshness seam**: make lifecycle/materialization decisions classify
   worktree-flow assets for evidence, then force them to the latest verified
   canonical bytes during ordinary sync/materialization. Existing custom bytes
   are backed up where supported, but are no longer a reason to defer the
   canonical update.

The Go migration is relevant only to the second seam. The Go binary remains the
implementation of record for `worktree-gate`; it does not own worktree cleanup.

## Source-Derived Cleanup Flow

The existing cleanup template has the following decision order:

1. Parse `--dir`, `--base`, `--dry-run`, topology, and optional submodule
   scopes.
2. Resolve the superproject root and the shared worktree prefix.
3. Enumerate either the superproject or every initialized submodule according
   to the resolved topology.
4. Parse each repository's `git worktree list --porcelain` record.
5. Ignore paths outside the configured worktree prefix.
6. Preserve detached records before merge detection.
7. Preserve dirty records before merge detection.
8. Call `is_merged` with the worktree branch tip and selected base.
9. In dry-run mode report `would remove`; otherwise remove the worktree and
   delete the branch only after the merge proof succeeds.

`is_merged` currently resolves local base candidates in this order:

1. Exact `--base` ref.
2. Configured upstream ref.
3. Configured remote-tracking ref.
4. `origin/<base>` only when the configured remote ref did not resolve.

It then checks ancestry for every candidate before checking patch-id
equivalence. `candidate_has_patch_equivalence` first establishes that the
branch has commits not already reachable from the candidate, then requires
`git cherry` to contain no `+` lines. That is the source-derived definition of
complete squash/rebase equivalence for this change.

### Cleanup Decision Table

| Fixture state | Expected result | Evidence to retain |
|---|---|---|
| Clean branch with multiple commits merged regularly | Remove in normal mode; report `would remove` in dry-run | Ancestry proof against an ordered local base candidate |
| Clean branch with multiple commits squash-merged | Remove/report `would remove` | Complete patch-id equivalence for all branch commits |
| Clean branch with one of multiple commits represented in base | Preserve as `unmerged` | At least one `+` line from the branch comparison |
| Branch changes later reverted from base | Preserve as `unmerged` | No complete patch-id/ancestry proof remains |
| Clean branch not merged | Preserve as `unmerged` | No candidate proves merge |
| Dirty otherwise-merged worktree | Preserve as `dirty` before merge checks | `git status --porcelain` output is non-empty |
| Main worktree | Never inspect for removal | Path is outside the configured worktree prefix |
| Detached worktree under the configured directory | Preserve as `detached` | No `refs/heads/` branch record |
| Uninitialized or unproven submodule topology | Preserve/skip | `git submodule status` and topology resolution do not prove ownership |
| Worktree outside an explicit submodule scope | Preserve/ignore | Scope filter excludes the module |

The implementation must first run these fixtures against the current function
set. If multi-commit squash already passes, the production diff should not
change the algorithm. If it fails, the fix must be limited to the failing
source-derived decision point and must leave all negative cases green.

## Freshness Boundary

### Governed assets

The freshness scope is limited to worktree-flow assets that affect cleanup or
gate enforcement:

| Asset | Current path | Current ownership evidence |
|---|---|---|
| Cleanup template output | `ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh` | `[managed."...cleanup.sh"]` hash, recipe, source, kind `template`, policy currently `auto` |
| Go launcher | `ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh` | `[managed."...worktree-gate.sh"]` baseline, kind `gate`, policy `auto` |
| Frozen Bash fallback copy | `ai-specs/recipes/worktree-flow/hooks/worktree-gate-legacy.sh` | Currently copied by `materialize_legacy_gate` without the same lock-backed baseline; this is a freshness gap to close |
| Go cache binary | `$AI_SPECS_HOME/cache/bin/worktree-gate/<cli-version>/<goos>-<goarch>/worktree-gate` | Current version-keyed path, committed `SHA256SUMS`, acquisition mismatch record, binary `--version`, and `--selftest` |

Other recipe templates and bundled skills are outside this card. Existing
generic `auto`, `confirm`, and `never-force` behavior must not be changed as a
side effect.

### State machine

The materializer and doctor must use the existing
`util.classify_managed_override` states, plus the existing digest/version
helpers for the binary cache:

| State | Ordinary sync/materialization | Explicit refresh | Evidence |
|---|---|---|---|
| Missing target | Materialize/acquire the latest canonical bytes, verify them, and record accepted provenance | Not needed | Target and new managed digest/version |
| Managed current | Verify the current desired bytes/state and leave the target unchanged | Not needed | Recorded and current digest/version |
| Managed stale | Back up where supported, atomically replace with verified canonical bytes, update provenance, and report the replacement | The same transaction may be requested explicitly | Target, prior/desired digest, recipe/source, backup |
| User-modified | Back up where supported, atomically replace with verified canonical bytes, update provenance only after success, and report the replacement | The same transaction may be requested explicitly | Target, observed/desired digest, replacement, backup |
| Unknown/untracked | Back up where supported, atomically replace with verified canonical bytes, then seed provenance from the installed bytes; never accept the unknown bytes as current | The same transaction may be requested explicitly | Target, observed digest, replacement, new provenance, backup |
| Cached binary missing or unsupported | Acquire the latest verified Go asset when the platform is supported; otherwise return the existing explicit unsupported/fallback diagnostic without claiming a current Go asset | The same acquisition path may be requested explicitly | Platform, version-keyed path, supported matrix, verification result |
| Cached binary executable but stale/unknown/mismatched | Revalidate digest, `--version`, and `--selftest`; force re-acquisition/rebuild when any check fails. Do not execute the prior candidate unless it is reverified as current | Explicit `--refresh-gates` may force the same path | Cache path, expected/observed digest, binary version, self-test result, replacement/failure |

The classifier is not a preservation gate for this user-selected worktree-flow
scope: stale, user-modified, and unknown states must proceed to forced
replacement. A freshness operation is hard only when canonical verification,
backup, atomic replacement, rollback, or lock update fails. In that case sync or
materialization returns a failure, the prior target/lock state is restored or
left internally consistent, and no unverified asset is accepted or executed.
Doctor remains read-only: it reports the same evidence and points to ordinary
sync, but it does not repair the project. Generic `auto`, `confirm`, and
`never-force` semantics for unrelated recipes remain unchanged.

### Enforcement points

The current sync pipeline performs mutating steps before invoking the full
recipe materializer. The implementation plan therefore requires two checks:

1. A read-only worktree-flow freshness preflight after target and CLI-version
   resolution but before ordinary sync writes. It must verify/obtain the latest
   canonical bytes and trust-root evidence; a stale or customized target alone
   is not a reason to stop the update.
2. A second state and verification check inside materialization immediately
   before each governed target replacement, so direct materializer calls and a
   state change between preflight and write remain safe.
3. If either verification or replacement fails, the operation must fail closed
   before any unverified asset is accepted. Governed replacements must restore
   the prior target and lock state when the existing rollback mechanism applies.

Doctor must call the same classification and verification logic in read-only
mode and emit actionable evidence when a governed asset is not current or
verified. It must not mutate the project, seed a missing baseline, or silently
repair a target; ordinary sync is the repair path.

The cleanup script itself must remain standalone and must not fetch catalog
files, call the CLI, or discover a remote source at cleanup runtime. Its
freshness is established by the lifecycle/materialization boundary that creates
the governed target. Adding a network or catalog lookup to Bash cleanup would
be a new algorithm and is out of scope.

## Explicit Refresh Paths

Use the current command surfaces without conflating their responsibilities:

- **Cleanup override**: ordinary `ai-specs sync` must render the current
  catalog template, verify it, back up the prior target where the existing
  cache mechanism supports it, and atomically replace stale, unknown, or
  user-modified bytes. The resulting lock entry records the installed bytes;
  no remove-then-sync action is required for this governed target.
- **Generated gate launcher/fallback**: ordinary sync and
  `ai-specs sync --refresh-gates` must use the same forced replacement
  transaction. It saves exact pre-refresh bytes in the CLI cache, writes the
  launcher or legacy fallback atomically, verifies the result, and updates the
  baseline only after success. The flag remains a useful explicit retry, not
  the only route to replacement.
- **Cached Go asset**: use the existing versioned acquisition and committed
  digest trust root. A stale, unknown, mismatched, or failed self-test cache
  candidate triggers forced re-acquisition; the replacement must be
  digest-verified, version-verified, self-tested, and installed atomically.
  The old candidate is never executed as a substitute for verification, and
  the implementation must not add a new unverified download path.

The final diagnostic must name the exact asset state, observed/expected
evidence, whether replacement occurred, the backup/recovery location when one
exists, and the ordinary sync or explicit retry command. It must not suggest a
generic `rm -rf` that silently discards custom bytes.

## Version, Lock, and Digest Rules

- `VERSION` remains the release/version source used by `build-gate.sh` and the
  version-keyed gate cache.
- The materialized launcher continues to carry
  `stamped_gate_version`, and doctor compares the binary's `--version` with
  that stamp.
- `.ai-specs.lock [meta].cli_version` remains the existing last-sync evidence.
  The plan must report a mismatch such as the inspected `0.21.0` lock versus
  `0.22.0` `VERSION`; a successful canonical refresh may update the existing
  sync metadata, but a failed freshness check must not silently restamp or
  partially rewrite the lock.
- `[managed.*]` remains the project-level provenance record for governed
  materialized bytes. New fields are not justified unless an implementation
  test proves that the existing `sha256`, `recipe`, `source`, `kind`, and
  `policy` fields cannot express the required state.
- `SHA256SUMS` remains the release trust root. Downloaded or rebuilt Go assets
  must be checked before install/acceptance. Cache acquisition, materialization,
  and doctor must revalidate digest, version, and self-test evidence; the
  launcher must reject a cache candidate without current verified evidence
  before `exec`. Keep the existing `WORKTREE_GATE_VERIFY=1` diagnostic behavior
  and do not change the Go gate decision policy merely to implement freshness.
- `scripts/verify-gate-sums.sh` and the release workflow remain the only
  release digest comparison surfaces. The plan must preserve exact asset names,
  canonical toolchain pin, build flags, and tag/version alignment.

## Go Gate Boundary

The current Go module already owns path/event parsing, topology classification,
decision, messages, and parity tests. Card #46 may touch its distribution
freshness only where required to force the latest verified canonical asset and
reject an unverified or stale cached asset.
It must not:

- add cleanup or merge detection to the Go module;
- change protected-branch policy, URI policy, shell heuristics, or fail-open
  messages;
- accept a cache candidate solely because it is executable or because its
  self-test passes without current digest/version evidence;
- add digest work inside the Go decision core on every tool invocation unless
  the launcher verification seam requires a bounded pre-exec check;
- remove the frozen Bash reference or change the stable materialized launcher
  path.

## Test Design

### Cleanup tests

Extend `tests/test_worktree_cleanup.py` with real temporary Git repositories,
not synthetic paths:

- multi-commit regular merge;
- multi-commit squash merge where ancestry is false but every branch commit is
  represented by the squash result;
- partial squash where at least one branch commit is absent;
- a squash result later reverted from base;
- clean fast-forward/rebase positives;
- dirty, main, detached, unmerged, and branch-ahead preservation;
- configured remote safety, no-fetch behavior, initialized/uninitialized
  submodule enumeration, explicit topology, and scope protection.

Assertions must cover worktree existence, branch existence, exact stable output,
and that no removal occurs on any negative case. The multi-commit cases must
exercise the real `is_merged` helpers, not a parallel test-only algorithm.

### Freshness and distribution tests

Use `TemporaryDirectory()`/`t.TempDir()` style fixtures and existing test
helpers:

- `tests/test_worktree_flow_recipe.py` for recipe policy, materialization, and
  direct materializer hard failures;
- `tests/test_override_ownership.py` for lock round trips, state matrices,
  forced replacement, immutable backup/rollback, and generic-policy regression;
- `tests/test_sync_pipeline.py` for ordinary forced refresh, the CLI refresh
  flag, and preflight ordering;
- `tests/test_gate_binary_dist.py` for cache digest/version/self-test acceptance,
  mismatch rejection/re-acquisition, rollback, and no unverified execution;
- `tests/test_worktree_gate_dist_config.py` for launcher/fallback/refresh
  behavior;
- `tests/test_doctor_worktree_gate.py` for actionable ERROR evidence;
- `tests/test_worktree_gate_release_phase4.py` for release toolchain/version and
  canonical digest checks;
- `tests/test_lock.py` and `tests/test_cli_version.py` for schema compatibility
  and the existing version/lock policy.

Go unit tests remain under the existing `catalog/recipes/worktree-flow/gate`
module only when the implementation changes there. Existing parity and
launcher tests must remain green; a cleanup change must not be hidden behind a
Go binary skip.

## Decision Points Before Apply

The force-latest policy for governed worktree-flow assets is user-selected and
authoritative; it is not a decision to reopen during apply. These are the
narrow source and RED-fixture decisions that remain:

1. Does the current `git cherry` loop already recognize the exact multi-commit
   squash fixture? If yes, keep the algorithm and land regression coverage; if
   no, correct only the failing complete-equivalence step.
2. Which exact commit graph represents a partial squash and a reverted squash
   without accidentally creating a regular merge or a new equivalent patch?
3. Which freshness preflight seam can run before the current sync writes while
   preserving direct materializer callers and fan-out behavior?
4. How should the currently untracked legacy gate copy share the existing lock
   provenance and immutable refresh backup without treating it as a new hook
   renderer target?
5. Which existing acquisition/refresh seam handles an executable but stale
   cache binary, and how is the old byte retained or quarantined without ever
   executing it when verification or replacement fails?
6. Does the existing CLI version check remain informational for generic lock
   staleness while worktree-flow verification binds `VERSION`, the stamped
   launcher, cache key, and `.ai-specs.lock` narrowly? The answer must be
   demonstrated by tests and must not broaden global `[tool]` policy.
7. Which existing backup/rollback helper can cover the cleanup override and
   legacy gate without creating a generic recipe ownership policy, and what
   exact failure evidence proves the target and lock were restored?

## Artifact Path

`openspec/changes/card-46-asset-freshness/design.md`
