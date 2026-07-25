# Design: relocate test-* recipe fixtures

## Problem

`hide-test-recipes` filters `test-*` from user-facing **lists/pickers**, but
fixtures still:

1. Ship inside `catalog/recipes/` (installed CLI tree).
2. Accept `recipe add` / `recipe init` by typed id.
3. Materialize on sync if somehow enabled (skills/commands leak into agents).

## Decisions

### D1 — Move, do not dual-ship

Move all `catalog/recipes/test-*` directories to `tests/fixtures/recipes/`.
Production catalog resolution stays a single root:
`$AI_SPECS_HOME/catalog/recipes`. No test overlay in production code.

### D2 — Shared helper for tests that need fixtures

Add `tests/helpers/fixture_catalog.py`:

| Helper | Role |
|---|---|
| `FIXTURE_RECIPES` | `tests/fixtures/recipes` |
| `PUBLIC_RECIPES` | `catalog/recipes` |
| `unit_catalog()` | path for unit tests that only need fixture recipes |
| `cli_home_with_fixtures(tmp)` | temp CLI home whose `catalog/recipes/` contains **symlinks** (or copies) of every public recipe **plus** every fixture recipe |

Tests that today use `AI_SPECS_HOME=ROOT` and enable `test-fixture` MUST switch
to `cli_home_with_fixtures(...)` (or equivalent) so materialize still finds
fixtures without shipping them.

### D3 — Block typed install of internal ids

`recipe add` and `recipe init` call `util.is_internal_test_recipe(id)` and
refuse with a clear error (exit 1). Hub pickers already omit them via
`list_recipes`.

Defense-in-depth (same change): `recipe-materialize` skips / hard-fails when an
enabled manifest id is `test-*` **and** the recipe is absent from the production
catalog (after the move, absence is the normal case). Prefer an explicit
reject-if-internal before catalog lookup so a stray directory cannot re-open the
leak.

### D4 — Docs

Remove the stale “ignore `test-*` in `recipe list`” note from
`docs/recipes-catalog.md`. Optionally one line under contributor/testing docs
pointing at `tests/fixtures/recipes/` — only if an existing testing doc already
discusses catalog fixtures; do not create a new markdown file unless needed.

## Non-goals

- Renaming public recipes.
- Hiding legitimate skills like `testing-foundation` / `tdd-flow`.
- Changing the `test-` prefix convention.

## Risk

| Risk | Mitigation |
|---|---|
| Many tests hardcode `CATALOG = catalog/recipes` + `test-fixture` | Grep-driven update; helper + `./tests/validate.sh` |
| Symlinks on Windows CI | Prefer `copytree` if the suite already avoids symlinks; otherwise match existing test patterns |
| Spec examples still say `recipe add test-fixture` succeeds | Spec delta rewrites happy-path to a public/synthetic id; internal ids get reject scenarios |
