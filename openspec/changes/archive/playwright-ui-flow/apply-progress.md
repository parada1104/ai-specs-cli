# Apply progress: playwright-ui-flow

## Phase 1 RED

- T1.1–T1.8: `tests/test_playwright_ui_flow_recipes.py` written.
- RED: 10 errors + 2 failures (missing recipes/docs).

## Phase 2 GREEN

- T2.1: `catalog/recipes/playwright-ui-flow/` (recipe.toml, discipline + CLI skills, ui-smoke, init, README)
- T2.2: `catalog/recipes/playwright-mcp/` (recipe.toml, MCP adapter, init, README, MCP preset)
- T2.3: Adapters defer to `ui-browser-testing`; MCP documents base augmentation
- T2.4: `docs/capabilities.md` + `docs/recipes-catalog.md` (+ MCP_RECIPES test map)
- T2.5: focused tests OK; `./tests/validate.sh` → 988 tests OK

Note: base tags = `["ui-testing"]` only (dropped `quality` to avoid WARN overlap with `tdd-flow`).
