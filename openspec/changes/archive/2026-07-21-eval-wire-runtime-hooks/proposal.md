# Wire runtime hooks in the live-eval harness so gate scenarios test end-to-end

## Problem

The plan-build live-eval harness (`tests/evals/lib/harness.py::materialize_project`)
only runs `recipe-materialize.materialize_recipes` — it never runs
`hooks-render.py`. So `[[provides.hooks]]` runtime hooks are **materialized but
never wired** into the runtime's native channel (`.claude/settings.json`
`PreToolUse`, opencode/pi/omp plugins). Verified: after `materialize_project`,
no `.claude/settings.json` exists in the fixture.

Consequence: the `plan-build-gate` hook is **inactive during live evals**. The
`ac8_approval_verb_without_folder` scenario therefore tests only the advisory
skill layer, not the gate it was written to validate. In the last run
`ac8`/cursor-agent edited production (advisory failure) and `ac8`/claude no-op'd
— neither exercised the hook.

## Solution

Make the harness wire the hook for the runtime under test, then scope
hook-dependent scenarios to runtimes that can take a file-write hook.

1. **Wire hooks per runtime.** Add `wire_runtime_hooks(project_root, runtime)`:
   - call `materialize_recipes(..., resolved_hooks_out=<temp>)` to emit the
     resolved-hooks JSON (the function already accepts this arg),
   - map the eval runtime id to the platform agent id (`cursor-agent → cursor`;
     others identity),
   - run `hooks-render.py <resolved-hooks.json> <agent> <project_root>` — the
     same call `sync-agent.sh` makes.
   Invoke it from `_run_scenario` after `setup_runtime_skills`.

2. **Scope hook-dependent scenarios.** Add an optional scenario field
   (`requires_hook = true`). `ac8` sets it. The runner skips that scenario on
   runtimes with no file-write hook event (`cursor-agent`), because
   `hooks-render` deliberately skips `Edit|Write` matchers for cursor — the hook
   cannot help there, so the advisory-only outcome is out of scope for a *gate*
   test.

3. **Deterministic wiring coverage.** Add a non-live unit test asserting that
   after wiring, the claude fixture's `.claude/settings.json` contains the
   `plan-build-gate` `PreToolUse` entry — so wiring is covered without burning
   live runs.

## Risks / open questions

- **Claude headless hook execution**: `claude -p` must actually run project
  `PreToolUse` hooks (may need a permissions setting in the fixture
  `.claude/settings.json`). If it does not run project hooks unattended, wiring
  alone won't gate claude in-eval; the fixture may need a permission grant. To
  be validated with one targeted live `ac8`/claude run.
- This does not give cursor-agent a gate (it structurally can't have one); `ac8`
  simply won't assert against cursor.

## Affected modules

- `tests/evals/lib/harness.py` — `wire_runtime_hooks`, runtime→agent map.
- `tests/evals/eval_plan_build_flow_live.py` — call wiring; honor `requires_hook`.
- `tests/evals/scenarios/plan-build-flow/ac8_approval_verb_without_folder/scenario.toml` — `requires_hook = true`.
- `tests/test_*` — new non-live wiring assertion.
- `openspec/specs/recipe-evals/spec.md` — delta.

## Out of scope

- Running the full `ai-specs sync` in the harness (targeted `hooks-render` call
  is enough; avoids skills/MCP side effects).
- Any change to the hook itself (shipped in #139).
