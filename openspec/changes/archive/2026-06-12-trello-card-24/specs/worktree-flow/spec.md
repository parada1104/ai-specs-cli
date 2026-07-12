# Delta for worktree-flow

## ADDED Requirements

### Requirement: Positive Base Candidate Resolution for Merge Detection
The system MUST treat a worktree branch as merged when any local candidate ref proves ancestry or patch-id equivalence for the branch tip: the exact `--base` ref, the base branch's configured upstream ref, or the remote-tracking ref `origin/<base>` (or the configured remote/base when present). If no candidate proves merge, the system MUST fall back to existing `git cherry` patch-id equivalence.

#### Scenario: Regular merge on origin/base with stale local base
- GIVEN a temp repo with clean worktree `feat-regular`
- AND `origin/main` contains a merge commit that includes `feat-regular`
- AND local `main` still points before that merge
- WHEN `worktree-cleanup.sh --base main --dry-run` runs
- THEN it MUST report `would remove feat-regular`

#### Scenario: Squash merge still resolves by patch-id
- GIVEN a temp repo where `feat-squash` was squash-merged into `main`
- AND local `main` does not contain the branch tip by ancestry
- WHEN cleanup runs
- THEN it MUST report `would remove feat-squash`

#### Scenario: Rebase merge still resolves by patch-id
- GIVEN a temp repo where `feat-rebase` was rebased onto `main`
- AND the branch commits are already present by patch-id
- WHEN cleanup runs
- THEN it MUST report `would remove feat-rebase`

#### Scenario: Fast-forward merge remains merged
- GIVEN a temp repo where local `main` already contains the tip of `feat-ff`
- WHEN cleanup runs
- THEN it MUST report `would remove feat-ff`

#### Scenario: Local-only branch with no match stays unmerged
- GIVEN a temp repo where `feat-local` has no remote ref and no upstream ref
- AND its changes are not patch-equivalent to `main`
- WHEN cleanup runs
- THEN it MUST report `skipped feat-local (unmerged)`

#### Scenario: Branch ahead of base stays unmerged
- GIVEN a temp repo where `feat-ahead` has commits not present in `main`
- WHEN cleanup runs
- THEN it MUST report `skipped feat-ahead (unmerged)`

#### Scenario: Remote-deleted branch still merges from local base
- GIVEN a temp repo where `feat-gone` was deleted on the remote
- AND local `main` already contains the branch tip
- WHEN cleanup runs
- THEN it MUST report `would remove feat-gone`

### Requirement: Conservative Skip for Dirty Worktrees
The system MUST still skip worktrees with uncommitted changes, blocking untracked files, or active in-progress merges before any merge detection.

#### Scenario: Dirty worktree overrides merged verdict
- GIVEN a temp repo where `feat-dirty` is otherwise merged into `main`
- AND the worktree has uncommitted changes
- WHEN cleanup runs
- THEN it MUST report `skipped feat-dirty (dirty)`
- AND it MUST not remove the worktree even if merge evidence exists

### Requirement: Bounded Candidate Resolution
Candidate-base resolution MUST use only refs already present in the local repository. It MUST NOT trigger `git fetch` or any network operation.

#### Scenario: Missing remote does not fetch
- GIVEN `origin/main` is absent locally
- AND local `main` plus its upstream ref are already present
- WHEN cleanup resolves base candidates
- THEN it MUST complete without fetch or network access
- AND it MUST decide using only local refs
