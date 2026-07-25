# runtime-hook-distribution

## Purpose

Define how `ai-specs sync` renders recipe-declared `[[provides.hooks]]` into each
enabled harness's native agent-runtime hook format, from a single script the
recipe author writes once.

## Non-Goals

- Sync-time hooks (`[[hooks]]` / `on-sync`) — a separate, unrelated capability.
- Per-harness hand-authored hook code in a recipe (v1 distributes one portable
  script; native code hooks are a future escape hatch).
- Harnesses `ai-specs` does not already target.
- Three-way merge of user-hand-edited hook config beyond the managed block.

## Requirements

### Requirement: Abstract event vocabulary mapped per harness

The product SHALL define a fixed set of abstract event names and a mapping from
each to the native event of every supported harness. The v1 set SHALL be
`pre-tool-use`, `post-tool-use`, `session-start`, and `stop`. The mapping SHALL
be the single source of truth used by every renderer.

#### Scenario: Known event maps to native names
- **GIVEN** a hook with `event = "pre-tool-use"`
- **WHEN** sync renders for an enabled harness that supports it
- **THEN** the rendered wiring SHALL use that harness's native event name (e.g. `PreToolUse` for Claude, `tool.execute.before` for OpenCode)

#### Scenario: Unsupported (event, harness) pair
- **GIVEN** a hook whose `event` has no native mapping for a given enabled harness
- **WHEN** sync renders for that harness
- **THEN** sync SHALL emit a warning naming the recipe, hook id, event, and harness
- **AND** SHALL skip that hook for that harness without emitting broken wiring
- **AND** SHALL continue rendering the hook for the harnesses that do support it

### Requirement: Normalized script contract

A hook script SHALL receive a normalized JSON event on stdin and communicate its
decision through its exit code: `0` allows the action, `2` blocks it (stderr is
surfaced to the agent). Any other exit code SHALL be treated as a non-blocking
error (fail-open). The normalized event SHALL include at least `event` (the
abstract name), `tool_name`, and `tool_input` for tool events.

#### Scenario: Script blocks an action
- **GIVEN** a `blocking` hook whose script exits `2` for a given event
- **WHEN** the agent triggers that event in any harness
- **THEN** the action SHALL be blocked and the script's stderr SHALL be surfaced

#### Scenario: Script allows an action
- **GIVEN** a hook whose script exits `0`
- **WHEN** the event fires
- **THEN** the action SHALL proceed

#### Scenario: Script errors fail open
- **GIVEN** a hook whose script exits with a code other than `0` or `2`
- **WHEN** the event fires
- **THEN** the action SHALL proceed (the guard never wedges work)

### Requirement: Script materialization

The hook script SHALL be materialized to a tracked, harness-neutral path under
the project (e.g. `ai-specs/recipes/<recipe-id>/hooks/<script>`) so every
harness's wiring references one copy.

#### Scenario: Script materialized once
- **GIVEN** a recipe declaring a hook with `script = "hooks/x.sh"`
- **WHEN** sync runs
- **THEN** the script SHALL be materialized to the harness-neutral recipe path
- **AND** SHALL be made executable

### Requirement: Direct rendering for the exit-code-native harness (Claude)

Claude Code natively honors the normalized contract (stdin JSON + `exit 2` →
block), so sync SHALL wire the materialized script **directly** into
`.claude/settings.json` within a managed block — no adapter.

#### Scenario: Claude wiring
- **GIVEN** an enabled `claude` agent and a `pre-tool-use` hook with a matcher
- **WHEN** sync runs
- **THEN** `.claude/settings.json` SHALL contain, inside the managed block, a `PreToolUse` entry whose matcher equals the hook matcher and whose command invokes the materialized script directly

### Requirement: Adapter rendering for non-native harnesses (Cursor, OpenCode, Pi, Omp)

Harnesses that do not decide by exit code SHALL each get a **generated adapter**
that runs the materialized script with the normalized event on stdin and
translates `exit 2` into that harness's native block signal. The adapter is the
only generated artifact that differs per harness; the script stays single-source.

