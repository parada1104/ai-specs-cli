# Tasks: fortalecer-boardid-validation

## 1. Schema & Parser

- [x] 1.1 Extend `ConfigField` dataclass in `lib/_internal/recipe_schema.py` with optional `validation` dict attribute
- [x] 1.2 Update `_parse_config` in `lib/_internal/recipe_schema.py` to parse `validation` table and reject unknown keys inside it
- [x] 1.3 Add unit tests in `tests/test_recipe_schema.py` for config field with validation, without validation, and malformed validation

## 2. Sync Hook Validation

- [ ] 2.1 Update `execute_hooks` in `lib/_internal/recipe-materialize.py` so `validate-config` evaluates `validation.regex` via `re.fullmatch`
- [ ] 2.2 Ensure regex validation failure emits a clear error naming the field and pattern
- [ ] 2.3 Add unit tests in `tests/test_recipe_materialize.py` for regex pass, regex fail, and missing validation

## 3. AGENTS.md Rendering

- [ ] 3.1 Add `render_recipe_config_table` helper in `lib/_internal/agents-md-render.py` to format a markdown table from config schema fields
- [ ] 3.2 Integrate config table rendering into `render_recipes_section` for enabled recipes with non-empty config schemas
- [ ] 3.3 Add integration tests for `agents-md-render.py` verifying config fields appear in generated AGENTS.md

## 4. Recipe Assets (trello-mcp-workflow)

- [ ] 4.1 Update `catalog/recipes/trello-mcp-workflow/recipe.toml` to add `validation.regex = "^[0-9a-fA-F]{24}$"` under `[config.board_id]`
- [ ] 4.2 Rewrite `catalog/recipes/trello-mcp-workflow/init.md` board_id hint to explain obtaining the real 24-char hex board ID

## 5. End-to-End & Regression

- [ ] 5.1 Run `./tests/run.sh` and confirm all existing tests still pass
- [ ] 5.2 Run `./tests/validate.sh` and confirm py_compile / bash -n checks pass
- [ ] 5.3 Perform a manual dry-run of `ai-specs sync` in a test project with `trello-mcp-workflow` enabled to verify board_id validation

## 6. Documentation & Artifact Closure

- [ ] 6.1 Review all spec delta files for consistency with implementation
- [ ] 6.2 Update `apply-progress.md` as tasks are completed during the apply phase
