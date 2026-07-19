---
name: playwright-mcp
description: >
  Thin MCP adapter for exploratory Playwright browser automation. Defer
  suite/smoke/evidence policy to the ui-browser-testing discipline skill;
  this skill covers when and how to use @playwright/mcp tools.
license: MIT
metadata:
  author: ai-specs
  version: "1.0"
  scope: runtime
  auto_invoke:
    - "Exploring a UI with Playwright MCP"
    - "Generating locators or debugging a page interactively"
---

# Playwright MCP Adapter

**Defer to the `ui-browser-testing` discipline skill for when/whether to run
UI suites/smokes and how to record evidence.** This skill covers *how* on the
MCP surface.

## Augments playwright-ui-flow

This recipe is an **add-on** to `playwright-ui-flow`. Enable both for hybrid
behavior (CLI suites + MCP explore). If the base recipe is not enabled,
limit yourself to explore/debug/locator help and tell the user that full
suite/smoke discipline lives in `playwright-ui-flow`.

## When to use MCP

Use Playwright MCP tools for:

- Interactive navigation and page exploration
- Debugging a failing UI flow step-by-step
- Locator discovery while authoring tests

Do **not** use MCP as the merge gate for UI verification when a configured
CLI smoke/suite command exists — run that command instead (see discipline).

## Tooling notes

- Server id: `playwright` (from recipe MCP preset / project `[mcp.playwright]`).
- Typical tools: navigate, snapshot, click/type, and locator helpers exposed
  by `@playwright/mcp` (exact names depend on the server version).
- Prefer accessibility snapshots over screenshot-only workflows.
- Close or leave the browser session tidy when finished; do not leave headed
  windows open without need.

## Overrides

Browser/headless/base-URL tuning belongs in the project manifest under
`[mcp.playwright]` (args/env), not in recipe config. Example:

```toml
[mcp.playwright]
args = ["-y", "@playwright/mcp@latest", "--browser", "chromium"]
```
