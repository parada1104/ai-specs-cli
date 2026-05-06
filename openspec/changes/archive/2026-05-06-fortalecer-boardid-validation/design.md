# Design: fortalecer-boardid-validation

## Context

The `trello-mcp-workflow` recipe declares `board_id` as a required string, but nothing prevents a user from entering the 8-character Trello shortLink visible in the browser URL. The `validate-config` hook in `recipe-materialize.py` only checks presence (`required`), not format. The `init.md` hint incorrectly tells the user to look at the URL for the board ID, reinforcing the shortLink mistake. Finally, `agents-md-render.py` does not surface recipe config fields in `AGENTS.md`, so agents have no runtime reference for what values are expected or validated.

## Goals / Non-Goals

**Goals:**
- Extend `ConfigField` to support an optional `validation` table with a `regex` string.
- Extend `recipe-materialize.py` so the `validate-config` hook evaluates `regex` against merged config values.
- Update `trello-mcp-workflow/recipe.toml` to declare `validation.regex` for `board_id`.
- Fix `trello-mcp-workflow/init.md` hint to describe obtaining the real 24-char hex board ID.
- Update `agents-md-render.py` to emit a per-recipe config fields table in `AGENTS.md`.
- Provide unit and integration test coverage for all of the above.

**Non-Goals:**
- General-purpose validation framework (min/max, enums, custom functions). Regex is sufficient for the immediate need and keeps the schema simple.
- Changing the Trello MCP server or its API contract.
- Auto-correcting or resolving shortLinks to board IDs.
- Modifying `recipe-init.py` or the CLI `recipe init` interactive flow beyond the static `init.md` text.

## Decisions

### 1. ConfigField `validation` attribute shape
- **Decision:** Add an optional `validation: dict | None` attribute to `ConfigField`. When present, it contains `{"regex": "..."}`.
- **Rationale:** A dict leaves room for future keys (e.g., `min`, `max`) without a breaking schema change. Using a single `validation` table in TOML mirrors common config patterns and keeps the top-level field table uncluttered.
- **Alternative considered:** Flattening `regex` directly into the field table (`regex = "..."`). Rejected because it pollutes the field namespace and makes future expansion harder.

### 2. Regex engine and matching semantics
- **Decision:** Use Python `re.fullmatch(pattern, value)` in the `validate-config` hook.
- **Rationale:** `fullmatch` is the intuitive semantics for a "format" constraint; `re.match` would allow trailing characters. Python’s built-in `re` module is already available and requires no new dependencies.
- **Alternative considered:** Using `re.match` with explicit `^...$` anchors. Rejected because it is error-prone for recipe authors; requiring anchors in the pattern is fragile.

### 3. Strictness of validation table parsing
- **Decision:** Reject unknown keys inside `[config.*.validation]` with a clear `RecipeValidationError`.
- **Rationale:** Strict parsing catches typos early (e.g., `validation.patterm`). Since the validation surface is small today, strictness is cheap and prevents silent misconfiguration.
- **Alternative considered:** Ignore unknown keys. Rejected because it weakens the contract and defers errors to runtime.

### 4. AGENTS.md rendering style
- **Decision:** Render config fields as a Markdown table under each enabled recipe in the existing "Active Recipes" section.
- **Rationale:** Tables are compact and agent-readable. Placing them under the recipe bullet keeps context local.
- **Alternative considered:** A separate "Recipe Config" top-level section. Rejected because it fragments the brief and requires readers to cross-reference recipe IDs.

### 5. Backward compatibility of `_parse_config`
- **Decision:** Continue to allow extra keys in the config field table itself (as today), but validate the `validation` sub-table strictly when present.
- **Rationale:** Existing recipes may have forward-compatible keys we do not yet recognize; we only need to be strict about the nested `validation` structure because it is new.

## Risks / Trade-offs

- **[Risk]** A recipe author writes an invalid regex (e.g., unescaped backslash in TOML).  
  **→ Mitigation:** The parser validates that `regex` is a string; sync-time `re.compile` will raise a clear `re.error` that we catch and surface as a recipe validation error.

- **[Risk]** `agents-md-render.py` grows large if recipes declare many config fields.  
  **→ Mitigation:** Only enabled recipes are rendered, and config schemas are typically small (<10 fields). If growth becomes a problem, a summary mode can be added later.

- **[Risk]** Existing `trello-mcp-workflow` users have invalid `board_id` values in their manifests.  
  **→ Mitigation:** This is the intended behavior; sync will now fail with a clear message, forcing them to correct the value. The change is flagged as breaking in the proposal.

## Migration Plan

1. Update `recipe_schema.py` parser and dataclass.
2. Update `recipe-materialize.py` hook logic.
3. Update `agents-md-render.py` to emit config tables.
4. Update `trello-mcp-workflow/recipe.toml` and `init.md`.
5. Run `./tests/run.sh` and `./tests/validate.sh`.
6. Commit; do not merge to `development` without human review.

No database or external state migration is required.

## Open Questions

None at design time.
