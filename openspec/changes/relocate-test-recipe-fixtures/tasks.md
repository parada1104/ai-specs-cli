# Tasks: relocate-test-recipe-fixtures

Depth: standard

Trello: [#54](https://trello.com/c/PeXcZwhK) — Relocate test-* catalog fixtures out of shipped catalog

## Goal

Close remaining leak paths for internal `test-*` recipes: docs, CLI add/init
block, and physical relocation out of the shipped catalog.

## Tasks

### 1. Docs
- [x] Remove stale “ignore `test-*`” paragraph from `docs/recipes-catalog.md`
- [x] If an existing testing/contributor doc mentions catalog fixtures, point it
      at `tests/fixtures/recipes/` (no new doc file unless required)
      *(no existing doc needed a pointer; fixtures path is in design + helper)*

### 2. Block typed install (add / init)
- [x] In `lib/_internal/recipe-add.py`, reject `is_internal_test_recipe(id)`
      before catalog success path; clear stderr + exit 1; no manifest mutation
- [x] In `lib/_internal/recipe-init.py`, same reject
- [x] Unit tests: add/init of `test-fixture` fails; happy-path uses non-`test-*` id

### 3. Relocate fixtures
- [x] `git mv` each `catalog/recipes/test-*` → `tests/fixtures/recipes/<id>/`
      (seven dirs: test-fixture, test-conflict-{a,b}, test-cmd-conflict-{a,b},
      test-mcp-conflict-{a,b})
- [x] Add `tests/_fixture_catalog.py` (FIXTURE_RECIPES, PUBLIC_RECIPES,
      `unit_catalog()`, `populate_catalog` / `cli_home_with_fixtures`)
- [x] Update unit tests that read fixtures via `CATALOG = catalog/recipes` to use
      the helper / fixture path
- [x] Update materialize / external-dirs / sync tests that set `AI_SPECS_HOME=ROOT`
      and enable `test-fixture` to use fixture CLI home + allowlist env
- [x] Assert shipped `catalog/recipes/` contains zero `test-*` dirs
      (`test_recipes_catalog.py`)

### 4. Materialize defense-in-depth
- [x] Refuse / skip materializing enabled ids that match
      `is_internal_test_recipe` (explicit error via `fail()` / sys.exit, unless
      `AI_SPECS_ALLOW_INTERNAL_TEST_RECIPES=1`)
- [x] Test: manifest with `[recipes.test-fixture] enabled = true` + production
      catalog (no fixture) does not create `test-skill` / `test-command`

### 5. Spec / changelog hygiene
- [x] Keep change-folder delta under `openspec/changes/.../specs/recipe-cli/`
- [x] Update archived-style examples in live `openspec/specs/recipe-cli/spec.md`
      only as needed for consistency with reject + non-internal happy path
- [x] CHANGELOG under Unreleased: Fixed/Changed note for fixtures relocation +
      add/init block

### 6. Verify
- [x] RED: focused test for add reject / catalog emptiness before full green
- [x] GREEN: `./tests/validate.sh` — **1052 tests OK, exit 0** (~269s)
- [x] Smoke: `recipe list` has no `test-*`; `recipe add test-fixture` rejects
      (exit 1); `catalog/recipes` has no `test-*`

## Out of scope

- Renaming public recipes or skills (`testing-foundation`, `tdd-flow`)
- Installer packaging changes beyond “fixtures no longer under catalog/”
