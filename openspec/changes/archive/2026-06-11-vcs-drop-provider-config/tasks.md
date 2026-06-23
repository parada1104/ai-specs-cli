# Tasks: vcs-drop-provider-config

## 1. Renderer and contract tests (RED first)

- [x] 1.1 RED: add failing tests in `tests/test_agents_render_brief_fragments.py` (or `tests/test_sync_pipeline.py`) asserting VCS bullet comes from recipe id, not `config.provider`; assert stale `provider` key warns but does not affect render.
- [x] 1.2 RED: update `tests/test_git_pr_flow_recipe.py` and `tests/test_gitlab_mr_flow_recipe.py` to assert `provider` field is absent from recipe schema.
- [x] 1.3 RED: add `tests/test_bitbucket_pr_flow_recipe.py` assertions (after cherry-pick) that `provider` is absent.

## 2. Renderer implementation (GREEN)

- [x] 2.1 GREEN: add `_VCS_RECIPE_LABELS` map in `lib/_internal/agents-render.py`; derive Runtime Flow VCS bullet from `bindings.vcs-pr-flow` recipe id + `base_branch` only.
- [x] 2.2 GREEN: rerun focused renderer tests until GREEN.

## 3. Recipe manifests and assets

- [x] 3.1 GREEN: remove `[config.provider]` from `catalog/recipes/git-pr-flow/recipe.toml`; fix `[provides.brief].workflow_rules` to hardcoded GitHub/gh prose.
- [x] 3.2 GREEN: same for `catalog/recipes/gitlab-mr-flow/recipe.toml` (GitLab/glab).
- [x] 3.3 GREEN: cherry-pick or merge `feat/bitbucket-pr-flow` assets into this branch **without** `[config.provider]`; fix brief fragments.
- [x] 3.4 GREEN: update bundled skills/commands/READMEs for all three recipes — config sections document only `base_branch`; remove `provider` references.

## 4. Documentation

- [x] 4.1 GREEN: update `docs/recipes-catalog.md` — remove `provider` from config tables; mark VCS recipes as specific providers.
- [x] 4.2 GREEN: update `docs/capabilities.md` tier table (`git-pr-flow` → specific, not foundational).
- [x] 4.3 GREEN: update `docs/recipe-schema.md` if it documents `provider` for VCS recipes.
- [x] 4.4 GREEN: extend `tests/test_recipes_catalog.py` contract tests for the new doc shape (no `provider` in README config tables).

## 5. Verification

- [x] 5.1 Run `./tests/run.sh tests/test_git_pr_flow_recipe.py tests/test_gitlab_mr_flow_recipe.py tests/test_bitbucket_pr_flow_recipe.py tests/test_recipes_catalog.py` (focused).
- [x] 5.2 Run `./tests/validate.sh` (full validation).
- [x] 5.3 Write `verify-report.md` comparing implementation to delta specs.
