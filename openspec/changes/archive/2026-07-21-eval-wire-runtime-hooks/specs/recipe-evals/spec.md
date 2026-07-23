## ADDED Requirements

### Requirement: Runtime hooks wired for live scenarios

The live-eval harness SHALL wire a recipe's `[[provides.hooks]]` runtime hooks
into the fixture's native runtime channel before invoking the agent, so
scenarios exercise runtime hooks end-to-end (not only the advisory skill/brief
layer). Wiring SHALL reuse `hooks-render.py` (the same renderer `sync-agent.sh`
uses), fed by the resolved-hooks JSON from `recipe-materialize`, mapping the
eval runtime id to the platform agent id (`cursor-agent → cursor`; others
identity).

#### Scenario: Hook wired for a hook-capable runtime

- GIVEN a recipe declaring a `pre-tool-use` blocking hook
- AND the fixture is materialized for the `claude` runtime
- WHEN the harness wires runtime hooks
- THEN the fixture `.claude/settings.json` MUST contain a `PreToolUse` entry
  invoking that hook's materialized script

#### Scenario: No file-write hook for cursor

- GIVEN the same recipe
- AND the fixture is prepared for the `cursor-agent` runtime
- WHEN the harness wires runtime hooks
- THEN no file-write (`Edit|Write|MultiEdit|NotebookEdit`) hook is wired for
  cursor (the runtime exposes no pre-file-write event)

### Requirement: Hook-dependent scenario scoping

A scenario MAY declare `requires_hook = true`. The runner SHALL skip such a
scenario on any runtime that cannot receive a file-write hook (currently
`cursor-agent`), so a gate scenario asserts only where the gate can actually be
enforced.

#### Scenario: Gate scenario skipped on cursor-agent

- GIVEN a scenario with `requires_hook = true`
- AND the selected runtime is `cursor-agent`
- WHEN the runner evaluates that scenario/runtime pair
- THEN it MUST be skipped (not counted as pass or fail)

#### Scenario: Gate scenario runs on claude

- GIVEN a scenario with `requires_hook = true`
- AND the selected runtime is `claude`
- WHEN the runner evaluates that scenario/runtime pair
- THEN it MUST run with the hook wired and assert the scenario's globs
