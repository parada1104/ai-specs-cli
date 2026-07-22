# Tasks: wire runtime hooks in the live-eval harness

## Planning depth

- **Classification**: Standard (spec + tasks). Multi-file eval-infra change;
  clear scope, needs written requirements.
- **Authorization**: PENDING — plan presented, awaiting maintainer go.
- **Dependency**: stacked on `feat/plan-build-gate-hook` (#139), where `ac8`
  and the hook live. Merge order: #139 first, then this.

## Implementation (red-green-refactor)

- [x] RED: non-live unit test — after `wire_runtime_hooks(root, "claude")`,
      `.claude/settings.json` contains a `PreToolUse` entry referencing
      `plan-build-gate`. And `wire_runtime_hooks(root, "cursor-agent")` does NOT
      add a file-write hook (documents the cursor limitation).
- [x] GREEN: `wire_runtime_hooks(project_root, runtime)` in `harness.py`
      (runtime→agent map; materialize with `resolved_hooks_out`; call
      `hooks-render.py`).
- [x] Call `wire_runtime_hooks` from `_run_scenario` after `setup_runtime_skills`.
- [x] Add `requires_hook` scenario field; runner skips such scenarios on
      runtimes without a file-write hook event (cursor-agent).
- [x] Set `requires_hook = true` in `ac8` scenario.toml.
- [x] Spec delta in `openspec/specs/recipe-evals/spec.md`.

## Validation

- [x] `./tests/validate.sh` exit 0; full `pytest tests/` green (1010 passed,
      143 subtests).
- [x] Targeted live check: instrumented the wired hook with a sentinel and ran
      `claude -p` — trace showed the hook FIRED, confirming headless claude
      executes the wired project `PreToolUse` hook. The "does headless claude
      run project hooks" risk is RESOLVED (no permissions grant needed).

## Outcome / accepted limitation

- Headless claude honors the wired hook (validated). The gate is now active in
  live scenarios.
- A diagnostic run showed the hook enforces **plan-before-production**, not
  **stop-after-planning**: when blocked, claude wrote a minimal plan folder and
  then proceeded to edit production. So `ac8`'s `forbidden src/**` assertion is
  not a clean test of the hook and stays partly advisory/flaky. The hook's real
  guarantee is covered deterministically by the 12 `test_plan_build_gate_hook`
  unit tests.
- Maintainer decision (2026-07-21): accept as-is. Not pursuing stream-json event
  assertions for `ac8` now. `requires_hook` still correctly scopes the scenario
  to hook-capable runtimes.
