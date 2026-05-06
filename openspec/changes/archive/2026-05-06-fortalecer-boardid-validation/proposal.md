# Proposal: fortalecer-boardid-validation

## Why

The `trello-mcp-workflow` recipe accepts an 8-character Trello shortLink as `board_id` because the `validate-config` hook only checks field presence, not format. The `init.md` hint incorrectly points to the URL shortLink instead of the real 24-character hexadecimal board ID. Additionally, the generated `AGENTS.md` omits recipe config fields, leaving agents without a runtime reference for expected values. This change closes the validation gap, fixes the onboarding hint, and surfaces config contracts in the runtime brief.

## What Changes

1. **ConfigField validation support** — Add an optional `validation` table to `[config.*]` fields in `recipe.toml` schema, supporting `regex` (string) for format validation. **BREAKING** for any custom recipe parsers that do not tolerate unknown keys in config field tables.
2. **Enforce `validate-config` hook format checks** — Extend `recipe-materialize.py` so the `validate-config` hook evaluates `validation.regex` against merged config values and fails sync with a descriptive error on mismatch.
3. **Tighten `trello-mcp-workflow` board_id contract** — Update `catalog/recipes/trello-mcp-workflow/recipe.toml` to declare `validation.regex = "^[0-9a-fA-F]{24}$"` under `[config.board_id]`.
4. **Fix `init.md` hint** — Rewrite the `board_id` hint in `catalog/recipes/trello-mcp-workflow/init.md` to explain how to obtain the real 24-character board ID (via the Trello API or board settings) instead of the URL shortLink.
5. **Render recipe config fields in `AGENTS.md`** — Update `agents-md-render.py` to emit a per-recipe config table (field, required, type, default, validation) under the recipes section of the runtime brief.
6. **Test coverage** — Add unit tests for regex validation in `test_recipe_schema.py` and hook enforcement in `test_recipe_materialize.py`; add integration tests for AGENTS.md rendering.

## Capabilities

### New Capabilities
<!-- No new top-level capabilities; the change extends existing domains. -->

### Modified Capabilities
- `recipe-config`: Adds a requirement that config fields MAY declare `validation.regex` and that sync SHALL enforce the regex during `validate-config` hook execution.
- `recipe-schema`: Extends the `ConfigField` schema to accept an optional `validation` table with a `regex` string. The parser SHALL reject invalid `validation` structures.
- `agents-md-runtime-brief`: Adds a requirement that the generated brief SHALL include a config fields subsection for each enabled recipe, listing field name, required, type, default, and validation.

## Impact

- `lib/_internal/recipe_schema.py` — parser dataclass and TOML validation.
- `lib/_internal/recipe-materialize.py` — `validate-config` hook logic.
- `lib/_internal/agents-md-render.py` — runtime brief rendering.
- `catalog/recipes/trello-mcp-workflow/recipe.toml` — board_id validation declaration.
- `catalog/recipes/trello-mcp-workflow/init.md` — corrected onboarding hint.
- `tests/test_recipe_schema.py`, `tests/test_recipe_materialize.py` — new test scenarios.
