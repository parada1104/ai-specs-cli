# Exploration: own post-merge cleanup sequence

## Context

The Go cleanup implementation already owns merge proof and removal of merged linked worktrees, local branches, and remote branches. Card #88 extends that ownership to stale local branches without worktrees and to the final base-branch synchronization, while the Git merge skill must describe the same sequence.

## Findings

1. `catalog/recipes/worktree-flow/gate/cleanup.go` is the source of truth for destructive cleanup. Its ancestry, per-commit cherry, combined patch-id, and NUL-delimited tree-entry proof is load-bearing and must remain unchanged, including the three-term OR chain in `isMergedCleanup`.
2. Cleanup currently scans only `git worktree list --porcelain`; local branches left after their worktree was removed are therefore invisible.
3. The safe post-merge order is: merge without `--delete-branch`; remove the worktree; delete the local branch; delete the remote branch; then fast-forward-only sync the base. Pulling before worktree release makes Git report confusing branch-in-use failures.
4. A stale branch cannot be judged by today's base diff alone. A merge commit must be proven as an ancestor; squash proof must compare cumulative patch identity; no-PR fallback must inspect every touched path and refuse when a path is absent from the base. Missing or ambiguous evidence is preserved, never guessed.
5. Local collections must be represented as Go slices and iterated with explicit loops. Tests must assert exact branch/path outcomes, not directory counts.
6. Go source changes require rebuilding release assets and replacing only the four published SHA256SUMS entries; `worktree-gate-current` remains excluded.

## Selected approach

Extend the existing Go cleanup command with a second candidate source for local branches not held by any worktree, reuse the existing merge-proof functions, add a conservative no-PR touched-path existence proof, and run a final `git pull --ff-only` against the base remote after cleanup. Update the Git provider skill so it never recommends `gh pr merge --delete-branch` and puts base sync last.

## Scope boundary

In scope: Go cleanup sequencing, stale local branch classification/deletion, final base sync, source skill documentation, focused tests, and the required checksum regeneration. Out of scope: changes to provisioning-owned files, merge/push operations by this worker, or weakening the existing merge proof.
