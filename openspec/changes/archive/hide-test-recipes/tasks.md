# hide-test-recipes

Depth: light

## Goal

Recipes with catalog id prefix `test-` must never appear in user-facing
pickers/lists: hub Recipes submenu, CLI `recipe list`, and init wizard/onboarding.

## Tasks

- [x] Add shared `is_internal_test_recipe(id)` helper (`startswith("test-")`)
- [x] Filter via helper in `list_recipes` (hub + CLI list)
- [x] Use same helper in `init_tui._catalog_recipes` (wizard/onboarding)
- [x] Unit tests: list_recipes and init catalog exclude `test-*`
- [x] Update CLI list output assertion (no longer expects `test-fixture`)
- [x] `./tests/validate.sh` green

## Evidence

- RED: `test_list_hides_internal_test_recipes` failed with `['public-recipe', 'test-fixture']`
- GREEN: focused `test_recipe_list` + `TestCatalogRecipes` OK after helper wiring
- SMOKE: `./tests/validate.sh` exit 0
