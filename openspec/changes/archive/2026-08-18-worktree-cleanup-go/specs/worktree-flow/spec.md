# Worktree cleanup Go migration and remote branch deletion

## Requirement: Go cleanup implementation and stable launcher

The `worktree-flow` cleanup command MUST execute the cleanup implementation from
the existing zero-dependency Go module at
`catalog/recipes/worktree-flow/gate`. The materialized
`worktree-cleanup.sh` path MUST remain stable and MUST be a thin launcher that
resolves the current verified version-keyed Go binary. Cleanup MUST fail closed
with a loud diagnostic and perform no destructive action when no verified binary,
current receipt, or supported implementation is available. It MUST NOT silently
fall back to the removed Bash cleanup algorithm.

### Scenario: Verified binary is selected

- GIVEN a current platform/version cache binary has a matching committed digest,
  current version, passing self-test, and current verification receipt
- WHEN the cleanup launcher runs
- THEN it MUST execute the Go cleanup subcommand
- AND it MUST preserve the existing cleanup flags and output contract

### Scenario: Missing binary fails closed

- GIVEN the cleanup launcher cannot resolve a verified Go binary
- WHEN cleanup runs
- THEN it MUST exit non-zero
- AND it MUST report that no destructive action was taken
- AND it MUST not remove a worktree, local branch, or remote branch

## Requirement: Ordered merge proof preservation

A worktree branch MUST be considered merged only when an ordered base candidate
proves ancestry or complete equivalent changes. Candidate resolution MUST use
exact base, configured upstream, configured remote-tracking ref, and conditional
`origin/<base>` fallback in that order, with positive local ref proof and no
fetch. Merge classification MUST perform ancestry first, then `git cherry`
patch-id, combined patch-id, and combined final-tree-entry equivalence.
Combined-tree comparison MUST consume NUL-delimited paths so filenames with
newlines compare verbatim. A partial squash or later-reverted squash MUST remain
unmerged.

### Scenario: Regular and fast-forward merge is eligible

- GIVEN a clean branch with one or more commits
- AND the branch tip is an ancestor of the selected base
- WHEN cleanup runs in dry-run mode
- THEN it MUST report the worktree as eligible for removal

### Scenario: Complete multi-commit squash is eligible

- GIVEN a clean branch with multiple commits
- AND all branch changes are represented by a complete squash or equivalent
  combined tree in the selected base
- AND the branch tip is not an ancestor of the base
- WHEN cleanup runs
- THEN the combined patch/tree proof MUST classify it as merged

### Scenario: Partial squash is unmerged

- GIVEN only a strict subset of a branch's changes appears in the selected base
- WHEN cleanup evaluates the branch
- THEN it MUST report `skipped <name> (unmerged)`
- AND it MUST preserve the worktree and branch

### Scenario: Reverted squash is unmerged

- GIVEN a complete squash was later reverted from the selected base
- WHEN cleanup evaluates the branch
- THEN it MUST report `skipped <name> (unmerged)`
- AND it MUST preserve the worktree and branch

### Scenario: Newline path is compared verbatim

- GIVEN a branch changes a path containing a newline
- AND the selected base does not contain the same final tree entry
- WHEN combined-tree proof runs
- THEN it MUST reject the equivalence
- AND it MUST preserve the worktree and branch

## Requirement: Conservative classification

Cleanup MUST preserve dirty, detached, unmerged, main, and otherwise ineligible
worktrees. Dirty status MUST be checked before merge proof. A branch MUST NOT be
deleted while any worktree still holds it. The main worktree MUST never be
removed.

### Scenario: Dirty merged worktree is preserved

- GIVEN a branch is merged but its linked worktree has uncommitted changes
- WHEN cleanup runs
- THEN it MUST report `skipped <name> (dirty)`
- AND it MUST perform no destructive action for that worktree

### Scenario: Unmerged worktree is preserved

- GIVEN a clean worktree branch has a change not proven present in the selected
  base
- WHEN cleanup runs
- THEN it MUST report `skipped <name> (unmerged)`
- AND it MUST preserve the worktree and branch

### Scenario: Held branch is protected

- GIVEN a branch is otherwise eligible but another worktree still holds its
  branch reference
- WHEN cleanup reaches deletion
- THEN it MUST refuse loudly
- AND it MUST not delete the local or remote branch

## Requirement: Protected branch names at every destructive entry point

The cleanup implementation MUST build a protected-name set containing `main`,
`master`, `development`, `staging`, the configured `base_branch`, and the
configured `integration_branch`. Immediately before every `git worktree remove`,
local branch deletion, and remote branch deletion, it MUST re-check the current
branch/name against that set. A protected name reaching a delete path MUST be a
loud non-zero refusal, not a silent skip.

### Scenario: Built-in protected name refuses worktree removal

