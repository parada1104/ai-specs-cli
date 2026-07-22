## ADDED Requirements

### Requirement: Post-merge brief rules

Each VCS sibling recipe (`git-pr-flow`, `gitlab-mr-flow`, `bitbucket-pr-flow`)
MUST surface, in its always-on brief (`[provides.brief].workflow_rules`), both:

1. a **post-merge base-sync** rule instructing that, after a merged PR/MR, the
   integration branch `{config.base_branch}` is synced in the main worktree
   (`git checkout {config.base_branch}` then `git pull --ff-only`); and
2. a **post-merge cleanup** rule instructing that the feature worktree and local
   branch are removed after merge (`git branch -D` after a squash merge), and the
   remote branch deleted if it still exists.

These rules mirror the merge skill's post-merge sequence so the guidance is
present even when the lazy-loaded merge skill has not been read. The base-sync
responsibility belongs to the VCS recipe (the merge is what makes the base
stale); worktree-flow MUST NOT perform base-sync (it never mutates the main
worktree).

#### Scenario: Base-sync rule present in every sibling brief

- GIVEN any of the three VCS sibling recipes
- WHEN its `workflow_rules` are rendered into the brief
- THEN the rules MUST include a post-merge base-sync rule referencing
  `git pull --ff-only` and `{config.base_branch}`

#### Scenario: Cleanup rule present in every sibling brief

- GIVEN any of the three VCS sibling recipes
- WHEN its `workflow_rules` are rendered into the brief
- THEN the rules MUST include a post-merge worktree/branch cleanup rule

#### Scenario: Sync uses the resolved remote

- GIVEN a sibling whose merge skill resolves a remote variable for pushing
- WHEN the skill performs the post-merge base-sync pull
- THEN it MUST pull from the same resolved remote (not a hardcoded remote that
  contradicts its push remote)
