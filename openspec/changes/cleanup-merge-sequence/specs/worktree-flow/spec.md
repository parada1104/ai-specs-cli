# Post-merge cleanup sequence

## Requirement: ordered cleanup and final base synchronization

The native cleanup command MUST own post-merge cleanup from the primary worktree. The merge workflow MUST merge without `--delete-branch`. Cleanup MUST remove a proven linked worktree, delete and verify its remote branch, delete its local branch, and ONLY THEN synchronize the configured base with `git pull --ff-only`. It MUST NOT pull while a candidate worktree still holds a branch.

The local branch MUST outlive remote deletion. Remote deletion is the step that fails for reasons outside the machine, and the local branch is the only handle a later run has for rediscovering the candidate. Deleting it first would leave a surviving remote branch that no subsequent pass can find.

### Scenario: complete feature cleanup precedes base sync

- GIVEN a merged feature worktree and local/remote branch
- WHEN cleanup runs normally from the main worktree
- THEN worktree removal precedes remote deletion and verification
- AND remote deletion precedes local branch deletion
- AND base synchronization is the final Git mutation

### Scenario: failed remote deletion stays recoverable

- GIVEN a merged candidate whose remote is unreachable
- WHEN cleanup attempts remote deletion and fails
- THEN the local branch remains and the pass reports the failure
- AND a later run with a reachable remote completes both deletions

### Scenario: dry run has no mutations

- GIVEN an eligible merged candidate
- WHEN cleanup runs with `--dry-run`
- THEN it reports exact planned paths
- AND performs no deletion, remote operation, or pull

## Requirement: stale local branch classification

Cleanup MUST inspect local branches not held by any worktree. A stale branch MAY be deleted only on content evidence: a merged tip, patch equivalence with the landed squash, identical tree content for every path it touched, or a pull-request merge commit proven an ancestor of the selected base. Every pull request for that head MUST be examined, so a head closed unmerged once and later reused for a merged pull request is still classified correctly.

Path existence MUST NOT count as merge evidence. Two commits can touch the same path with entirely different content and never meet, so a matching name proves nothing about whether the work landed. Open, missing, failed, or ambiguous evidence MUST refuse cleanup and preserve the branch.

### Scenario: local branch with removed worktree is cleaned

- GIVEN a local branch has no linked worktree
- AND its merge proof is positive
- WHEN cleanup runs
- THEN the branch is considered and removed safely

### Scenario: same-named path is not merge evidence

- GIVEN a local branch has no pull request
- AND the base tree holds the same paths with different content
- WHEN cleanup runs
- THEN the branch remains and cleanup reports an unmerged/refusal result

### Scenario: ambiguous evidence refuses

- GIVEN PR lookup or branch path evidence cannot be completed
- WHEN cleanup classifies the branch
- THEN it does not delete the branch or remote

## Requirement: preserved safety proof

The existing ordered base-ref resolution and `isMergedCleanup` three-term OR chain MUST remain semantically unchanged. Protected branches MUST be checked immediately before every destructive call, and any worktree-held branch MUST be refused for local or remote deletion. Collections MUST be iterated structurally and tests MUST assert exact paths/branch names.
