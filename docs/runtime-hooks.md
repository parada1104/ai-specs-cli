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
override at dispatch time. `trello-mcp-workflow` likewise stamps
`__TRACKER_CARD_GATE_MODE__` (default `warn`) and `__TRACKER_CLI_HOME__`
into `tracker-card-gate.sh`; `TRACKER_CARD_GATE_MODE` is the one-shot env
override.

## Gate implementation and launcher (worktree-flow)

`worktree-flow`'s `worktree-gate.sh` is a **thin launcher** (bash 3.2 only) that
resolves the gate implementation and `exec`s it, so stdin and the exit code pass
through untouched. The only implementation is a single zero-dependency Go
binary. `gate_impl` accepts `auto | go` only; both acquire the verified binary
and fail open when none resolves (`ai-specs doctor` reports ERROR).

Resolution order (first hit wins):

1. `$WORKTREE_GATE_BIN` — explicit per-invocation override (debugging/pinning).
2. Project-local pin `bin/worktree-gate` under the launcher's `BASH_SOURCE[0]`
   physical installation root (the `hooks/../bin` layout) — never `$PWD`.
3. Version-keyed cache
   `$AI_SPECS_HOME/cache/bin/worktree-gate/<cli-version>/<goos>-<goarch>/worktree-gate`,
   populated by `ai-specs sync` (digest-verified against the committed
   `SHA256SUMS` trust root, mode 0755, self-tested, atomic install).
4. Otherwise one stderr warning naming the missing path and the
   `ai-specs sync` / `ai-specs sync --refresh-gates` / `ai-specs doctor`
   remedy, then exit `0` (fail open).

The build matrix is `darwin/arm64`, `darwin/amd64`, `linux/amd64`,
`linux/arm64` (reproducible: `CGO_ENABLED=0`, `-trimpath`, `-buildvcs=false`,
version stamped at link time). The gate never computes a digest on the
invocation path unless `WORKTREE_GATE_VERIFY=1` requests it.

**Recovery (no Bash rollback path):** per invocation, set
`WORKTREE_GATE_MODE=off` or `WORKTREE_GATE_BIN=<path>`. Per install, `rm -rf
$AI_SPECS_HOME/cache/bin/worktree-gate` then `ai-specs sync`. Full revert is
install the previous CLI release and run `ai-specs sync`. `gate_impl = auto`
with no usable binary is a doctor ERROR, not a silent fallback.

Because every renderer references only `hook["script_path"]`, all five harnesses
keep working without re-render churn: the launcher materializes at the unchanged
path `ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh`, and the Cursor
wrapper maps exit `2` → `{"permission":"deny"}` exactly as before (the binary's
empty stdout does not degrade the deny decision — the gate message travels on
stderr).

### Workspace context model

The hook keeps three distinct contexts, and no adapter or launcher conflates
them:

| Context | Producer | Meaning | Fallback |
|---------|----------|---------|----------|
| Event cwd | Runtime adapter + normalized event | Directory whose repository/worktree context the gate evaluates | Gate process cwd when the event value is unusable |
| Installation root | Materialized launcher / generated module | Root used to find project-local hook assets | No process-cwd substitution for project-local assets |
| Process cwd | Actual hook child process | Fallback context when the event cwd is unusable | The OS/runtime process cwd |

- **Adapter launcher paths are module-derived.** The generated OpenCode plugin
  and Pi/OMP extensions resolve the materialized launcher at runtime from
  `import.meta.url` (`../../` from `.opencode/plugin`, `.pi/extensions`, or
  `.omp/extensions` up to the project root), so a relocated extension keeps
  working from any process cwd. No generated adapter emits a relative `SCRIPT`
  or a sync-time machine-specific absolute path.
- **OpenCode validates the explicit directory.** `directory` is outer-trimmed
  and accepted only as a string, absolute, existing directory; the one
  normalized value drives both the event `cwd` and the child `spawnSync` cwd.
  Absent, non-string, whitespace-only, relative, nonexistent, or non-directory
  values fall back to the process cwd for both. Child `spawnSync` errors and
  thrown child-process exceptions fail open; status `2` remains the only block.
