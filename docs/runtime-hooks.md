# Runtime hooks

Recipes can ship **agent-runtime lifecycle hooks** via `[[provides.hooks]]`.
`ai-specs sync` distributes one portable script to every enabled harness in its
native runtime-hook format. The author writes the script once; the product owns
the per-harness adapters.

> Distinct from the sync-time `[[hooks]]` table (`event = "on-sync"`), which runs
> *during* `ai-specs sync` materialization. Runtime hooks fire while the coding
> agent runs (per tool call, session, stop).

## Normalized script contract

A hook script receives a normalized JSON event on **stdin** and communicates its
decision through its **exit code**:

```json
{ "event": "pre-tool-use", "tool_name": "Edit",
  "tool_input": { "file_path": "..." }, "cwd": "..." }
```

| exit code | meaning |
|-----------|---------|
| `0` | allow the action |
| `2` | block the action (stderr is surfaced to the agent) |
| any other | non-blocking error → **fail-open** (allow) |

One contract for all harnesses. A buggy guard must never wedge all work.

## Abstract → native event map (v1)

The product owns a fixed set of abstract events mapped to each harness's native
event. This map is the single source of truth used by every renderer
(`lib/_internal/hooks-render.py`).

| abstract | claude | cursor | opencode | pi | omp |
|----------|--------|--------|----------|-----|-----|
| `pre-tool-use` | `PreToolUse` | `beforeShellExecution` (shell/MCP only; **no pre-file-write hook**) | `tool.execute.before` | `tool_call` | `tool_call` |
| `post-tool-use` | `PostToolUse` | `afterShellExecution` | `tool.execute.after` | `tool_result` | `tool_result` |
| `session-start` | `SessionStart` | `sessionStart` | — (observe-only) | `session_start` | `session_start` |
| `stop` | `Stop` | `stop` | — (no equivalent) | `agent_end` | `agent_end` |

Unsupported `(event, harness)` pairs (—) are **warned and skipped** — sync never
emits broken wiring, and continues rendering the hook for harnesses that support
it.

## Per-harness distribution

| harness | wiring | block signal |
|---------|--------|--------------|
| **claude** | script wired directly into a managed block in `.claude/settings.json` | script `exit 2` (native) |
| **cursor** | generated shell wrapper in `.cursor/hooks/<recipe>-<hook>.sh`, referenced by `.cursor/hooks.json` | wrapper emits `{"permission":"deny", ...}` (snake_case) on `exit 2` |
| **opencode** | generated TS plugin `.opencode/plugin/<recipe>-<hook>.ts` | `throw` on `exit 2` |
| **pi** | generated TS extension `.pi/extensions/<recipe>-<hook>.ts` (imports `@earendil-works/pi-coding-agent`) | `return { block: true, reason }` on `exit 2` |
| **omp** | generated TS extension `.omp/extensions/<recipe>-<hook>.ts` (imports `@oh-my-pi/pi-coding-agent`) | `return { block: true, reason }` on `exit 2` |

Only **Claude** natively honors the normalized contract, so it gets the script
wired directly. Every other harness decides through a different channel, so sync
generates a thin **adapter** that runs the script with the normalized event on
stdin and translates `exit 2` into that harness's native decision.

### Known gaps

- **Cursor** has no generic "pre tool" event and **no pre-file-write hook**. A
  `pre-tool-use` hook whose matcher targets file writes (`Edit`/`Write`/
  `MultiEdit`/`NotebookEdit`) has no Cursor target → warn-and-skip.
- **OpenCode** `tool.execute.before` does **not** fire for **subagent** tool
  calls (opencode#5894) or **MCP** tool calls (opencode#2319).
- **Pi / omp** `tool_call` handlers apply to tool calls in **that agent
  process**. Typical subagent/task delegation spawns a **separate process**;
  the parent's extension handlers do not see the child's `write`/`edit` calls.
  Child enforcement only happens if that child also loads the same project
  extensions — not part of the ai-specs distribution contract. Do not treat
  worktree/plan-build gates as the sole guard for delegation-heavy workflows.
- **OpenCode** `session-start`/`stop` are observe-only; there is no blocking
  session hook.

## Script materialization

The hook script is materialized once to a harness-neutral path under the project
and made executable:

```
ai-specs/recipes/<recipe-id>/hooks/<script>
```

Every harness's wiring references this single copy.

## Config flow

Tunable values declared in the recipe `[config.*]` (overridable per-project in
`[recipes.<id>.config]`) reach the rendered hook as **environment variables**.
For example, `worktree-flow` exposes `WORKTREE_GATE_PROTECTED` (space-separated
protected branch names) to its `worktree-gate.sh` hook.

Some hooks also use a sync-stamped constant in the rendered script for values
that should not be overwritten by env export order. `worktree-flow` uses this
for `gate_mode`: `ai-specs sync` stamps the resolved mode into
`worktree-gate.sh`, while `WORKTREE_GATE_MODE` remains a one-shot process
override at dispatch time.

## Idempotency

All generated wiring lives in a managed block keyed by
`ai-specs:hooks:<recipe>:<hook>` (a `_ai_specs_managed` tag in JSON; a wholly
generated file for TS shims and the Cursor wrapper). A second `sync` with no
manifest change produces no diff, and user-authored hooks outside the managed
block are preserved.

## Status

| harness | `pre-tool-use` blocking | notes |
|---------|-------------------------|-------|
| claude | ✅ | native exit-2 |
| pi | ✅ (this process) | not a cross-process subagent guarantee |
| omp | ✅ (this process) | same handler shape as pi |
| opencode | ✅ (primary agent) | not subagent/MCP tool calls |
| cursor | ⚠️ | no pre-file-write hook; shell/MCP gates only |
