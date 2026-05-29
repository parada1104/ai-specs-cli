---
name: tdd-flow
description: >
  Tool-agnostic red-green-refactor discipline for any project. Write a failing
  test first (RED), make it pass with the minimum change (GREEN), then refactor.
  Drive every cycle through the project's configured test command and record
  RED/GREEN evidence before merge.
license: MIT
metadata:
  author: ai-specs
  version: "1.0"
  scope: runtime
  auto_invoke:
    - "Writing or changing code that should be tested"
    - "Verifying a change before merge"
---

# TDD Flow

Red-green-refactor is the default discipline for any change that affects testable
behavior. The cycle is tool-agnostic: it works the same whether the project tests
with a shell script, a language test runner, or a build target. The actual test
command is **configuration**, never assumed by this skill.

## The Test Command Is Config

This recipe does not know or hardcode how your project runs tests. The command
comes from your project manifest:

```toml
[recipes.tdd-flow.config]
test_command = "<your project's test command>"
```

Whenever this skill says "run the test command", run exactly the value of
`test_command` from `[recipes.tdd-flow.config]`. If it is not set, ask the user
or read the project's runtime brief / docs to discover how this project runs its
tests, and propose setting `test_command` so the cycle is reproducible. Do not
invent or assume a command.

## The Cycle

1. **RED — write a failing test first.**
   - Add or extend a test that expresses the intended behavior before touching
     production code.
   - Run the configured `test_command`. Confirm the new test fails for the right
     reason (asserts the missing behavior, not a typo or import error).
   - A change that cannot show a meaningful RED is a signal the test is not
     pinning real behavior.

2. **GREEN — make it pass minimally.**
   - Write the smallest production change that makes the failing test pass.
   - Run the configured `test_command` again. Confirm the previously failing
     test now passes and no other tests regressed.
   - Resist adding behavior the tests do not yet require.

3. **REFACTOR — clean up under green.**
   - With the suite green, improve names, structure, and duplication.
   - Re-run the configured `test_command` after each meaningful refactor to keep
     the suite green throughout.

Repeat the cycle per behavior, in small increments. Prefer many tight cycles
over one large batch.

## Minimum Test Layers

Use the smallest relevant set of these generic layers for a change:

- **Unit** — pure logic: parsing, normalization, calculation, validation,
  rendering. Fast and isolated; the bulk of RED/GREEN cycles happen here.
- **Integration** — components working together: a command end to end, a module
  against a real dependency or temp fixtures, generated artifacts checked on
  disk.
- **Smoke** — a broad, shallow run that confirms the system is not obviously
  broken before a change is considered ready (often the project's full suite or
  validation target).

A change usually needs unit coverage of the new logic plus at least one
integration or smoke layer to trust it.

## Evidence Policy

Every change should leave clear evidence of the cycle:

- Record the **RED** result (the failing run and which test failed) and the
  **GREEN** result (the passing run) before merge.
- Capture the final smoke / full-suite run.
- In structured changes, record this in the change's apply/verify artifacts. In
  ad-hoc changes, record it in the commit message, PR description, or task
  update.
- Never claim a test passed unless the configured `test_command` was actually run
  and observed to pass.
