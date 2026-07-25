# Honest runtime-hook coverage + OpenCode matcher fix

## Problem

A live dogfood incident showed that `worktree-flow:worktree-gate` (generated,
wired, and enabled with `WORKTREE_GATE_PROTECTED="main development"`) did **not**
fire when an omp-family orchestrator delegated edits of production files to a
`task` subagent. The subagent wrote on `development` in the main worktree; the
gate never blocked or warned. Direct (non-delegated) writes in the same session
**were** blocked correctly.

The first hypothesis — a tool-name regex miss for session tools like `_edit` /
`_write` — was **refuted** by that direct-write check. The real failure mode is
different and already partially known in this repo:

1. **Subagent tool-call visibility** — OpenCode already documents that
   `tool.execute.before` does not fire for subagent/MCP calls
   (`docs/runtime-hooks.md`, opencode#5894 / #2319). `pi` is listed as
   "✅ covers all tool calls" with no sourced evidence for *delegated* calls;
   `omp` is a first-class harness in code (`render_omp`, platform map) but is
   **absent** from every compatibility table in `docs/runtime-hooks.md`. This
   session's incident is evidence that at least this omp-family build behaves
   like the undocumented case, not the claimed "✅ all tool calls" case.
2. **Orchestrator / gate tension** — project workflow rules encourage
   delegation ("fan work out to task subagents"). That is exactly the path that
   bypasses a pre-tool-use hook that only sees top-level calls. A security-
   relevant gate cannot have its only enforcement layer be an event that may
   not fire under the project's own recommended workflow.
3. **Separate, smaller bug** — `render_opencode` builds
   `new RegExp(\`^(?:${MATCHER})$\`)` **without** the `"i"` flag, while
   `render_pi` / `render_omp` use `"i"` because tool names are often lowercase.
   If OpenCode reports lowercase tool ids, top-level Edit/Write matchers never
   match either. This is independent of subagent visibility and was not live-
   re-verified against an OpenCode binary during exploration ([INFERENCE]).

Live evals (`tests/evals`) drive a single top-level agent; they cannot
distinguish "gate fires for direct edits" from "gate fires for delegated
edits," so this class of gap stays invisible until a real multi-agent session
hits it.

See `exploration.md` in this change folder for the full decision trail.

## Solution

Ship a **truthful + defense-in-depth** slice — not a false "hook fixed" claim:

1. **Verify ground truth (D)** — Confirm from `@oh-my-pi/pi-coding-agent` /
   `@earendil-works/pi-coding-agent` sources (and/or a minimal live probe)
   whether `tool_call` fires for subagent-originated tool invocations. Record
   the finding in the change artifacts and in `docs/runtime-hooks.md`. If the
   host API cannot see subagent calls, treat it as an inherent limitation (like
   OpenCode); if it should and does not, file/track an upstream defect.
2. **Correct docs (A)** — Add the missing **omp** row to every relevant table in
   `docs/runtime-hooks.md`. Downgrade unverified "✅ covers all tool calls" for
   pi (and omp) to an accurate, sourced status (verified / unverified /
   primary-agent-only). Document that worktree/plan-build gates must not be
   treated as the sole guard for delegation-heavy workflows on
   opencode/pi/omp when subagent coverage is absent or unverified.
3. **Orchestrator-level guard (B)** — Add an always-on runtime rule (brief /
   workflow_rules surface used by this project) requiring the orchestrator to
   verify current worktree + branch **before** dispatching any write-capable
   subagent/task, independent of whether a runtime hook will fire. This is
   defense-in-depth at the layer that actually failed in the incident.
4. **OpenCode matcher case-insensitivity (E)** — Add the `"i"` flag to
   `render_opencode`'s generated matcher regex so it matches `render_pi` /
   `render_omp`. Cover with a unit/golden test. Optionally confirm via the
   existing `requires_hook` live eval when an `opencode` binary is available.

**Non-goals for this change:** inventing a new host API that forces subagent
hooks to fire; changing Cursor's known lack of a pre-file-write event;
rewriting the eval harness to spawn nested agents (may be a follow-up card
once D answers whether such an eval is even meaningful).

## Affected modules

- `docs/runtime-hooks.md` — omp rows; honest pi/omp/opencode status; known-gaps
  wording for subagent delegation.
- `lib/_internal/hooks-render.py` — `render_opencode` matcher `"i"` flag (+ any
  generated-comment consistency).
- Tests covering OpenCode shim generation / matcher behavior (existing hooks-
  render / golden tests — extend rather than invent a new harness).
- Runtime brief / workflow rule surface that ships the pre-delegation check
  (exact file(s) decided in design: recipe brief fragment vs local skill vs
  AGENTS.md-managed rule — prefer the always-on brief path already used for
  worktree/VCS rules).
- Possibly `catalog/recipes/worktree-flow` and/or `plan-build-flow` README /
  brief notes pointing at the documented limitation (no behavior change to the
  shell gates themselves beyond docs).

## Out of scope

- Fixing OpenCode's host-level subagent/MCP hook gap (upstream only).
- Making Cursor block file writes via pre-tool-use (no native event).
- Nested-subagent live-eval scenarios (follow-up once D is settled).
- Changing matcher strings in recipe.toml (`Edit|Write|...`) beyond what the
  case-insensitive flag already covers.
- Path-resolution hardening for pi/omp hook script `cwd` (Trello #7 — separate).

## Capabilities

### Modified

- `runtime-hook-distribution` — document accurate per-harness `pre-tool-use`
  reach (including omp); OpenCode adapter matcher case-insensitivity.

### Unchanged

- Normalized stdin JSON / exit-code contract for hook scripts.
- Recipe `[[provides.hooks]]` schema.
- `worktree-gate.sh` / `plan-build-gate.sh` decision logic (still exit 2 on
  protected writes when the event reaches them).

## Risks / open questions

| Risk / question | Mitigation |
|---|---|
| D finds pi *does* cover subagents and this incident was config/version-specific | Docs still gain omp rows + "do not sole-rely" guidance; B still valuable; E still ships |
| Prompt-level B is ignored the same way the skill was | Acceptable as defense-in-depth; pair with honest docs so humans know the hook is not enough |
| Shipping only E looks like "the gap is fixed" | Explicit out-of-scope and docs language; do not claim subagent coverage from a regex change |
| OpenCode tool names are not lowercase ([INFERENCE]) | `"i"` is still correct and aligned with siblings; live eval is confirmatory, not blocking |

## Success criteria

- `docs/runtime-hooks.md` lists **omp** wherever other harnesses appear, with
  status grounded in the D finding (or explicitly marked unverified pending D
  if verification is deferred to apply).
- pi/omp subagent coverage claims are no longer stronger than evidence.
- Always-on rule: check worktree/branch before dispatching write-capable
  subagents.
- OpenCode-generated matcher uses case-insensitive RegExp; unit/golden coverage
  proves it.
- Full `./tests/validate.sh` green.
