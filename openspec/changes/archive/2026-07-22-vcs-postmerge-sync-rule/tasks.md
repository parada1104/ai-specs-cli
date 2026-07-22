# Tasks: surface post-merge base-sync across VCS recipes

## Planning depth

- **Classification**: Standard (spec + tasks). Touches the `vcs-pr-flow` brief
  contract across 3 recipes + skill/command fixes.
- **Authorization**: plan + scope ("Core + secundario") approved by maintainer
  (session 2026-07-22). worktree-flow warn deferred.

## Implementation (red-green-refactor)

- [x] RED: extend `test_{git_pr_flow,gitlab_mr_flow,bitbucket_pr_flow}_recipe.py`
      to assert the recipe's brief `workflow_rules` include a post-merge
      base-sync rule (mentions `ff-only`) and a post-merge cleanup rule.
- [x] GREEN: add the base-sync `workflow_rule` to all three recipe.toml; add the
      cleanup `workflow_rule` to gitlab + bitbucket; bump versions.
- [x] Fix `gitlab-merge-workflow` post-merge pull `origin` → `$REMOTE`.
- [x] Fix duplicated step numbers in the 3 merge skills and 3 create commands.
- [x] Spec delta in `openspec/specs/vcs-pr-flow/spec.md` (brief rules requirement).

## Validation

- [x] `./tests/validate.sh` exit 0; full `pytest tests/` green.
- [x] New brief-rule assertions pass for all three recipes.
