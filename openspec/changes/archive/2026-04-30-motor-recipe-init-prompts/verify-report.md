# Verification Report: `motor-recipe-init-prompts`

## Summary

| Dimension | Status |
|---|---|
| Completeness | PASS: 46/46 tasks complete; apply instructions report `all_done` |
| Correctness | PASS: 11/11 requirements implemented with focused test coverage |
| Coherence | PASS: implementation follows the read-only/reviewable init design |
| Full Validation | WARNING: full suite still has 3 pre-existing sync-related errors |

Final assessment: PASS for the #66 implementation, with a non-#66 baseline warning. Do not archive until the baseline sync failures are resolved or explicitly accepted.

## Completeness

- `openspec/changes/motor-recipe-init-prompts/tasks.md`: 46/46 checkboxes complete.
- `openspec instructions apply --change motor-recipe-init-prompts --json`: state `all_done`, progress 46/46.
- `openspec validate motor-recipe-init-prompts --strict`: PASS.
- `apply-progress.md` records RED/GREEN evidence and the full-suite baseline distinction.

## Requirement Mapping

### `recipe-schema`

- Optional `[init]` declaration implemented in `lib/_internal/recipe_schema.py:79` and `lib/_internal/recipe_schema.py:244`.
- Prompt path validation covers relative-only paths, directory traversal, missing files, and directory targets in `lib/_internal/recipe_schema.py:280`.
- Recipes without `[init]` remain valid through `init: InitWorkflow | None = None` and parser default behavior.
- Coverage: `tests/test_recipe_read.py:198`, `tests/test_recipe_read.py:233`, `tests/test_recipe_read.py:242`, `tests/test_recipe_read.py:249`, `tests/test_recipe_read.py:256`, `tests/test_recipe_read.py:263`, `tests/test_recipe_read.py:270`, `tests/test_recipe_read.py:277`.

### `recipe-cli`

- `ai-specs recipe init <id> [path]` dispatch implemented in `lib/recipe.sh:31` and wrapper `lib/recipe-init.sh:13`.
- Helper entrypoint implemented in `lib/_internal/recipe-init.py:180` and `lib/_internal/recipe-init.py:253`.
- Init brief includes recipe identity, install state, prompt content, manifest context, config guidance, MCP context, template preview, and next actions in `lib/_internal/recipe-init.py:180`.
- Coverage: `tests/test_recipe_init.py:67`, `tests/test_recipe_init.py:76`, `tests/test_recipe_init.py:83`, `tests/test_recipe_init.py:89`, `tests/test_recipe_init.py:95`, `tests/test_recipe_init.py:102`, `tests/test_recipe_init.py:137`.

### `recipe-config` and `recipe-manifest-contract`

- Existing recipe declarations and config keys are detected without duplicate proposals in `lib/_internal/recipe-init.py:96`.
- Missing required config fields are presented as setup targets without bypassing sync validation in `lib/_internal/recipe-init.py:117`.
- Unknown config keys are reported explicitly in `lib/_internal/recipe-init.py:127`.
- Coverage: `tests/test_recipe_init.py:76`, `tests/test_recipe_init.py:83`, `tests/test_recipe_init.py:123`.

### `recipe-sync-materialization`

- Init remains read-only and does not run sync or materialize primitives in `lib/_internal/recipe-init.py:244`.
- Template/override preview reports target state and review action without copying files in `lib/_internal/recipe-init.py:166`.
- Coverage: `tests/test_recipe_init.py:130`, `tests/test_recipe_init.py:148`.

### `mcp-preset-merge`

- MCP discovery reports configured and missing servers plus recipe presets in `lib/_internal/recipe-init.py:139`.
- Redaction preserves env references and redacts secret-like literal values through `lib/_internal/recipe-init.py:40`.
- The brief states that project manifest values take precedence during sync-time MCP merge in `lib/_internal/recipe-init.py:143`.
- Coverage: `tests/test_recipe_init.py:111`.

## Design Coherence

- Design decision “Init metadata lives in `recipe_schema.py`” followed: parser/dataclasses updated in `recipe_schema.py`.
- Design decision “`recipe-read.py` emits init metadata, not execution” followed: JSON exposes `init` in `lib/_internal/recipe-read.py:50`; execution lives in `recipe-init.py`.
- Design decision “Default init is read-only and reviewable” followed: helper prints a brief and performs no writes.
- Design decision “Init preview does not materialize recipe primitives” followed: sync/materialization functions were not invoked by the helper.
- Documentation updated in `docs/recipe-schema.md:168`.

## Verification Evidence

### Passing Checks

- `python3 -m unittest tests.test_recipe_read tests.test_recipe_init`: PASS.
- `python3 -m unittest tests.test_recipe_schema tests.test_recipe_read tests.test_recipe_init tests.test_recipe_add tests.test_recipe_list tests.test_recipe_materialize tests.test_toml_read`: `Ran 91 tests in 0.473s` / `OK`.
- `python3 -m py_compile lib/_internal/*.py tests/*.py`: PASS.
- `bash -n lib/*.sh bin/ai-specs tests/*.sh`: PASS.
- `openspec validate motor-recipe-init-prompts --strict`: PASS.

### Baseline Warning

- `./tests/run.sh`: exit 1, `Ran 225 tests in 50.064s`, `FAILED (errors=3)`.
- `./tests/validate.sh`: exit 1 at the unittest layer with the same 3 sync-related failures.
- Failing tests:
  - `test_context_precedence_skill.ContextPrecedenceSkillTests.test_sync_renders_agents_reference_when_bundled_skill_present`
  - `test_sync_pipeline.SyncPipelineTests.test_sync_accepts_minimal_manifest_with_omitted_sections`
  - `test_sync_pipeline.SyncPipelineTests.test_sync_preserves_runtime_brief_marker_in_agents_md`
- These same three failures were recorded before applying #66, and the three-test subset passes when run directly. They are tracked as baseline/order-related sync failures, not new recipe-init failures.

## Issues

### Critical

- None for the #66 implementation.

### Warnings

- Full-suite validation is not clean because of the pre-existing sync-related baseline errors above. Recommendation: resolve or explicitly waive those baseline failures before archive/merge.

### Suggestions

- Future enhancement: add a write/apply flag only under a separate spec if the project wants `recipe init` to apply reviewed TOML changes automatically.

## Final Assessment

The implementation satisfies the OpenSpec artifacts for `motor-recipe-init-prompts` and is ready for review. It is not ready for archive under a strict clean-suite policy until the unrelated baseline sync failures are handled.
