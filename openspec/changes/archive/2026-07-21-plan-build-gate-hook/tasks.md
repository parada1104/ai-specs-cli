# Tasks: plan-build-flow pre-tool-use artifact gate

## Planning depth

- **Classification**: Standard (spec + tasks). Additive enforcement on an
  existing recipe; mirrors the worktree-gate precedent.
- **Authorization**: plan presented and approved by maintainer (session 2026-07-21).

## Implementation (red-green-refactor)

- [x] RED: `tests/test_plan_build_gate_hook.py` — exit-code contract:
      - block (exit 2) Write to `src/**` when no change folder exists
      - allow (exit 0) once `openspec/changes/<slug>/tasks.md` exists
      - allow (exit 0) edits under `openspec/changes/**` (writing the plan)
      - allow (exit 0) non-production paths (tests, docs)
      - allow (exit 0) gitignored agent config on production trees
      - fail-open (exit 0) on malformed stdin / missing file_path
      - non-bypassable: no on/off/ask mode (mode env ignored)
      - `PLAN_BUILD_GATE_PATHS` scope override honored
- [x] GREEN: `catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh`.
- [x] Wire `[[provides.hooks]]` in `catalog/recipes/plan-build-flow/recipe.toml`.
- [x] Eval: `ac8_approval_verb_without_folder/` scenario.toml + prompt.txt.
- [x] Register `ac8` in `LIVE_SCENARIOS` + `test_` method in
      `tests/evals/eval_plan_build_flow_live.py`.
- [x] Spec delta: new requirement in `openspec/specs/plan-build-flow/spec.md`.

## Validation

- [x] `./tests/validate.sh` passes (exit 0); full `pytest tests/` green.
- [x] New hook unit tests pass.
