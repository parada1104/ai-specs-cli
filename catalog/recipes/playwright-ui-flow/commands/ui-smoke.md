# ui-smoke

Run the project's configured UI smoke (or suite) command and record evidence.

## Steps

1. Read `[recipes.playwright-ui-flow.config]` for `ui_smoke_command`, falling
   back to `ui_test_command` if smoke is unset.
2. If both are unset, discover the project's Playwright script and propose
   config values — do not invent a silent default.
3. Run the chosen command from the project root (or documented cwd).
4. Record the command and pass/fail result as evidence (change verify notes,
   PR body, or tracker update).
5. For policy (when this is required, hybrid vs MCP), follow the
   `ui-browser-testing` discipline skill.
