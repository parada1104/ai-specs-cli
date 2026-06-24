# Tasks: Recipe tags/categories + conflicts_with

## Phase 1 — Schema (RED → GREEN)

- [x] 1.1 Add `tags: list[str]` and `conflicts_with: list[str]` to `Recipe`
      dataclass (default empty).
- [x] 1.2 Implement `_parse_tags()` (array-of-strings validation).
- [x] 1.3 Implement `_parse_conflicts_with()` (array-of-strings + no
      self-reference).
- [x] 1.4 Wire both into `validate_recipe_toml` from the `[recipe]` table.
- [x] 1.5 Verify `tests/test_recipe_schema.py::RecipeTagsConflictsTests` (7) GREEN.

## Phase 2 — Detection (RED → GREEN)

- [x] 2.1 Add `TagConflict` dataclass (`tag`, `recipes` set, `severity`,
      `to_dict()`).
- [x] 2.2 Implement `check_tag_conflicts(recipes)`: group by tag, ≥2 → conflict;
      fatal if symmetric `conflicts_with`, else warning.
- [x] 2.3 Verify `tests/test_recipe_conflicts.py::TagConflictTests` (7) GREEN.

## Phase 3 — Catalog metadata

- [x] 3.1 Add `tags` to git-pr-flow, gitlab-mr-flow, trello-mcp-workflow,
      worktree-flow, session-context, tdd-flow, vault-canonical-store.
- [x] 3.2 Add `tags` to bitbucket-pr-flow.
- [x] 3.3 **CORRECTION (Design Decision 2):** remove the
      `conflicts_with = [...]` line from bitbucket-pr-flow — VCS recipes coexist
      via `[[bindings]]`.

## Phase 4 — Sync surfacing

- [x] 4.1 Add `check_tag_conflicts(catalog_dir, recipe_ids)` wrapper in
      `recipe-materialize.py`.
- [x] 4.2 **CORRECTION (Design Decision 1):** make the sync integration
      advisory — warn for both severities, never `return 1`, never skip
      materialization.

## Phase 5 — Docs

- [x] 5.1 Document `tags` and `conflicts_with` in `docs/recipe-schema.md`.

## Phase 6 — Verification

- [x] 6.1 `./tests/run.sh` (focused) GREEN including the 2 dual-provider tests.
      (16/16 targeted GREEN: 14 card + 2 dual-provider.)
- [x] 6.2 `./tests/validate.sh` (full: py_compile + bash -n + suite) GREEN.
      (`Ran 724 tests ... OK`, exit 0.)
