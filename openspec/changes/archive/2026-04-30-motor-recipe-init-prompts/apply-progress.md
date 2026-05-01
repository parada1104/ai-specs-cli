# Apply Progress: `motor-recipe-init-prompts`

## Scope

Implemented `ai-specs recipe init <id> [path]` as a read-only recipe initialization brief, plus optional `[init]` parsing in `recipe.toml`, recipe-read JSON exposure, CLI dispatch, docs, and tests.

## RED Evidence

### Recipe Init Focused Tests

- Command: `python3 -m unittest tests.test_recipe_read tests.test_recipe_init`
- Exit: `1`
- Result: expected RED before implementation.
- Summary: 23 tests ran with failures/errors for missing `recipe-init.py`, absent `Recipe.init`, missing `init` JSON output, and missing validation for `[init]` prompt/fields/path rules.

## GREEN Evidence

### Focused Recipe Init Tests

- Command: `python3 -m unittest tests.test_recipe_read tests.test_recipe_init`
- Exit: `0`
- Result: PASS.

### Recipe-Related Regression Tests

- Command: `python3 -m unittest tests.test_recipe_schema tests.test_recipe_read tests.test_recipe_init tests.test_recipe_add tests.test_recipe_list tests.test_recipe_materialize tests.test_toml_read`
- Exit: `0`
- Result: `Ran 91 tests in 0.473s` / `OK`.

### OpenSpec Validation

- Command: `openspec validate motor-recipe-init-prompts --strict`
- Exit: `0`
- Result: `Change 'motor-recipe-init-prompts' is valid`.

### Syntax Validation

- Command: `python3 -m py_compile lib/_internal/*.py tests/*.py`
- Exit: `0`
- Result: PASS.
- Command: `bash -n lib/*.sh bin/ai-specs tests/*.sh`
- Exit: `0`
- Result: PASS.

## Full Suite Baseline Distinction

### `./tests/run.sh`

- Command: `./tests/run.sh`
- Exit: `1`
- Result: `Ran 225 tests in 50.064s` / `FAILED (errors=3)`.
- Failing tests:
  - `test_context_precedence_skill.ContextPrecedenceSkillTests.test_sync_renders_agents_reference_when_bundled_skill_present`
  - `test_sync_pipeline.SyncPipelineTests.test_sync_accepts_minimal_manifest_with_omitted_sections`
  - `test_sync_pipeline.SyncPipelineTests.test_sync_preserves_runtime_brief_marker_in_agents_md`
- Assessment: same three sync-related failures observed before applying #66. The three-test subset passes when rerun directly, so this remains a full-suite baseline/order issue rather than a new recipe-init failure.

### `./tests/validate.sh`

- Command: `./tests/validate.sh`
- Exit: `1`
- Result: fails at the unittest layer with the same three sync-related errors above.
- Syntax layers pass independently.

## Implementation Notes

- New helper: `lib/_internal/recipe-init.py`.
- New wrapper: `lib/recipe-init.sh`.
- Updated dispatcher/help: `lib/recipe.sh`, `bin/ai-specs`.
- Updated parser/JSON: `lib/_internal/recipe_schema.py`, `lib/_internal/recipe-read.py`.
- Updated docs: `docs/recipe-schema.md`.
- Tests added/updated: `tests/test_recipe_init.py`, `tests/test_recipe_read.py`.

## Status

All implementation tasks in `tasks.md` are complete. Full-suite verification is blocked only by the pre-existing sync-related baseline failures documented above.
