## Phase 1: Manifest and Binding RED→GREEN

- [x] 1.1 RED: create failing manifest assertions in `tests/test_gitlab_mr_flow_recipe.py` for `vcs-pr-flow`, `provider=gitlab`, `base_branch=development`, `validate-config`, skill, command, and docs.
- [x] 1.2 GREEN: create `catalog/recipes/gitlab-mr-flow/recipe.toml` matching the contract; run `./tests/run.sh tests/test_gitlab_mr_flow_recipe.py` or nearest supported focused unittest.
- [x] 1.3 REFACTOR: align recipe metadata/provisions with `catalog/recipes/git-pr-flow/recipe.toml`; rerun the focused test.

## Phase 2: Materialization and Provider Semantics

- [x] 2.1 RED: extend `tests/test_gitlab_mr_flow_recipe.py` for materialized `SKILL.md`, `commands/mr-create.md`, and README paths without touching GitHub assets; depends on 1.2.
- [x] 2.2 RED: add focused ambiguity/explicit-binding coverage in `tests/test_recipe_materialize.py` only if existing resolver tests do not cover dual `vcs-pr-flow` providers.
- [x] 2.3 GREEN: add minimal recipe assets placeholders under `catalog/recipes/gitlab-mr-flow/` and, only if tests require it, adjust `lib/_internal/recipe-materialize.py` binding behavior.
- [x] 2.4 REFACTOR: remove duplicated fixture setup between GitHub/GitLab recipe tests where practical; rerun `./tests/run.sh`.