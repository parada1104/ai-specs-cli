---
name: playwright-cli
description: >
  Thin CLI adapter for Playwright UI suites and smokes. Defer when/whether
  and evidence policy to the ui-browser-testing discipline skill; this skill
  covers how to invoke CLI/test commands and local Playwright tooling.
license: MIT
metadata:
  author: ai-specs
  version: "1.0"
  scope: runtime
  auto_invoke:
    - "Running npx playwright test or UI smoke commands"
    - "Installing Playwright browsers or using playwright codegen"
---

# Playwright CLI Adapter

**Defer to the `ui-browser-testing` discipline skill for when/whether to run
UI verification and how to record evidence.** This skill covers *how* on the
CLI surface.

## Config commands

From `[recipes.playwright-ui-flow.config]`:

- Prefer `ui_smoke_command` for pre-merge smoke.
- Use `ui_test_command` for the full suite.
- If unset, discover (e.g. `package.json` scripts named `test:e2e` /
  `test:ui` / `playwright`) and propose config — do not hardcode forever.

Run exactly the configured string in the project root (or documented cwd).

## Common CLI operations

```bash
# Suite / smoke (prefer configured commands above)
npx playwright test
npx playwright test --grep @smoke

# Browsers (once per machine/CI image)
npx playwright install

# Authoring helpers
npx playwright codegen <url>
npx playwright show-report
```

If `playwright_config` is set, pass it through (`--config <path>`) when the
configured command does not already include it.

## Pitfalls

- Prefer project-local Playwright (`npx` / package scripts) over a global
  binary so versions match the repo.
- Headless is normal for suite/smoke; headed/debug is for local investigation.
- Do not replace a failing suite run with ad-hoc MCP clicks when claiming
  verification — fix tests or record the failure.
