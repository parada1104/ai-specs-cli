# Worktree gate brief reconciliation

## Objective
Make the generated runtime brief faithfully reflect `[recipes.worktree-flow.config].gate_mode` and the existing ask/always/off gate contract.

## Problem
The project `[brief].runtime_flow` and the worktree-flow recipe both emit unconditional "dedicated worktree" prose. This contradicts `gate_mode = ask`, whose contract requires the user to choose among a dedicated worktree, a feature branch in the current checkout, or an explicit protected-branch override.

## Scope
- Remove the dogfood project's unconditional runtime-flow worktree statement.
- Replace the recipe's unconditional brief fragment with a config-aware fragment using the resolved `gate_mode` value.
- Update worktree-flow README wording so ask mode describes user-mediated destination selection, not a self-bypass hint.
- Add regression coverage proving generated/materialized brief text is coherent for `always`, `ask`, and `off`, while preserving the runtime hook's stamped mode behavior.
- Regenerate only the worktree's generated AGENTS/recipe output through sync; do not commit generated consumer state as product source.

## Constraints
- Preserve the existing Go/bash gate behavior and golden/live ask evals.
- Do not advertise `WORKTREE_GATE_MODE=off` as a self-bypass for ask mode.
- Keep recipe configuration as the source of the effective mode; no runtime manifest lookup.
- Do not touch the unrelated dirty ledger follow-up worktree.
- TDD mode: enabled; focused runner: `python3 -m unittest tests.test_worktree_flow_recipe tests.test_agents_render_brief_fragments tests.test_sync_pipeline`.

## Tracker

- **card_id**: `AImzsLWw`
- **url**: https://trello.com/c/AImzsLWw/37-feat-worktree-flow-recipe-modes-always-ask-off
- **note**: Follow-up to the shipped worktree-flow gate-mode implementation.

## Tasks

- [x] T1 — Add RED coverage for config-aware brief generation and remove duplicate unconditional source text. RED: 8 new assertions failed before implementation.
- [x] T2 — Implement recipe/project brief and README reconciliation, then verify generated output for all modes. GREEN: focused suite passed (180 tests).
- [x] T3 — Run isolated/temp sync coverage, full validation, and update evidence. Live dogfood sync is intentionally deferred until the authored worktree is clean, per the isolation policy.

## Acceptance criteria

- `gate_mode = always` brief requires a dedicated worktree for protected writes.
- `gate_mode = ask` brief requires asking the user to choose a destination and does not claim worktree is mandatory.
- `gate_mode = off` brief permits work where the user directs it.
- No unconditional dedicated-worktree rule remains in the generated brief when ask/off is selected.
- Focused tests and `./tests/validate.sh` pass.

## Progress

- Worktree: `.worktrees/worktree-gate-brief-sync`
- Branch: `fix/worktree-gate-brief-sync`
- Base: `development` at `03e227c`
- Current state: source, focused tests, isolated temp sync, and full validation complete; generated live dogfood state remains intentionally untouched.
- Evidence: focused suite — 180 tests OK; full `./tests/validate.sh` — 2118 tests OK, 142 skipped.
- Verification: independent verifier confirmed no runtime hook changes and correct always/ask/off rendering; end-to-end temp sync proved ask-mode prose reaches generated AGENTS.md.

## Next step
Close the work unit through the authorized commit/PR flow; do not hand-edit generated AGENTS.md.
