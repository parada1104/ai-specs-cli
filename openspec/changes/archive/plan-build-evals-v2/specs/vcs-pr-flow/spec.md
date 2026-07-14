# Spec delta: vcs-pr-flow

## ADDED Requirements

### Requirement: Pre-merge archive hard stop

Provider merge-workflow skills SHALL NOT merge a PR/MR until the matching
change folder has been archived on the review branch. Active
`openspec/changes/<slug>/` (excluding `archive/`) is a hard blocker.

#### Scenario: GitHub merge blocked with active change folder

- GIVEN a PR ready to merge
- AND `openspec/changes/<slug>/` still exists on the review branch
- WHEN the merge workflow evaluates preconditions
- THEN merge MUST NOT run
- AND the skill reports that archive-tail must complete first

### Requirement: Mandatory post-merge worktree cleanup

After a successful merge, provider merge-workflow skills SHALL:

1. Change directory to the main repository root (never remove a worktree while
   `$PWD` is inside it)
2. Prefer the materialized `worktree-cleanup.sh` with the configured
   integration/base branch
3. Fall back to explicit `git worktree remove` + force-delete of the local
   feature branch only when the script is unavailable
4. Stop without deleting if the worktree is dirty

#### Scenario: Cleanup runs from main root after squash merge

- GIVEN a squash-merged PR whose worktree still exists under `.worktrees/`
- WHEN post-merge cleanup runs
- THEN the agent is not inside the worktree path
- AND the worktree is removed (via cleanup script or equivalent)
- AND the local feature branch is deleted with `-D` when squash-merged
