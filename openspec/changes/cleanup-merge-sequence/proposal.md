# Proposal: make post-merge cleanup own the complete sequence

## Intent

Make the native Go cleanup command the authoritative post-merge sequence for linked worktrees, local and remote feature branches, and final base synchronization.

## Problem

The merge workflow recommends `gh pr merge --delete-branch`, but GitHub cannot delete the local branch while the base branch is checked out in the main worktree, and the remote branch remains alive. Cleanup also cannot see local branches whose worktrees were already removed. Finally, the base is not synchronized after cleanup, leaving the canonical checkout stale.

## Goals

- Merge without `--delete-branch` and document that it is unsupported for this worktree layout.
- Preserve the existing merge proof byte-for-byte in semantics, including its three-term `isMergedCleanup` OR chain.
- Remove eligible worktree, local branch, and remote branch in that order.
- Classify stale local branches conservatively using ancestor, cumulative patch identity, or the explicit no-PR touched-path existence proof; refuse ambiguous evidence.
- Sync the configured base branch with `git pull --ff-only` only after all cleanup deletions complete.
- Keep protected-name and worktree-held-branch checks immediately before every destructive Git call.
- Prove every batch member by exact branch/path assertions.

## Non-goals

- No changes to merge hosting settings or PR merge behavior beyond documentation.
- No cleanup of protected branches or unproven branches.
- No changes to materialized `ai-specs/` output or provisioning-owned files.
- No fetch or network operation during merge classification; only the final configured base sync may contact the remote.

## Affected paths

- `catalog/recipes/worktree-flow/gate/cleanup.go`
- `catalog/recipes/worktree-flow/gate/cleanup_test.go`
- `catalog/recipes/git-pr-flow/skills/git-merge-workflow/SKILL.md`
- `catalog/recipes/worktree-flow/bin/SHA256SUMS` (generated trust-root update when Go changes)

## Tracker

- **card_id**: `88`
- **url**: https://trello.com/c/BY26fvb3

## Success Criteria

- [ ] Cleanup removes eligible worktree, local branch, and remote branch in that order, then performs the final base fast-forward-only sync.
- [ ] `gh pr merge --delete-branch` is not recommended anywhere in the Git merge workflow; documentation describes the explicit cleanup sequence.
- [ ] Stale local branches without worktrees are classified by positive merge evidence or the explicit no-PR path-presence proof and ambiguous branches are preserved.
- [ ] Existing merge proof and protected-name/worktree-held safety behavior remain intact.
- [ ] Structural batch tests prove exact candidates are visited and no unrelated branch/path is removed.
- [ ] Go tests and `./tests/validate.sh` pass, with release checksums regenerated for any changed Go module.
