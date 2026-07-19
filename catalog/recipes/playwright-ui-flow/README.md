# Playwright UI Flow

Base recipe for Playwright-backed **UI tests and smokes**. Ships the canonical
`ui-browser-testing` discipline plus a thin CLI adapter. For interactive
exploration, enable the add-on recipe [`playwright-mcp`](../playwright-mcp/).

## What it provides

- **Skill `ui-browser-testing`** — when to run suites/smokes, CLI vs MCP
  precedence, evidence policy; complements `tdd-flow`.
- **Skill `playwright-cli`** — how to invoke configured Playwright commands,
  install browsers, codegen.
- **Command `/ui-smoke`** — run the configured smoke/suite command and record
  evidence.
- **Doc** — this README, materialized to
  `ai-specs/recipes/playwright-ui-flow/README.md`.

## Capability

- `ui-browser-testing` — agent-operable browser UI verification.

## Enable

```toml
[recipes.playwright-ui-flow]
enabled = true

[recipes.playwright-ui-flow.config]
ui_test_command = "npx playwright test"
ui_smoke_command = "npx playwright test --grep @smoke"
```

Then run `ai-specs sync`. Optional hybrid: also enable `playwright-mcp`.

## Config

| Key | Type | Required | Default | Description |
| --- | ---- | -------- | ------- | ----------- |
| `ui_test_command` | string | no | _(unset)_ | Full UI suite command. |
| `ui_smoke_command` | string | no | _(unset)_ | Fast smoke subset (e.g. `--grep @smoke`). |
| `playwright_config` | string | no | _(unset)_ | Path to `playwright.config.*` when non-standard. |

Suggested smoke convention: tag tests with `@smoke` and filter via
`ui_smoke_command`.

## Hybrid precedence

| Job | Surface |
| --- | ------- |
| Suite / smoke | CLI (this recipe) |
| Explore / debug / locators | MCP (`playwright-mcp`) when enabled |
