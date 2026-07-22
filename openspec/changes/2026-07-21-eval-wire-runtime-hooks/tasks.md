# Tasks: wire runtime hooks in the live-eval harness

## Planning depth

- **Classification**: Standard (spec + tasks). Multi-file eval-infra change;
  clear scope, needs written requirements.
- **Authorization**: PENDING — plan presented, awaiting maintainer go.
- **Dependency**: stacked on `feat/plan-build-gate-hook` (#139), where `ac8`
  and the hook live. Merge order: #139 first, then this.

## Implementation (red-green-refactor)

- [ ] RED: non-live unit test — after `wire_runtime_hooks(root, "claude")`,
      `.claude/settings.json` contains a `PreToolUse` entry referencing
      `plan-build-gate`. And `wire_runtime_hooks(root, "cursor-agent")` does NOT
      add a file-write hook (documents the cursor limitation).
- [ ] GREEN: `wire_runtime_hooks(project_root, runtime)` in `harness.py`
      (runtime→agent map; materialize with `resolved_hooks_out`; call
      `hooks-render.py`).
- [ ] Call `wire_runtime_hooks` from `_run_scenario` after `setup_runtime_skills`.
- [ ] Add `requires_hook` scenario field; runner skips such scenarios on
      runtimes without a file-write hook event (cursor-agent).
- [ ] Set `requires_hook = true` in `ac8` scenario.toml.
- [ ] Spec delta in `openspec/specs/recipe-evals/spec.md`.

## Validation

- [ ] `./tests/validate.sh` exit 0; full `pytest tests/` green (non-live).
- [ ] One targeted live run: `ac8` on claude — confirm the gate blocks the
      production edit (resolves the "does headless claude run project hooks"
      risk). If headless claude needs a permissions grant, add it to the fixture
      settings and note it.

## Note

If the live check shows headless claude does not execute project hooks even when
wired, stop and report — the reinforcement's in-eval value depends on it, and
that finding changes the plan.
