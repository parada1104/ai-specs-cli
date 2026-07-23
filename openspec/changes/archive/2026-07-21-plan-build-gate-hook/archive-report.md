# Archive report — plan-build-gate-hook

**Archived:** 2026-07-21
**Branch:** feat/plan-build-gate-hook
**PR:** #139
**Status:** ready-to-merge

## Outcome

- `plan-build-flow` now ships `hooks/plan-build-gate.sh`, a `pre-tool-use`
  blocking hook that machine-enforces the plan-before-build artifact
  precondition: production edits (`src`/`lib`/`catalog`, override
  `PLAN_BUILD_GATE_PATHS`) are blocked (exit 2) until an active change folder
  (`openspec/changes/*/tasks.md` outside `archive/`) exists.
- The gate is **non-bypassable**: no on/off/ask mode. Writing the plan is the
  only way past it. Writing the plan, non-production paths, and gitignored agent
  config are always allowed; fail-open on parse errors.
- Runtime reach: wired by `hooks-render.py` for `claude`, `opencode`, `pi`,
  `omp`; `cursor` has no pre-file-write hook event, so it keeps the advisory
  skill + workflow-rules layer.
- TDD evidence: 11 exit-code-contract unit tests (`test_plan_build_gate_hook.py`)
  went RED → GREEN, including a portability fix (realpath both sides so a
  symlinked prefix like macOS `/tmp`→`/private/tmp` cannot defeat the repo-root
  strip) and a non-bypass regression test.
- Eval: `ac8_approval_verb_without_folder` added and registered — covers the
  exact regression (approval verb, no prior change folder, must still plan).

## Files changed

- `catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh` — new hook.
- `catalog/recipes/plan-build-flow/recipe.toml` — `[[provides.hooks]]`, version 1.1.1 → 1.2.0.
- `tests/test_plan_build_gate_hook.py` — 11 unit tests.
- `tests/evals/scenarios/plan-build-flow/ac8_approval_verb_without_folder/` — new scenario.
- `tests/evals/eval_plan_build_flow_live.py` — register ac8.
- `openspec/specs/plan-build-flow/spec.md` — new requirement (AC14) promoted from delta.

## Verification

- `./tests/validate.sh` — exit 0.
- Full `pytest tests/` — 1008 passed, 143 subtests passed.
- Live evals (cursor-agent, claude-code/sonnet-5) — see PR thread.

## Process note

Went through the corrected plan-build sequence: classified Standard, plan
presented and approved, implemented, then refined (dropped the on/off/ask mode
per maintainer feedback that a switch defeats a non-bypassable gate).
