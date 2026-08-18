# Post-merge cleanup sequence

## Requirement: ordered cleanup and final base synchronization

The native cleanup command MUST own post-merge cleanup from the primary worktree. The merge workflow MUST merge without `--delete-branch`. Cleanup MUST remove a proven linked worktree, delete its local branch, delete and verify its remote branch, and ONLY THEN synchronize the configured base with `git pull --ff-only`. It MUST NOT pull while a candidate worktree still holds a branch.

### Scenario: complete feature cleanup precedes base sync

- GIVEN a merged feature worktree and local/remote branch
- WHEN cleanup runs normally from the main worktree
- THEN worktree removal precedes local branch deletion
- AND local deletion precedes remote deletion and verification
- AND base synchronization is the final Git mutation

### Scenario: dry run has no mutations

- GIVEN an eligible merged candidate
- WHEN cleanup runs with `--dry-run`
- THEN it reports exact planned paths
- AND performs no deletion, remote operation, or pull

## Requirement: stale local branch classification

Cleanup MUST inspect local branches not held by any worktree. A stale branch MAY be deleted only when its PR merge commit is proven an ancestor of the selected base, its cumulative diff is patch-identical to the landed squash, or it has no PR and every Git path it touched exists in the selected base tree. Open, missing, failed, or ambiguous PR evidence MUST refuse cleanup. A branch with an absent touched path MUST be preserved as unmerged/lost-work evidence.

### Scenario: local branch with removed worktree is cleaned

- GIVEN a local branch has no linked worktree
- AND its merge proof is positive
- WHEN cleanup runs
- THEN the branch is considered and removed safely

### Scenario: branch with absent touched path is preserved

- GIVEN a local branch has no PR and touches a path absent from the base tree
- WHEN cleanup runs
- THEN the branch remains and cleanup reports an unmerged/refusal result

### Scenario: ambiguous evidence refuses

- GIVEN PR lookup or branch path evidence cannot be completed
- WHEN cleanup classifies the branch
- THEN it does not delete the branch or remote

## Requirement: preserved safety proof

The existing ordered base-ref resolution and `isMergedCleanup` three-term OR chain MUST remain semantically unchanged. Protected branches MUST be checked immediately before every destructive call, and any worktree-held branch MUST be refused for local or remote deletion. Collections MUST be iterated structurally and tests MUST assert exact paths/branch names.
