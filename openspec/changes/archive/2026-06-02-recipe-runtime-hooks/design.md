# Design: Recipe-declared runtime hooks, distributed to every harness

## Technical Approach

Add `[[provides.hooks]]` to the recipe schema and a new render stage in the sync
pipeline that fans each declared hook out to every enabled harness. The author
writes one script honoring a normalized contract; the pipeline owns the four
adapters. Mirror the existing `runtime-brief-rendering` pattern: `sync.sh`
pre-resolves data and passes a `--resolved-hooks <json>` blob to a renderer that
has no catalog access.

## Architecture Decisions

1. **Hooks are a recipe primitive, never a manifest section.** Declared only in
   `recipe.toml` under `[[provides.hooks]]`. The project manifest only enables
   the recipe. Tunables ride the existing `[config.*]` → `[recipes.<id>.config]`
   override path; resolved values reach the hook via env on the rendered wiring.
2. **Write-once script + product adapters.** One language-agnostic script
   (shell/python) with a normalized stdin-JSON + exit-code contract. Rejected:
   per-harness authored hooks (authoring burden, drift).
3. **One native + three adapters** (revised after verification). Only **Claude**
   natively matches the normalized contract (stdin JSON + `exit 2` → block), so it
   gets the script wired directly into `.claude/settings.json`. Every other harness
   decides through a different channel, so sync generates a per-harness **adapter**
   that runs the script with the normalized event on stdin, reads its exit code,
   and translates to that harness's native decision:
   - **Cursor** (`.cursor/hooks.json`) — decision is **stdout JSON**
     (`{"permission":"deny","user_message":…,"agent_message":…}`, snake_case),
     NOT the exit code. Generate a thin **shell wrapper** that execs the script and
     emits `permission:"deny"` when it exits `2`, else `permission:"allow"`.
   - **OpenCode** (`.opencode/plugin/<…>.ts`) — decision is **`throw`**. Generate a
     TS plugin (`tool.execute.before`) that spawns the script and throws on `exit 2`.
   - **Pi** (`.pi/extensions/<…>.ts`) — decision is **`return {block:true,reason}`**.
     Generate a TS extension (`pi.on("tool_call",…)`) that spawns the script and
     returns block on `exit 2`.
4. **Single materialized script, harness-neutral path.**
   `ai-specs/recipes/<recipe-id>/hooks/<script>` — every harness's wiring points
   at this one copy. Made executable on materialize.
5. **Managed block for idempotency.** All generated wiring lives inside a block
   keyed by `ai-specs:hooks:<recipe>:<hook>` (JSON: a managed sub-object; TS:
   a generated file is wholly owned). Re-sync rewrites only the managed region;
   user-authored hooks outside it are preserved. Second sync → no diff.
6. **Warn-and-skip unsupported pairs.** If an abstract event has no native
   mapping for an enabled harness, log a warning (recipe, hook, event, harness)
   and skip — never emit broken wiring. No silent drops.
7. **Fail-open script semantics.** Exit 0 allow, 2 block, anything else →
   proceed. A buggy guard must never wedge all work.

## Data Flow

```
recipe.toml [[provides.hooks]]
   │  (recipe-materialize.py parses + validates)
   ├─► materialize script → ai-specs/recipes/<id>/hooks/<script> (chmod +x)
   └─► emit --resolved-hooks JSON  { hooks:[{recipe,id,event,matcher,blocking,
                                      script_path,env}], enabled_agents }
                  │  (sync.sh passes the blob, mirrors --resolved-config)
                  ▼
          hooks-render.py  (no catalog access)
                  │  for each enabled agent × hook:
                  │    map abstract event → native (skip+warn if unmapped)
                  ├─ claude  → merge managed block into .claude/settings.json
                  ├─ cursor  → merge managed block into .cursor/hooks.json
                  ├─ opencode→ write .opencode/plugin/<recipe>-<hook>.ts
                  └─ pi      → write .pi/extensions/<recipe>-<hook>.ts
```

## File Changes

