# Design: ordered post-merge cleanup and stale local branch safety

## Sequence

The cleanup command runs from the primary repository worktree and performs:

1. classify linked worktrees and stale local branches;
2. for every proven eligible linked candidate: remove worktree, delete local branch, delete and verify remote branch;
3. for every proven eligible stale local branch: delete local branch, then delete and verify its remote branch;
4. only after all candidate destructive work succeeds, run `git pull --ff-only <remote> <base>` when a configured base remote ref exists.

Dry-run performs classification and prints exact planned actions but performs no deletions, remote calls, or pull.

## Candidate model

Use structured slices:

```go
type localBranchRecord struct { branch, sha string }
type cleanupCandidate struct { branch, sha, path, remote string; worktree bool }
```

Enumerate `git for-each-ref` with NUL-delimited fields. Exclude protected names, the current base branch, and branches still held by any worktree from stale candidates. Linked worktree candidates retain their current merge proof and ordering.

## Evidence

For each stale branch:

- first apply existing local base candidate ancestry and patch proofs;
- if no positive merge proof exists, query PR metadata when `gh` is available and reject any branch with an open/ambiguous PR or an API failure;
- for a proven no-PR branch, collect all paths touched from its merge-base to its tip using NUL-delimited output; require every path to have a base tree entry; an empty/unknown path set is refusal.

The classifier never compares a branch's current diff against today's base as a merge decision. It never treats a missing binary artifact as lost source work: path checks concern only files represented in the branch diff and use Git tree entries.

## Safety wrappers

Every `git worktree remove`, `git branch -d/-D`, `git push --delete`, verification, and final pull is guarded by the current protected set and expected repository identity. Local/remote deletion re-reads worktree records immediately before mutation and refuses if any worktree still holds the branch. Failures are aggregated while later independent candidates are still reported; final base sync is skipped when destructive failures occurred.

## Documentation

The Git merge skill's merge command is always `gh pr merge --squash` without `--delete-branch`. Its post-merge section invokes the cleanup launcher from the main worktree and states that base synchronization is last, after worktree/local/remote deletion.

## Testing

Strict TDD adds RED tests for command ordering, final pull, stale branch visibility, no-PR path absence refusal, exact batch paths, and docs removing `--delete-branch`. Focused Go tests run before the full validation suite. A changed Go module requires `scripts/build-gate.sh` and four published SHA256SUMS digests, excluding `worktree-gate-current`.
