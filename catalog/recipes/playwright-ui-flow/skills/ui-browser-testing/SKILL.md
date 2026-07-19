---
name: ui-browser-testing
description: >
  Canonical discipline for browser UI verification: when to run Playwright
  suites and smokes, how CLI vs MCP surfaces are chosen, and how to record
  evidence before merge. Complements tdd-flow for non-UI tests.
license: MIT
metadata:
  author: ai-specs
  version: "1.0"
  scope: runtime
  auto_invoke:
    - "Running or writing Playwright UI tests or smokes"
    - "Verifying UI behavior before merge or PR"
    - "Choosing between Playwright CLI and browser MCP"
---

# UI Browser Testing Discipline

This is the **canonical discipline** skill for the `ui-browser-testing`
capability. Tool-specific how-to lives in thin adapters:

- `playwright-cli` — run/author suites via CLI
- `playwright-mcp` — exploratory navigation, debug, locators (when enabled)

Defer *how* on a surface to those adapters. Keep *when / whether / evidence*
here.

## Relationship to tdd-flow

- **`tdd-flow` (`test-runner`)** — unit/integration RED/GREEN against a generic
  `test_command`.
- **This skill (`ui-browser-testing`)** — browser UI suites, smokes, and
  optional interactive exploration.

Both may be enabled. Do not treat UI smokes as a substitute for unit TDD, and
do not skip UI verification when the change touches user-facing UI.

## Configuration

Read from `[recipes.playwright-ui-flow.config]` in `ai-specs/ai-specs.toml`:

| Key | Required | Description |
|---|---|---|
| `ui_test_command` | no | Full UI suite (e.g. `npx playwright test`) |
| `ui_smoke_command` | no | Fast smoke subset (e.g. `npx playwright test --grep @smoke`) |
| `playwright_config` | no | Path to `playwright.config.*` if non-standard |

If smoke/suite commands are unset, discover the project's Playwright scripts
(`package.json`, config files), propose values for the human to accept, and do
**not** invent a permanent default without recording it in config.

## Hybrid surface precedence

| Job | Preferred surface |
|---|---|
| Run UI suite / smoke | CLI / configured command |
| Author or extend UI tests in-repo | CLI (+ repo test files); MCP optional for locator discovery |
| Exploratory UI navigation / debug | MCP (`playwright-mcp` recipe) when enabled |
| Generate locators while writing tests | MCP when enabled; otherwise CLI snapshot/codegen |

When only one surface is enabled, degrade to that surface. Never require MCP
for suite/smoke verification. Never use MCP tool calls as a substitute for the
configured UI smoke/suite command when claiming merge-ready UI verification.

## Evidence policy

Before treating UI verification as done:

1. Run `ui_smoke_command` if set, otherwise `ui_test_command` (or the agreed
   discovered command).
2. Record the **command** and **pass/fail** observation in the change's
   verify/apply artifacts, PR body, or tracker update.
3. Never claim a UI check passed unless that command was actually run and
   observed to pass.

## When to invoke adapters

- Need to run or debug a suite command, install browsers, or use codegen →
  load `playwright-cli`.
- Need interactive page exploration or locator discovery and
  `playwright-mcp` is enabled → load `playwright-mcp`.
