# Reinforce plan-build-flow with a pre-tool-use artifact gate hook

## Problem

The `plan-build-flow` gate ("classify → plan → stop for authorization → then
implement") is currently **advisory only**: it lives entirely in the bundled
`SKILL.md` prose and the AGENTS.md `workflow_rules`. Nothing machine-enforces it.
The skill frontmatter carries only `scope` + `auto_invoke` (discovery hints); the
"hard stop" sections are behavioral instructions the agent may or may not honor.

Observed failure: an agent (Claude Code) received an approval verb ("dale") with
no prior change folder and jumped straight to editing production code, skipping
the classify/plan/stop step — exactly the case the rule "approval verbs do not
skip the plan step" is meant to prevent. Advisory prose did not stop it.

The repo already proves a stronger pattern exists: `worktree-flow` ships
`worktree-gate.sh`, a `pre-tool-use` blocking hook (`matcher =
Edit|Write|MultiEdit|NotebookEdit`, `blocking = true`, `exit 2` blocks) wired
per-runtime by `hooks-render.py`. plan-build-flow declares no such hook.

Note on runtime reach: `hooks-render.py` can wire a file-write block on `claude`
(`PreToolUse`), `opencode`, `pi`, and `omp`, but **not** `cursor` (its
`beforeShellExecution` event covers shell/MCP only; file-write matchers are
skipped there). So this hook hardens the runtimes that can take it; Cursor keeps
the advisory layer it already follows well.

## Solution

Add a `pre-tool-use` hook `plan-build-gate.sh` to the plan-build-flow recipe,
mirroring the worktree-gate contract:

- Reads normalized stdin JSON `{event, tool_name, tool_input, cwd}`; `exit 0`
  allows, `exit 2` blocks; fail-open on any parse/lookup error.
- Blocks `Edit`/`Write`/`MultiEdit`/`NotebookEdit` on **production paths**
  (default top-level `src`, `lib`, `catalog`; override via `PLAN_BUILD_GATE_PATHS`)
  when **no** change folder exists (no `openspec/changes/*/tasks.md` outside
  `archive/`).
- Allows everything else: edits under `openspec/changes/**` (so the agent can
  always write the plan), tests, docs, and gitignored agent config.
- Non-bypassable by design: no on/off/ask mode. An on/off switch would just be a
  "skip the plan" affordance — the exact thing the gate prevents. The only knob
  is `PLAN_BUILD_GATE_PATHS` (which dirs count as production), scope config, not
  a switch. The way past the gate is to write the plan it asks for.

This enforces the artifact precondition of the gate: production code cannot be
written until planning artifacts exist. It does not (and cannot) prove a human
said "yes", but it removes the failure mode where the agent skips planning
entirely.

Also add an eval scenario `ac8_approval_verb_without_folder` covering the exact
regression: an approval-verb prompt with no seeded plan must still yield planning
artifacts and touch no production code.

## Affected modules

- `catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh` — new hook.
- `catalog/recipes/plan-build-flow/recipe.toml` — `[[provides.hooks]]` block.
- `tests/test_plan_build_gate_hook.py` — hook unit tests (exit-code contract).
- `tests/evals/scenarios/plan-build-flow/ac8_approval_verb_without_folder/` — new scenario.
- `tests/evals/eval_plan_build_flow_live.py` — register scenario + test method.
- `openspec/specs/plan-build-flow/spec.md` — new requirement (delta).

## Out of scope

- Any on/off/ask mode for this gate (deliberately omitted — a switch would
  defeat the purpose).
- Any Cursor file-write enforcement (not supported by the hook pipeline).
- Detecting explicit human authorization (a hook cannot; the artifact
  precondition is the enforceable proxy).
