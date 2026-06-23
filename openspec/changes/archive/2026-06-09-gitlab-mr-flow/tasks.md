# Tasks: GitLab MR Flow Recipe

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 650-900 |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 manifest+materialization tests → PR 2 skill+command assets → PR 3 docs+validation |
| Delivery strategy | auto-forecast |
| Chain strategy | pending |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: pending
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | Recipe manifest and sync materialization | PR 1 | base `development`; proves schema, assets, binding behavior |
| 2 | GitLab skill and command content | PR 2 | depends on PR 1; golden checks for push-before-create |
| 3 | Catalog/capability docs and final validation | PR 3 | depends on PR 2; docs and smoke evidence |

## Phase 1: Manifest and Binding RED→GREEN

- [x] 1.1 RED: create failing manifest assertions in `tests/test_gitlab_mr_flow_recipe.py` for `vcs-pr-flow`, `provider=gitlab`, `base_branch=development`, `validate-config`, skill, command, and docs.
- [x] 1.2 GREEN: create `catalog/recipes/gitlab-mr-flow/recipe.toml` matching the contract; run `./tests/run.sh tests/test_gitlab_mr_flow_recipe.py` or nearest supported focused unittest.
- [x] 1.3 REFACTOR: align recipe metadata/provisions with `catalog/recipes/git-pr-flow/recipe.toml`; rerun the focused test.

## Phase 2: Materialization and Provider Semantics

- [x] 2.1 RED: extend `tests/test_gitlab_mr_flow_recipe.py` for materialized `SKILL.md`, `commands/mr-create.md`, and README paths without touching GitHub assets; depends on 1.2.
- [x] 2.2 RED: add focused ambiguity/explicit-binding coverage in `tests/test_recipe_materialize.py` only if existing resolver tests do not cover dual `vcs-pr-flow` providers.
- [x] 2.3 GREEN: add minimal recipe assets placeholders under `catalog/recipes/gitlab-mr-flow/` and, only if tests require it, adjust `lib/_internal/recipe-materialize.py` binding behavior.
- [x] 2.4 REFACTOR: remove duplicated fixture setup between GitHub/GitLab recipe tests where practical; rerun `./tests/run.sh`.

## Phase 3: Skill and Command Golden Content

- [x] 3.1 RED: add golden text checks in `tests/test_gitlab_mr_flow_recipe.py` for `command -v glab`, `glab auth status`, explicit `git push -u origin`, `glab mr create --source-branch --target-branch --title --description --yes`, and no `--fill`/auto-merge.
- [x] 3.2 GREEN: write `catalog/recipes/gitlab-mr-flow/skills/gitlab-merge-workflow/SKILL.md` with MR creation, verification evidence, approval-gated merge, cleanup, and blockers.
- [x] 3.3 GREEN: write `catalog/recipes/gitlab-mr-flow/commands/mr-create.md` as thin MR creation only; stop after MR URL.
- [x] 3.4 REFACTOR: tighten wording against the spec scenarios; rerun focused tests.

## Phase 4: Docs and Verification

- [x] 4.1 RED: extend `tests/test_recipes_catalog.py` or docs contract tests for `gitlab-mr-flow` catalog and `vcs-pr-flow` provider wording.
- [x] 4.2 GREEN: write `catalog/recipes/gitlab-mr-flow/README.md`; update `docs/recipes-catalog.md` and `docs/capabilities.md` with enablement, binding, prerequisites, explicit push, and no auto-merge policy.
- [x] 4.3 REFACTOR: verify docs link paths and terminology; run `./tests/run.sh` then `./tests/validate.sh` and record RED/GREEN evidence.