- GIVEN an otherwise eligible worktree is named/held by branch `main` (or
  `master`, `development`, or `staging`)
- WHEN cleanup reaches the worktree-removal entry point
- THEN it MUST refuse loudly
- AND it MUST not invoke `git worktree remove`

### Scenario: Built-in protected name refuses local branch deletion

- GIVEN an otherwise eligible branch reaches local branch deletion
- AND its name is in the built-in protected set
- WHEN cleanup reaches the local deletion entry point
- THEN it MUST refuse loudly
- AND it MUST not invoke `git branch -d` or `git branch -D`

### Scenario: Built-in protected name refuses remote deletion

- GIVEN an otherwise eligible branch reaches remote deletion
- AND its name is in the built-in protected set
- WHEN cleanup reaches the remote deletion entry point
- THEN it MUST refuse loudly
- AND it MUST not invoke `git push --delete`

### Scenario: Configured base and integration names are protected

- GIVEN configured `base_branch` or `integration_branch` names are not one of the
  built-ins
- WHEN a branch with either configured name reaches any destructive entry point
- THEN cleanup MUST refuse loudly before the destructive Git command

## Requirement: Remote deletion and independent verification

After a clean, proven-merged worktree is removed from the main worktree, normal
cleanup MUST delete its remote branch using the selected remote and MUST verify
that `git ls-remote --heads <remote> <branch>` returns no matching ref. Remote
delete MUST NOT occur for dirty, detached, unmerged, protected, or still-held
branches. A failed delete or non-empty verification MUST produce a loud
non-success and MUST NOT claim remote cleanup succeeded.

### Scenario: Merged remote branch is deleted and verified

- GIVEN a clean worktree branch is proven merged
- AND its remote branch exists
- AND cleanup runs from the main worktree in normal mode
- WHEN cleanup deletes the candidate
- THEN it MUST run `git push <remote> --delete <branch>`
- AND it MUST run `git ls-remote --heads <remote> <branch>`
- AND it MUST report success only when the remote ref is absent

### Scenario: Remote verification catches a surviving ref

- GIVEN the remote deletion command returns but `git ls-remote --heads` still
  returns the branch
- WHEN cleanup verifies the result
- THEN it MUST exit non-zero or report an explicit refusal/error
- AND it MUST NOT report the remote branch as deleted

### Scenario: Remote failure is loud

- GIVEN `git push --delete` fails or the remote cannot be resolved
- WHEN cleanup reaches remote deletion
- THEN it MUST report a loud non-success
- AND it MUST not claim the remote branch was removed

## Requirement: Structural batch iteration and topology preservation

Cleanup MUST represent repository passes and worktree candidates as structured
collections and MUST iterate every selected member. Under `standalone` and
`monorepo-apps`, it MUST scan the root repository once. Under
`monorepo-submodules`, it MUST scan every initialized in-scope submodule and
skip uninitialized modules. The superproject worktree list MUST NOT be treated
as the source for submodule-owned linked worktrees.

### Scenario: Multiple candidates are all visited

- GIVEN two clean, merged feature worktrees are in scope
- WHEN cleanup runs in dry-run or normal mode
- THEN it MUST emit an eligibility/removal result for both candidates
- AND it MUST not stop after the first candidate

### Scenario: Multiple initialized submodules are all visited

- GIVEN two initialized submodules each own an in-scope linked worktree
- WHEN cleanup runs without a module scope
- THEN it MUST scan both module repositories
- AND it MUST consider both candidates independently

### Scenario: Uninitialized module is skipped

- GIVEN `.gitmodules` contains an uninitialized module
- WHEN cleanup runs
- THEN it MUST not invoke Git worktree operations in that module
- AND it MUST continue safely with initialized modules

### Scenario: Module scope limits batch

- GIVEN initialized modules `apps/api` and `apps/web` each have an eligible
  worktree
- WHEN cleanup runs with `--submodule apps/api`
- THEN it MUST process `apps/api`
- AND it MUST not process or report `apps/web`

## Requirement: Remote cleanup runs from the main worktree

The documented post-merge cleanup operation MUST be invoked from the main
repository worktree after the pull request merge. The cleanup command MUST NOT
switch to the base branch or require the base branch to be free in another
worktree. The workflow documentation MUST explain that GitHub's repo-wide
`delete_branch_on_merge` and `gh pr merge --delete-branch` are not substitutes
for this explicit cleanup step in a multi-worktree layout.

### Scenario: Main-worktree invocation owns remote deletion

- GIVEN the integration branch is checked out in the main worktree
- AND a feature worktree remains linked after its PR merge
- WHEN cleanup is invoked from the main worktree
- THEN it MUST be able to remove the feature worktree/local branch
- AND it MUST own and verify remote deletion without checking out the base