#### Scenario: Cursor wrapper translates exit code to permission JSON
- **GIVEN** an enabled `cursor` agent and a `pre-tool-use` hook mapped to a Cursor blocking event (e.g. `beforeShellExecution`)
- **WHEN** sync runs
- **THEN** sync SHALL generate a shell wrapper referenced by `.cursor/hooks.json` that execs the script and emits `{"permission":"deny", ...}` on `exit 2`, else `{"permission":"allow"}`
- **AND** the response fields SHALL use snake_case (`permission`, `user_message`, `agent_message`)

#### Scenario: OpenCode plugin throws on block
- **GIVEN** an enabled `opencode` agent and a `pre-tool-use` hook
- **WHEN** sync runs
- **THEN** sync SHALL generate `.opencode/plugin/<recipe>-<hook>.ts` that registers `tool.execute.before`, spawns the script, and `throw`s when the script exits `2`

#### Scenario: OpenCode matcher is case-insensitive
- **GIVEN** an enabled `opencode` agent and a `pre-tool-use` hook with a non-empty matcher
- **WHEN** sync generates the OpenCode plugin
- **THEN** the plugin SHALL construct the matcher as `new RegExp(\`^(?:${MATCHER})$\`, "i")`

#### Scenario: Pi extension returns block
- **GIVEN** an enabled `pi` agent and a `pre-tool-use` hook
- **WHEN** sync runs
- **THEN** sync SHALL generate `.pi/extensions/<recipe>-<hook>.ts` (importing `@earendil-works/pi-coding-agent`) that registers `pi.on("tool_call", …)`, spawns the script, and returns `{ block: true }` when the script exits `2`

#### Scenario: Omp extension returns block
- **GIVEN** an enabled `omp` agent and a `pre-tool-use` hook
- **WHEN** sync runs
- **THEN** sync SHALL generate `.omp/extensions/<recipe>-<hook>.ts` (importing `@oh-my-pi/pi-coding-agent`) that registers `pi.on("tool_call", …)`, spawns the script, and returns `{ block: true }` when the script exits `2`

#### Scenario: No pre-file-write target on Cursor
- **GIVEN** an enabled `cursor` agent and a `pre-tool-use` hook whose `matcher` targets file writes (e.g. `Edit|Write`)
- **WHEN** sync runs
- **THEN** sync SHALL warn that Cursor has no pre-file-write hook and SHALL skip the hook for Cursor, while still rendering it for harnesses that support file-write gating

### Requirement: Documented pre-tool-use reach is honest about process boundaries

Product documentation for runtime hooks SHALL state that pi and omp `tool_call`
handlers apply to tool calls in **that agent process**, and SHALL NOT claim they
cover subprocess/subagent-delegated tool calls unless that coverage is verified.
OpenCode's primary-agent-only caveat (subagent/MCP) SHALL remain. Guidance SHALL
state that worktree/plan-build gates must not be the sole guard for
delegation-heavy workflows on opencode/pi/omp.

#### Scenario: Status table distinguishes process vs cross-process coverage
- **GIVEN** a reader of `docs/runtime-hooks.md`
- **WHEN** they consult the pre-tool-use status table
- **THEN** pi and omp SHALL be described as blocking in the current agent process
- **AND** omp SHALL appear as its own row

### Requirement: Config values flow to hooks

Hook behavior tunable through recipe `[config.*]` overridden in
`[recipes.<id>.config]` SHALL be made available to the rendered hook (e.g. via
environment variables on the generated wiring), without declaring the hook in
the project manifest.

#### Scenario: Overridden config reaches the hook
- **GIVEN** a recipe hook that reads `WORKTREE_GATE_PROTECTED` and a manifest `[recipes.worktree-flow.config]` setting protected branches
- **WHEN** sync renders the hook
- **THEN** the rendered wiring SHALL pass the resolved config value to the script's environment

### Requirement: Idempotent managed materialization

Hook wiring SHALL be written inside a managed block keyed by recipe + hook id so
a second `sync` with no manifest change produces no diff and never clobbers
user-authored hook entries outside the managed block.

#### Scenario: Second sync produces no diff
- **GIVEN** a synced project with rendered hooks
- **WHEN** sync runs again with no manifest change
- **THEN** the harness hook configs SHALL be byte-identical

#### Scenario: User hooks preserved
- **GIVEN** a `.claude/settings.json` with a user-authored hook outside the managed block
- **WHEN** sync runs
- **THEN** the user-authored hook SHALL be preserved and only the managed block SHALL be rewritten
