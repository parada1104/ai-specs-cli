# Surface the post-merge base-sync rule across all VCS recipes

## Problem

All three VCS sibling recipes (`git-pr-flow`, `gitlab-mr-flow`, `bitbucket-pr-flow`,
all providing `vcs-pr-flow`) already sync the integration branch after a merge in
their **merge skill** (git step 8, gitlab/bitbucket step 10:
`git checkout <base>` then `git pull --ff-only`). But that step lives only in the
lazy-loaded skill — it is **not** surfaced in the always-on brief
(`[provides.brief].workflow_rules`). An agent that merges without having loaded
the merge skill leaves the canonical checkout behind origin (observed this
session across #138/#139/#141).

Current brief state:

| Recipe | base-sync rule in brief | cleanup rule in brief |
|---|---|---|
| git-pr-flow | ❌ | ✅ |
| gitlab-mr-flow | ❌ | ❌ |
| bitbucket-pr-flow | ❌ | ❌ |

Secondary issues found in the same files: `gitlab-merge-workflow` is internally
inconsistent in its remote handling (push uses the resolved `$REMOTE`, the
post-merge pull hardcodes `origin`); and every merge skill plus every create
command has a duplicated step number (dup `4.` in skills; dup `4.`/`5.` in
commands).

## Solution

Ownership: **the VCS recipe owns the post-merge base-sync** (the merge is the
event that makes the base stale, and the merge is VCS-recipe territory).
worktree-flow stays purely worktree-lifecycle and keeps its "never touches the
main worktree" invariant.

1. **Surface base-sync in the brief** of all three recipes: add a
   `workflow_rules` entry — "After a merged PR/MR, sync `{config.base_branch}` in
   the main worktree: `git checkout {config.base_branch}` then
   `git pull --ff-only`." (PR vs MR wording per provider).
2. **Parity cleanup rule**: add the post-merge cleanup `workflow_rule` (which
   only `git-pr-flow` has) to `gitlab-mr-flow` and `bitbucket-pr-flow`.
3. **Version bumps** (additive brief change): git 1.4.1→1.5.0, gitlab
   1.3.1→1.4.0, bitbucket 1.2.1→1.3.0.
4. **Spec delta** on `vcs-pr-flow`: require each sibling to surface both a
   post-merge base-sync rule and a cleanup rule in its brief.
5. **Remote consistency (sync step)**: fix `gitlab-merge-workflow`'s post-merge
   pull to use the already-resolved `$REMOTE` (matches its own push and
   bitbucket). `git-pr-flow` intentionally keeps `origin` (internally consistent
   and the `gh` convention — not a bug); noted, not changed.
6. **Step-numbering fixes**: correct the duplicated step numbers in the three
   merge skills and the three create commands.

## Affected modules

- `catalog/recipes/{git-pr-flow,gitlab-mr-flow,bitbucket-pr-flow}/recipe.toml`
- `catalog/recipes/{...}/skills/*/SKILL.md` (gitlab remote fix; step numbering ×3)
- `catalog/recipes/{...}/commands/*.md` (step numbering ×3)
- `tests/test_{git_pr_flow,gitlab_mr_flow,bitbucket_pr_flow}_recipe.py` (assert brief rules)
- `openspec/specs/vcs-pr-flow/spec.md` (delta)

## Out of scope

- Refactoring `git-pr-flow` to a resolved `$REMOTE` (it uses `origin` throughout,
  consistently, aligned with `gh`).
- The optional worktree-flow "warn if base behind origin" nudge (deferred).
