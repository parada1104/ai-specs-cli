# Spec delta: worktree-flow

## ADDED Requirements

### Requirement: Leave worktree before removal

Any cleanup path (script or agent-driven) MUST ensure the process working
directory is outside the worktree being removed. Removing a worktree while
`$PWD` is inside it is a hard failure mode and MUST be avoided.

#### Scenario: Cleanup refuses or relocates when cwd is inside target

- GIVEN the current working directory is inside `.worktrees/<slug>`
- WHEN cleanup is requested for that slug
- THEN cleanup first changes to the main repository root
- AND then removes the worktree

### Requirement: Script-first post-merge cleanup

Post-merge cleanup guidance SHALL prefer
`ai-specs/recipes/worktree-flow/bin/worktree-cleanup.sh` (or the catalog
template equivalent) with `--base <integration_branch>` over ad-hoc
`git worktree remove` sequences. The script remains conservative: dirty and
unmerged worktrees are skipped; squash/rebase merges remain detectable.

#### Scenario: Skill documents script-first cleanup

- GIVEN `worktree-flow` skill and VCS merge skills after this change
- WHEN post-merge cleanup instructions are inspected
- THEN they name `worktree-cleanup.sh` as the preferred path
- AND they require leaving the worktree before removal