- **The launcher derives its installation root from `BASH_SOURCE[0]`.** A
  relative reference is anchored to the invocation cwd exactly once, the final
  launcher symlink (and parent symlinks) resolve to the physical launcher, and
  project-local assets resolve as `hooks/../bin` under that root —
  never from `$PWD`. An unresolvable root skips project-local lookup
  and continues through the explicit override or cache, or fails open.
- **Pi/OMP keep process-cwd-only events** and claim no workspace root; Cursor's
  missing pre-file-write hook and OpenCode's subagent/MCP coverage gap are
  unchanged (see below).

**Unchanged pre-existing coverage gaps** (not introduced by the Go cutover):

- **Cursor** has no generic "pre tool" event and **no pre-file-write hook**;
  a file-write-matcher `pre-tool-use` hook has no Cursor target → warn-and-skip.
  The shell matcher still renders as a genuine `beforeShellExecution` hook.
- **OpenCode** `tool.execute.before` does **not** fire for **subagent** tool
  calls (opencode#5894) or **MCP** tool calls (opencode#2319).
- **Pi / omp** `tool_call` handlers apply to tool calls in **that agent
  process**; subagent/task delegation spawns a separate process whose
  `write`/`edit` calls the parent's handlers do not see.

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

## Dual-hook gates (worktree-flow + trello-mcp-workflow)

| Recipe | Path hook id | Shell hook id | Shell heuristic |
|--------|--------------|---------------|-----------------|
| `worktree-flow` | `worktree-gate` | `worktree-gate-shell` | shell writes into protected main |
| `trello-mcp-workflow` | `tracker-card-gate` | `tracker-card-gate-shell` | `gh pr create` + change-archive helpers |

Both share one script per recipe with two `[[provides.hooks]]` ids so
Cursor's file-write skip does not swallow shell coverage. Neither gate
intercepts MCP tool calls.

## Shell write-bypass coverage (worktree-flow)

`worktree-flow`'s `worktree-gate.sh` also registers a second matcher
(`worktree-gate-shell`, matcher `Bash|Shell|Execute|Terminal`) that
heuristically detects shell commands (redirects, `tee`, `sed -i`/`perl -i`,
`cp`/`mv`, interpreter `-c`/heredoc bodies calling `write_text`/`open(...,'w')`
and language-specific writers) that would write into the protected main
worktree — closing the gap where an agent falls back to `bash` after a
structured Edit/Write call is blocked or errors. The heuristic is
**best-effort and fails open** on any ambiguous, unparseable, or
out-of-scope command; it is not a general shell-command sandbox.

Coverage is **not uniform** across harnesses, by structural necessity (not
an oversight):

| harness | shell write-bypass detection | notes |
|---------|-------------------------------|-------|
| claude | ✅ | shell/Bash matcher intersects no file-write token; renders as a normal second `PreToolUse` entry |
| pi | ✅ (this process) | same `tool_call` handler shape as the path-write hook; not a cross-process subagent guarantee |
| omp | ✅ (this process) | same as pi |
| opencode | ✅ (primary agent) | not subagent/MCP tool calls, same limitation as the path-write hook |
| cursor | ✅ | registers as a genuinely separate `beforeShellExecution` hook (its own managed id), since `_matcher_targets_file_writes` only skips matchers that target Edit/Write/MultiEdit/NotebookEdit — the shell matcher never matches that set |

Where shell interception is structurally unavailable or unreliable (e.g. any
harness's subagent/MCP tool-call boundary), the mitigation is policy-level:
`worktree-flow`'s brief `workflow_rules` and `SKILL.md` explicitly instruct
agents that a blocked or errored Edit/Write on a protected branch is never
grounds to retry the write via bash/shell — the correct response is to create
a worktree first.

`tracker-card-gate` does **not** intercept Trello MCP and does **not**
claim uniform full prevention across harnesses — see Residual platform
gaps in the trello-mcp-workflow README.