| File | Change |
|------|--------|
| `lib/_internal/recipe-materialize.py` | Parse/validate `[[provides.hooks]]`; materialize + chmod scripts; emit `--resolved-hooks-out`. |
| `lib/_internal/hooks-render.py` | **New.** Abstract→native event map; per-harness renderers; managed-block merge; warn-and-skip. |
| `lib/sync.sh` | `mktemp` a `RESOLVED_HOOKS_TEMP`; pass `--resolved-hooks-out` to materialize and `--resolved-hooks` to `hooks-render.py`; `rm -f` at cleanup. |
| `lib/sync-agent.sh` | Invoke `hooks-render.py` in the per-agent fan-out, passing the enabled agent + resolved-hooks blob. |
| `lib/_internal/platform.sh` | Add per-agent hook target paths/format (settings.json key, hooks.json, plugin dir, extensions dir). |
| `catalog/recipes/worktree-flow/recipe.toml` | Declare the worktree-gate hook via `[[provides.hooks]]`; drop the prototype template entry. |
| `catalog/recipes/worktree-flow/hooks/worktree-gate.sh` | Keep; adjust to the normalized event contract if needed. |
| `docs/recipe-schema.md`, `docs/runtime-hooks.md`, `README.md` | Document the primitive + compatibility/status. |

## Interfaces / Contracts

### `[[provides.hooks]]` schema
```toml
[[provides.hooks]]
id          = "worktree-gate"      # required, unique within recipe
event       = "pre-tool-use"       # required, known abstract event
script      = "hooks/worktree-gate.sh"  # required, path inside recipe dir
matcher     = "Edit|Write|MultiEdit|NotebookEdit"  # optional
blocking    = true                 # optional, default false
description = "Block writes to the main worktree on a protected branch"  # optional
```

### Abstract → native event map (v1) — VERIFIED June 2026

| abstract | claude | cursor | opencode | pi |
|----------|--------|--------|----------|-----|
| `pre-tool-use` | `PreToolUse` ✅block | `beforeShellExecution`/`beforeMCPExecution`/`beforeReadFile` ⚠️ (no generic, no pre-file-write) | `tool.execute.before` ✅block | `tool_call` ✅block |
| `post-tool-use` | `PostToolUse` | `afterShellExecution`/`afterMCPExecution`/`afterFileEdit` (split, observe) | `tool.execute.after` | `tool_result` |
| `session-start` | `SessionStart` | `sessionStart` (observe) | `event`→`session.created` ⚠️observe-only | `session_start` (observe) |
| `stop` | `Stop` | `stop` (observe) | ❌ no equivalent (`session.idle` observe-only) | `agent_end` (observe) |

Key verified facts that shape rendering:
- **Cursor has no generic "pre tool" event and no pre-file-write hook.** It splits by
  category: `beforeShellExecution`, `beforeMCPExecution`, `beforeReadFile` (all
  block); `afterFileEdit` is post-only. A hook matching file writes (Edit/Write)
  therefore has **no Cursor target** → warn-and-skip. (The generic
  `preToolUse`/`postToolUse` names some docs show are UNCONFIRMED; do not emit.)
