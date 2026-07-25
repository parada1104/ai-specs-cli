## ADDED Requirements

### Requirement: Omp is a first-class runtime-hook adapter target

Sync SHALL render `[[provides.hooks]]` for the `omp` harness into
`.omp/extensions/<recipe>-<hook>.ts`, importing `@oh-my-pi/pi-coding-agent`,
registering `pi.on("tool_call", …)` for `pre-tool-use`, and returning
`{ block: true, reason }` when the script exits `2`. Documentation that lists
per-harness hook wiring SHALL include `omp` alongside `claude`, `cursor`,
`opencode`, and `pi`.

#### Scenario: Omp extension shim generated
- **GIVEN** an enabled `omp` agent and a `pre-tool-use` hook
- **WHEN** sync runs
- **THEN** sync SHALL generate `.omp/extensions/<recipe>-<hook>.ts` that
  registers `pi.on("tool_call", …)`, spawns the script, and returns
  `{ block: true }` when the script exits `2`

### Requirement: OpenCode tool-name matcher is case-insensitive

Generated OpenCode plugins SHALL match tool names case-insensitively, consistent
with the pi and omp adapters (tool ids may arrive lowercase while recipe
matchers use Claude-style names such as `Edit`/`Write`).

#### Scenario: OpenCode matcher uses the i flag
- **GIVEN** an enabled `opencode` agent and a `pre-tool-use` hook with a
  non-empty matcher
- **WHEN** sync generates the OpenCode plugin
- **THEN** the plugin SHALL construct the matcher as
  `new RegExp(\`^(?:${MATCHER})$\`, "i")`

### Requirement: Documented pre-tool-use reach is honest about process boundaries

Product documentation for runtime hooks SHALL state that pi and omp
`tool_call` handlers apply to tool calls in **that agent process**, and SHALL
NOT claim they cover subprocess/subagent-delegated tool calls unless that
coverage is verified. OpenCode's existing primary-agent-only caveat
(subagent/MCP) SHALL remain. Guidance SHALL state that worktree/plan-build
gates must not be the sole guard for delegation-heavy workflows on
opencode/pi/omp.

#### Scenario: Status table distinguishes process vs cross-process coverage
- **GIVEN** a reader of `docs/runtime-hooks.md`
- **WHEN** they consult the pre-tool-use status table
- **THEN** pi and omp SHALL be described as blocking in the current agent
  process (not as unconditional "all tool calls")
- **AND** omp SHALL appear as its own row
