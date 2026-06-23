# Proposal: GitLab MR Flow Recipe

## Intent

GitLab projects need the same recipe-driven VCS review flow that `git-pr-flow` provides for GitHub, without changing the semantic capability contract. Add `gitlab-mr-flow` as a sibling recipe that provides `vcs-pr-flow`, materializes a GitLab MR skill/command, and targets `development` by default.

## Scope

### In Scope
- Add `catalog/recipes/gitlab-mr-flow/recipe.toml` with `vcs-pr-flow`, `provider = "gitlab"`, `base_branch = "development"`, `validate-config`, docs, skill, and command provisions.
- Add bundled `gitlab-merge-workflow` skill and `/mr-create` command using explicit `git push` then `glab mr create`.
- Add README/docs and tests proving recipe list/add/sync materialization and provider binding behavior.

### Out of Scope
- MR merge automation, CI behavior changes, or auto-merge.
- GitLab instance-specific auth setup.
- Renderer changes unless tests expose a required clarity gap.

## Capabilities

### New Capabilities
- `vcs-pr-flow`: Specify the provider-swappable review-flow contract for GitLab MR creation, explicit push, configured target branch, and no auto-merge.

### Modified Capabilities
- None.

## Approach

Mirror `git-pr-flow` as a low-risk sibling recipe with distinct primitive IDs (`gitlab-merge-workflow`, `mr-create`). Keep `glab` install/auth checks in runtime skill/command guardrails because `validate-config` only validates manifest config. Use strict TDD with tests modeled after `test_git_pr_flow_recipe.py`.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `catalog/recipes/gitlab-mr-flow/` | New | Recipe manifest, README, skill, command. |
| `docs/recipes-catalog.md` | Modified | Document catalog availability. |
| `docs/capabilities.md` | Modified | Name GitLab as an actual `vcs-pr-flow` provider. |
| `tests/test_gitlab_mr_flow_recipe.py` | New | Manifest/materialization coverage. |
| `tests/test_recipe_materialize.py` | Modified | Targeted GitHub+GitLab binding/ambiguity coverage if needed. |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| GitHub and GitLab both enabled leave `vcs-pr-flow` unbound. | Med | Document explicit `[[bindings]]`; add focused test. |
| `glab mr create --fill` pushes implicitly. | Med | Command uses explicit push and avoids `--fill`. |
| `glab` unavailable/auth missing at runtime. | Med | Skill/command check `command -v glab` and `glab auth status`. |
| GitLab provider unclear in brief. | Low | Recipe prose names `glab`; avoid renderer scope unless needed. |

## Rollback Plan

Remove `catalog/recipes/gitlab-mr-flow/`, related docs/tests/spec deltas, and any manifest entries or `[[bindings]]` using `gitlab-mr-flow`; rerun `ai-specs sync` to dematerialize project assets.

## Dependencies

- Runtime users must have `glab` installed and authenticated.
- Projects enabling both providers must bind `vcs-pr-flow` explicitly.

## Success Criteria

- [ ] `ai-specs recipe list` shows `gitlab-mr-flow` as available.
- [ ] `ai-specs recipe add gitlab-mr-flow` plus sync materializes skill, command, and README.
- [ ] GitLab repo flow works end-to-end: branch push, MR created against `development`, no merge.
