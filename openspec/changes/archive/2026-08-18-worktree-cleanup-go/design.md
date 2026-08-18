# Design: Go worktree cleanup and post-merge remote deletion

## 1. Architecture

The cleanup command is a subcommand of the existing zero-dependency Go module:

```text
worktree-cleanup.sh (stable materialized path)
        │ resolve verified executable
        ▼
worktree-gate --cleanup ...
        │
        ├── parse compatible flags/config
        ├── enumerate repository/module passes as []repoPass
        ├── enumerate worktrees as []worktreeRecord
        ├── classify dirty/detached/unmerged
        ├── immediately re-check protected name before each destructive call
        ├── remove worktree + local branch
        ├── delete remote branch from main worktree
        └── verify git ls-remote --heads is empty
```

The existing gate mode remains the default when no `--cleanup` flag is given.
The cleanup launcher must resolve a verified current binary using the same
version-keyed cache and receipt checks as the gate distribution. It may accept
`WORKTREE_CLEANUP_BIN` as an explicit diagnostic pin, but it must reject a
missing/unverified binary with a non-zero exit and must never execute the old
Bash implementation as a destructive fallback.

## 2. Command and compatibility contract

The launcher accepts the existing cleanup options:

```text
worktree-cleanup.sh [--dir <worktrees_dir>] [--base <integration_branch>]
                    [--dry-run] [--topology <value>]
                    [--submodule <path>|--subrepo <path>]...
```

The Go command uses:

```text
worktree-gate --cleanup [same options]
```

Configuration is resolved as follows:

1. `--base`, when supplied, is authoritative for the pass.
2. Otherwise use the configured integration/base branch value.
3. The protected-name set always contains `main`, `master`, `development`,
   `staging`, the configured base branch, and configured integration branch;
   duplicates are removed.
4. Remote selection is derived from the branch's configured upstream when
   present; otherwise use the configured remote for the branch or `origin` only
   when a local origin ref/configuration proves it exists. No network fetch is
   performed for classification.

The launcher computes its physical recipe root from its own path, not `$PWD`,
and resolves the same version-keyed cache path as `worktree-gate.sh`. It checks
`<binary>.verified` and invokes `--selftest` only when the existing distribution
policy requests verification; the normal hot path does not hash bytes. Cleanup
fails closed if the cache candidate is absent, lacks the current receipt, or is
otherwise unusable.

## 3. Data model and pass iteration

```go
type cleanupConfig struct {
    worktreesDir       string
    baseBranch         string
    integrationBranch  string
    protected          map[string]struct{}
    dryRun             bool
    topology           string
    scopes             []string
}

type repoPass struct {
    root       string
    modulePath string
}

type worktreeRecord struct {
    path   string
    sha    string
    branch string
}
```

`enumerateModules` returns a slice, not a newline-delimited scalar. The caller
uses `for _, pass := range passes`, and each pass uses `for _, record := range
records`. Scope filtering is applied to the slice before scanning. This makes
batch iteration explicit and prevents shell word-splitting behavior from being
reintroduced.

The main worktree is naturally excluded because only paths under the configured
worktree directory are candidates. Detached records are reported and preserved.
Dirty status is checked before merge proof, as in the reference implementation.

## 4. Merge proof port

The following functions retain the Bash proof's ordered behavior and failure
semantics:

- `resolveBaseCandidates(base) []string`: exact base, configured upstream,
  configured remote-tracking ref, conditional origin fallback. Candidate refs are
  emitted only after `git rev-parse --verify --quiet`; duplicates are removed by
  exact spelling. No fetch.
- `candidateHasMergedTip(sha, candidate)`: `git merge-base --is-ancestor`.
- `candidateHasPatchEquivalence(sha, candidate)`: `git rev-list` and `git cherry`,
  rejecting any `+` line.
- `candidateHasCombinedPatchEquivalence(sha, candidate)`: use the common base,
  compute stable patch ids for the complete branch and candidate deltas, and
  require equality.
- `candidateHasCombinedTreeEquivalence(sha, candidate)`: use
  `git diff --name-only -z --no-renames` and read each path's `git ls-tree`
  entry for both tips. NUL-delimited scanning is mandatory; no line splitting or
  shell interpolation may touch these paths. Require at least one changed path.
- `isMerged`: ancestry across all ordered candidates first, then patch-id,
  combined patch-id, and combined-tree checks across all candidates.

No historical candidate commit is inspected for squash proof. Consequently a
squash that was later reverted remains unmerged.

## 5. Immediate destructive safety checks

All destructive operations route through wrappers. Each wrapper invokes
`assertDeleteAllowed(kind, branch, worktreePath, cfg)` as its first operation,
not merely relying on an earlier classification:

