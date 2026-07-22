# Archive report — vcs-postmerge-sync-rule

**Archived:** 2026-07-22
**Branch:** feat/vcs-postmerge-sync-rule
**Status:** ready-to-merge

## Outcome

- All three `vcs-pr-flow` siblings (`git-pr-flow`, `gitlab-mr-flow`,
  `bitbucket-pr-flow`) now surface a **post-merge base-sync** `workflow_rule`
  in their always-on brief (`git checkout {config.base_branch}` then
  `git pull --ff-only`). Added the **cleanup** `workflow_rule` to gitlab and
  bitbucket (only git-pr-flow had it). This closes the gap where the sync lived
  only in the lazy merge skill, so an agent merging without loading the skill
  left the canonical checkout behind (observed this session with #138/#139/#141).
- Versions bumped: git-pr-flow 1.4.1→1.5.0, gitlab-mr-flow 1.3.1→1.4.0,
  bitbucket-pr-flow 1.2.1→1.3.0.
- Secondary: made the post-merge pull self-contained by re-resolving `REMOTE`
  inside the sync block in gitlab and bitbucket skills — fixes gitlab's
  push=`$REMOTE`/pull=`origin` inconsistency and bitbucket's out-of-scope
  `$REMOTE`. `git-pr-flow` keeps `origin` (internally consistent, `gh`
  convention). Fixed duplicated step numbers in all three merge skills and
  create commands.

## Ownership decision

The VCS recipe owns post-merge base-sync (the merge is the event that makes the
base stale). worktree-flow stays worktree-lifecycle only and never mutates the
main worktree. The optional worktree-flow "warn if base behind origin" nudge was
deferred by the maintainer.

## Files changed

- `catalog/recipes/{git-pr-flow,gitlab-mr-flow,bitbucket-pr-flow}/recipe.toml` — workflow_rules + version.
- `catalog/recipes/{gitlab-mr-flow,bitbucket-pr-flow}/skills/*/SKILL.md` — self-contained REMOTE in sync block.
- `catalog/recipes/{...}/skills/*/SKILL.md` + `commands/*.md` — step-number fixes.
- `tests/test_{git_pr_flow,gitlab_mr_flow,bitbucket_pr_flow}_recipe.py` — brief-rule assertions.
- `openspec/specs/vcs-pr-flow/spec.md` — "Post-merge brief rules" requirement promoted from delta.

## Verification

- `./tests/validate.sh` — exit 0.
- Full `pytest tests/` — 1013 passed, 143 subtests passed (includes 3 new brief-rule tests).
