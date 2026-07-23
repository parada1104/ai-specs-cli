# Archive report — eval-wire-runtime-hooks

**Archived:** 2026-07-21
**Branch:** feat/eval-wire-runtime-hooks (stacked on feat/plan-build-gate-hook / #139)
**Status:** ready-to-merge (after #139)

## Outcome

- The live-eval harness now wires a recipe's `[[provides.hooks]]` into the
  runtime's native channel before invoking the agent, via `wire_runtime_hooks`
  (materialize with `resolved_hooks_out` + `hooks-render.py`, mapping
  `cursor-agent → cursor`). So live scenarios exercise runtime hooks
  end-to-end, not only the advisory skill/brief layer.
- Added a `requires_hook` scenario field; the runner skips such scenarios on
  runtimes with no pre-file-write event (`cursor-agent`). `ac8` sets it.
- Non-live coverage: `test_eval_hook_wiring` proves claude gets the `PreToolUse`
  entry and cursor does not.

## Validated live

Instrumented the wired hook with a sentinel and ran `claude -p`: the trace
showed the hook **FIRED** — headless claude executes the wired project
`PreToolUse` hook. The "does headless claude run project hooks" risk is
**resolved**; no permissions grant needed.

## Accepted limitation

A diagnostic run showed the gate enforces **plan-before-production**, not
**stop-after-planning**: when blocked, claude wrote a minimal plan folder and
then edited production. So `ac8`'s `forbidden src/**` assertion is not a clean
test of the hook and stays partly advisory/flaky. The hook's real guarantee is
covered deterministically by the 12 `test_plan_build_gate_hook` unit tests.
Maintainer accepted this as-is rather than pursuing stream-json event assertions.

## Files changed

- `tests/evals/lib/harness.py` — `wire_runtime_hooks`, `RUNTIME_TO_AGENT`, `NO_FILE_WRITE_HOOK_RUNTIMES`.
- `tests/evals/eval_plan_build_flow_live.py` — call wiring; skip `requires_hook` on cursor-agent.
- `tests/evals/scenarios/plan-build-flow/ac8_approval_verb_without_folder/scenario.toml` — `requires_hook = true`.
- `tests/test_eval_hook_wiring.py` — non-live wiring coverage.
- `openspec/specs/recipe-evals/spec.md` — two requirements (AC8) promoted from delta.

## Verification

- `./tests/validate.sh` — exit 0.
- Full `pytest tests/` — 1010 passed, 143 subtests passed.
- Live: `claude -p` with instrumented hook — hook FIRED.
