# /tdd — Drive a red-green-refactor cycle

Operationalize the `tdd-flow` skill for the current change. The test command is
configuration: use the value of `test_command` from
`[recipes.tdd-flow.config]` in `ai-specs.toml`. Do not assume or hardcode any
command.

## Steps

1. **Resolve the test command.**
   Read `test_command` from `[recipes.tdd-flow.config]`. If it is missing, stop
   and ask the user how this project runs tests (or read the runtime brief),
   then propose setting `test_command`. Do not guess.

2. **RED — write the failing test first.**
   Add or extend a test for the intended behavior before changing production
   code. Run the configured `test_command`. Confirm it fails for the right
   reason and capture the failing output.

3. **GREEN — implement minimally.**
   Make the smallest production change that turns the failing test green. Run the
   configured `test_command` again. Confirm the target test passes and nothing
   else regressed.

4. **Re-run until green.**
   If still red, iterate on the implementation (not the test, unless the test was
   wrong) and re-run the configured `test_command` until green.

5. **REFACTOR.**
   With the suite green, clean up structure and duplication, re-running the
   configured `test_command` after each meaningful change.

6. **Record evidence.**
   Note the RED and GREEN runs (and the final smoke / full-suite run) in the
   commit, PR, or change artifacts before merge.

Work in small increments — one behavior per cycle.
