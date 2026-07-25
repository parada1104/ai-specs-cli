# Design: honest runtime-hook coverage + OpenCode matcher fix

## Ground truth (D) — recorded

Upstream `@earendil-works/pi-coding-agent` (and omp's fork-shaped
`@oh-my-pi/pi-coding-agent` adapter) expose `pi.on("tool_call", …)` for tools
executed **in that agent process**. The shipped `examples/extensions/subagent/`
pattern documents that delegation **spawns a separate `pi` process** per
subagent. Therefore:

- The **parent** process's extension handlers see the parent's tool calls
  (including the delegation/`subagent`/`task` tool itself).
- They do **not** automatically see the child's `write`/`edit`/`bash` calls.
- Whether the **child** enforces project hooks depends on whether that child
  process loads the same project extensions — not part of the ai-specs
  distribution contract today, and not claimed as guaranteed.

This matches the dogfood incident (direct writes blocked; delegated writes
bypassed) and aligns pi/omp with OpenCode's documented primary-agent-only
caveat for different host reasons (OpenCode: event simply does not fire for
subagent/MCP; pi/omp: event fires per-process, subprocesses are separate).

**Doc status to publish:**

| harness | `pre-tool-use` blocking | notes |
|---|---|---|
| claude | ✅ | native exit-2 |
| pi | ✅ (this process) | not a cross-process subagent guarantee |
| omp | ✅ (this process) | same handler shape as pi; was missing from docs |
| opencode | ✅ (primary agent) | not subagent/MCP (upstream #5894 / #2319) |
| cursor | ⚠️ | no pre-file-write hook |

## Implementation shape

### E — OpenCode matcher case-insensitivity

In `lib/_internal/hooks-render.py` `render_opencode`, change:

```ts
const re = new RegExp(`^(?:${MATCHER})$`);
```

to:

```ts
const re = new RegExp(`^(?:${MATCHER})$`, "i");
```

Add a short comment mirroring pi/omp (tool ids may be lowercase). Extend
`tests/test_hooks_render.py::test_opencode_plugin_shim` to assert the `"i"`
flag is present (same assertion already used for pi/omp).

### A — Docs

Update `docs/runtime-hooks.md`:

1. Abstract→native map: add **omp** column (same native events as pi:
   `tool_call` / `tool_result` / `session_start` / `agent_end`).
2. Per-harness distribution table: add **omp** row (`.omp/extensions/…`,
   `@oh-my-pi/pi-coding-agent`, `{ block: true }`).
3. Known gaps: add bullet that pi/omp `tool_call` is **per agent process**;
   subprocess/subagent delegation does not inherit parent handlers unless the
   child loads the same extensions (not guaranteed). Restate OpenCode caveat.
4. Status table: replace "pi | ✅ | covers all tool calls" with the table above;
   include omp.

### B — Orchestrator pre-delegation rule

Add one `workflow_rules` entry to
`catalog/recipes/worktree-flow/recipe.toml` `[provides.brief]`:

> Before dispatching a write-capable subagent or task, verify the current
> worktree and branch yourself (`git rev-parse --show-toplevel`,
> `git branch --show-current`, `git worktree list`). Do not rely solely on
> runtime pre-tool-use hooks — they may not fire for delegated/subprocess tool
> calls on opencode/pi/omp.

Also surface the same guidance briefly in
`catalog/recipes/worktree-flow/skills/worktree-flow/SKILL.md` (When to create
a worktree / Rules) and a short note in the recipe README under the gate
section, so skill-driven and brief-driven paths converge.

No new hook script; no change to `worktree-gate.sh` decision logic.

### Spec deltas

- `runtime-hook-distribution`: document omp as a first-class adapter target;
  require OpenCode matcher case-insensitivity; document per-process / primary-
  agent reach limits in Requirements (honest coverage).
- `worktree-flow`: require the pre-delegation workflow_rule in the recipe brief.

## Out of scope (unchanged from proposal)

Nested-subagent live evals; Cursor file-write hooks; upstream OpenCode #5894
fix; pi/omp relative-script cwd hardening (Trello #7).