- **OpenCode** `session-start`/`stop` are observe-only via the `event` hook; there
  is no blocking session hook. Also: `tool.execute.before` does **not** fire for
  **subagent** tool calls (#5894) or **MCP** tool calls (#2319) — document this gap.
- **Pi** package renamed to `@earendil-works/pi-coding-agent` (old `@mariozechner`
  deprecated); engine Node ≥22.19; TS via jiti; blocking works in headless/print.

### Per-harness block contract (what the adapter translates `exit 2` into)
| harness | wiring | block signal | fail-open default |
|---------|--------|--------------|-------------------|
| claude | direct command in `.claude/settings.json` | script `exit 2` (native) | yes (non-0/2 → allow) |
| cursor | shell wrapper in `.cursor/hooks.json` | wrapper emits `{"permission":"deny"}` on script exit 2 | yes (`failClosed:true` to invert) |
| opencode | generated TS plugin | `throw` on script exit 2 | yes |
| pi | generated TS extension | `return {block:true,reason}` on script exit 2 | yes |

### worktree-gate coverage (concrete consequence for the first consumer)
`event=pre-tool-use`, `matcher=Edit|Write`, `blocking=true`:
| harness | enforced? | why |
|---------|-----------|-----|
| claude | ✅ yes | `PreToolUse` matches Edit/Write |
| pi | ✅ yes | `tool_call` covers all tools |
| opencode | ✅ yes (primary agent) | `tool.execute.before`; ⚠️ NOT subagent/MCP tools |
| cursor | ❌ warn-and-skip | no pre-file-write hook exists |

### Normalized event JSON (stdin to script)
```json
{ "event": "pre-tool-use", "tool_name": "Edit",
  "tool_input": { "file_path": "..." }, "cwd": "..." }
```
Exit: `0` allow · `2` block (stderr → agent) · other → fail-open allow.

### `--resolved-hooks` JSON shape
```json
{ "enabled_agents": ["claude","cursor","opencode","pi"],
  "hooks": [ { "recipe":"worktree-flow", "id":"worktree-gate",
    "event":"pre-tool-use", "matcher":"Edit|Write", "blocking":true,
    "script_path":"ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh",
    "env":{"WORKTREE_GATE_PROTECTED":"main development"} } ] }
```

### Managed block markers
- JSON (settings.json / hooks.json): a top-level/managed object the renderer owns
  wholesale, with sibling user keys preserved. Keyed comment-free; identified by
  a stable `_ai_specs_managed` subtree or equivalent.
- TS shim files + the Cursor shell wrapper: wholly generated, header comment
  `// GENERATED by ai-specs — do not edit` (`#` for the shell wrapper).

### Generated adapter shapes (one per non-Claude harness)
**Cursor wrapper** (`.cursor/hooks/<recipe>-<hook>.sh`, referenced by `hooks.json`):
```bash
#!/usr/bin/env bash
# GENERATED by ai-specs — runs the recipe script, maps exit 2 → Cursor deny.
out="$(WORKTREE_GATE_PROTECTED="$ENV…" "$SCRIPT")"; code=$?   # script reads stdin (Cursor passes it through)
if [ "$code" = 2 ]; then printf '{"permission":"deny","agent_message":%s}' "$(jq -Rs . <<<"$out")"; else printf '{"permission":"allow"}'; fi
exit 0
```
**OpenCode** `tool.execute.before` → `spawn(script)`, `if (code===2) throw new Error(stderr)`.
**Pi** `pi.on("tool_call", …)` → `spawn(script)`, `if (code===2) return {block:true, reason:stderr}`.
All three feed the script the same normalized JSON on stdin; only the decision
channel differs. Claude alone needs no adapter.

## Testing Strategy (TDD — tests first; runner `./tests/validate.sh`)

- **Schema (unit):** valid hook parses; missing field fails; unknown event fails;
  script-path escape fails; no-hooks passes.
- **Render goldens (unit):** for each harness, a fixture recipe → expected native
  wiring (settings.json managed block, hooks.json, opencode/pi shim contents).
- **Warn-and-skip:** event unmapped for a harness → warning emitted, hook absent
  for that harness, present for others.
- **Idempotency:** sync twice → byte-identical configs; user hook outside managed
  block preserved.
- **Script contract (integration):** drive `worktree-gate.sh` with normalized
  events → exit 0/2 as expected (reuse the prototype's 6 cases).

## Migration / Rollout

- Land schema + renderer + worktree-flow consumer together so the recipe never
  references an unimplemented primitive.
- Remove the prototype (template-bundled script + manual wiring) from the
  `recipe-docs-cleanup` branch so only the productized form ships.
- Version bump worktree-flow once, here (not in the docs branch).

## Open Questions — mostly RESOLVED by verification (June 2026)

1. ✅ **Cursor events** — RESOLVED. Confirmed set: `beforeShellExecution`,
   `afterShellExecution`, `beforeMCPExecution`, `afterMCPExecution`,
   `beforeReadFile`, `afterFileEdit`, `beforeSubmitPrompt`, `stop`, `sessionStart`,
   `sessionEnd`. Generic `preToolUse`/`postToolUse`/`subagent*` are UNCONFIRMED →
   do NOT emit. Decision channel is **stdout JSON `permission`**, not exit code →
   Cursor needs a shell wrapper. No pre-file-write hook → file-write gates skip Cursor.
2. ✅ **OpenCode plugin dir** — RESOLVED. Both `.opencode/plugin/` and
   `.opencode/plugins/` are globbed; emit singular `plugin/`. Block = `throw`.
   Documented gaps: subagent (#5894) and MCP (#2319) tool calls bypass the hook.
3. ✅ **Pi runtime** — RESOLVED. Package `@earendil-works/pi-coding-agent`, Node
   ≥22.19, TS via jiti, blocking works in headless/print (CI-safe). Block =
   `return {block:true}`. Gate generation on `pi` being enabled (it is here).
4. ⚠️ **Claude settings.json managed-block shape** — still to pin during apply:
   choose a managed subtree that round-trips and preserves sibling user hooks.

### Remaining (pin during apply, not blocking the plan)
- Version-pin each harness adapter to the user's installed version (events are
  version-sensitive: OpenCode #14808/#16879, Cursor camelCase→snake_case migration).
- Cursor response casing: emit **snake_case** (`permission`, `user_message`,
  `agent_message`) to match current docs.
