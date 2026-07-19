# Playwright MCP

Add-on recipe that ships the **`@playwright/mcp`** preset and a thin adapter
skill for exploratory UI navigation, debugging, and locator discovery.

**Augments [`playwright-ui-flow`](../playwright-ui-flow/).** Enable both for
hybrid behavior. Enabling this recipe alone is explore-only — suite/smoke
discipline lives in the base recipe.

## What it provides

- **Skill `playwright-mcp`** — when/how to use Playwright MCP tools (defers
  policy to `ui-browser-testing`).
- **MCP preset `playwright`** — `npx -y @playwright/mcp@latest --headless`.
- **Doc** — this README → `ai-specs/recipes/playwright-mcp/README.md`.

## Enable (hybrid)

```toml
[recipes.playwright-ui-flow]
enabled = true

[recipes.playwright-mcp]
enabled = true

# Optional override (manifest wins):
# [mcp.playwright]
# args = ["-y", "@playwright/mcp@latest", "--browser", "chromium"]
```

Then `ai-specs sync`. Ensure browsers are installed (`npx playwright install`)
when first using the MCP server.

## Config

No recipe config keys in v1. Tune browser/headless via `[mcp.playwright]` in
the project manifest.
