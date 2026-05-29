# TDD Flow

Foundational, tool-agnostic recipe that brings red-green-refactor discipline to
any project. The discipline lives in the skill; the **test command is
configuration**, so this recipe never hardcodes how your project runs tests.

## What it provides

- **Skill `tdd-flow`** (bundled) — the red-green-refactor discipline: write a
  failing test (RED), make it pass minimally (GREEN), refactor; generic minimum
  test layers (unit, integration, smoke); and an evidence policy (record
  RED/GREEN before merge).
- **Command `/tdd`** — a thin agent-facing command that runs one cycle: write the
  failing test, run the configured test command, implement, and re-run until
  green.
- **Doc** — this README, materialized to
  `ai-specs/recipes/tdd-flow/README.md`.

## Capability

- `test-runner` — disciplined test-driven execution against a configurable test
  command.

## Enable

In your project's `ai-specs.toml`:

```toml
[recipes.tdd-flow]
enabled = true
version = "1.0.0"

[recipes.tdd-flow.config]
test_command = "<your project's test command>"
```

Then run `ai-specs sync` to materialize the skill, command, and docs.

## Config

| Key | Type | Required | Default | Description |
| --- | ---- | -------- | ------- | ----------- |
| `test_command` | string | no | _(none)_ | The command that runs this project's tests. Project-specific — there is no default. The skill and `/tdd` command run exactly this value for every RED/GREEN step. |

If `test_command` is not set, the skill and command will ask how the project runs
tests rather than guessing.