```text
removeWorktree:
  assert protected name → refuse loudly
  assert no worktree holds protected/target branch unexpectedly
  git worktree remove <path>

removeLocalBranch:
  assert protected name → refuse loudly
  assert no worktree list entry holds branch
  git branch -d <branch> || git branch -D <branch>

removeRemoteBranch:
  assert protected name → refuse loudly
  assert no worktree list entry holds branch
  git push <remote> --delete <branch>
  git ls-remote --heads <remote> <branch>
  require zero matching refs, else fail loudly
```

A protected-name refusal is an error with stable text containing
`refusing destructive cleanup of protected branch <name>` and the operation.
The code must not silently skip it or convert it to an ordinary `skipped`
status. The check is repeated before each wrapper call even when the same
branch was already checked during classification. If a branch is still held by
any worktree, local and remote deletion are refused; this protects against
state changes between enumeration and deletion.

Remote deletion is attempted only in normal (non-dry-run) cleanup after the
local worktree/branch removal has succeeded. A remote branch is never deleted
for an unmerged, dirty, detached, protected, or still-held branch. Dry-run
reports the eligible worktree and remote action without mutating or contacting
the remote.

## 6. Topology behavior

`enumerateModules` preserves the existing topology contract:

- `standalone` and `monorepo-apps`: one pass for the super/root repository.
- `auto`: initialized submodules produce one pass per initialized module;
  otherwise one root pass.
- `monorepo-submodules`: one pass per initialized module; uninitialized entries
  are skipped.
- repeated `--submodule`/`--subrepo` values filter to matching module paths.

Each pass recomputes its base branch from the explicit `--base` or the current
branch, matching the existing script. The shared worktree prefix remains based
on the superproject root and configured directory, so submodule worktrees retain
names such as `apps/api-feat-x`.

## 7. Distribution and release

No second binary is introduced. `scripts/build-gate.sh` continues to build the
four assets and the existing `SHA256SUMS` entries remain the trust root. The Go
`--selftest` must cover cleanup command registration and its required Git
capabilities in addition to existing gate regex/git checks. The cleanup launcher
uses the version stamp and cache hierarchy already materialized for the gate.

The cleanup launcher does not invoke Python acquisition logic and does not
compile on demand. Sync continues to acquire the single binary through the
existing `gate_binary.py`; its verification receipt is the only authority for
normal cleanup execution. If sync cannot acquire it, cleanup reports:

```text
worktree-cleanup: no verified Go implementation available; no destructive action taken
```

The old template algorithm is removed from the catalog source rather than kept
as a second implementation. The materialized override remains governed by the
existing template ownership rules and is updated only from the catalog source.

## 8. Testing strategy

### Unit tests (Go)

Table-driven tests cover:

- protected set construction and duplicate removal;
- refusal immediately before worktree, local branch, and remote deletion;
- held-branch detection;
- base-candidate order and no-fetch behavior;
- ancestry, cherry, combined patch, combined tree, newline paths, and reverted
  squash proof;
- topology pass enumeration and explicit scope filtering;
- remote verification success, remote ref still present, and command failure;
- cleanup flag parsing and dry-run no-mutation behavior.

### Integration tests (existing Python harness)

`tests/test_worktree_cleanup.py` continues to create temporary Git repositories
and linked worktrees, but invokes the Go implementation through a test-built
binary. It preserves the existing proof cases and adds:

- protected names at each destructive entry point;
- configured base/integration branch names in the protected set;
- unmerged and dirty refusal;
- a batch with two eligible branches proving both are visited and removed;
- remote bare repository setup, deletion, and `git ls-remote` absence assertion;
- remote verification failure preserving a loud non-success;
- branch-held-by-worktree refusal;
- launcher missing/unverified binary fail-closed behavior.

Strict TDD evidence records each RED failure before the corresponding Go
implementation and each GREEN result after it. Every test assertion is checked
by temporarily reverting the relevant fix and restoring it afterward.

### Broad validation

Run `go test ./...`, focused cleanup tests, and `./tests/validate.sh`. No
coverage, linter, type checker, or configured formatter is claimed beyond the
commands actually available; `gofmt` and `go vet` are run when the Go toolchain
is present.

## 9. Operational flow

After a PR merge, the coordinator runs the cleanup launcher from the main
worktree root, not from the removed feature worktree:

```text
cd <main-repository-root>
bash ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh \
  --dir <worktrees_dir> --base <integration_branch>
```

The output distinguishes removed, dry-run eligible, dirty, unmerged, detached,
and explicit safety refusals. A successful remote deletion is not claimed until
`git ls-remote --heads` proves the remote ref is absent.
