# Apply Progress: plan-build-phase-compatibility-direct

## Scope

Direct, documentation-and-contract implementation for Trello card #82. The
paused SDD change is read-only context and is not modified.

## TDD Evidence

### RED

- `./tests/run.sh` — run once with the bounded 120-second timeout; the suite did
  not reach a final summary before timeout and emitted failures from the new
  contract assertions.
- Isolated confirmation:
  `python3 -m unittest tests.test_plan_build_flow_recipe.PlanBuildFlowRecipeTests.test_full_phase_contract_maps_dependencies_and_fallbacks tests.test_plan_build_flow_recipe.PlanBuildFlowRecipeTests.test_preflight_and_presentation_contracts_are_composed tests.test_plan_build_flow_recipe.PlanBuildFlowRecipeTests.test_standard_and_light_remain_collapsed tests.test_plan_build_flow_recipe.PlanBuildFlowRecipeTests.test_phase_contract_stays_out_of_recipe_config_and_named_vocabulary`
  — 4 tests, 28 assertion failures because the new phase, preflight, and
  presentation contract text was not yet present.

### GREEN

- `python3 -m unittest tests.test_plan_build_flow_recipe` — `Ran 34 tests in
  0.187s`, `OK`.
- The focused recipe suite covers the new phase mapping/fallback, preflight
  composition, presentation fields and decisions, Standard/Light preservation,
  vocabulary, version, materialization, and existing gate contracts.

### Judgment Day fix round (`JD-DIRECT-RECIPE-TEST`)

- The candidate's surface-exclusion test defined a dead variable
  `removed_phrase = "review " + "budget"` that encoded a generic negative
  contract ("the phrase 'review budget' must be absent"). The new preflight
  contract intentionally introduces `review budget` as a preflight field
  (asserted positively in `test_preflight_and_presentation_contracts_are_composed`),
  so that dead generic negative was a deterministic contradiction.
- Fix: removed the dead `removed_phrase` variable and documented the intent in a
  comment. The narrow retired-section-marker negatives that already existed
  (`^#{1,6} 7.5`, `^#{1,6} Review workload budget`, `^\s*WARN: review budget`)
  are preserved — the recipe still must not expose the retired review-budget
  session-control section/heading, while it may and must mention the preflight
  field `review budget`. No production code changed.

### Final validation

- `./tests/run.sh` — exit `0`; `Ran 1685 tests`, `OK (skipped=116)`.
- `./tests/validate.sh` — exit `0`; syntax checks and the same `1685` tests
  passed with `116` skips.
- Post-fix re-run: `python3 -m unittest tests.test_plan_build_flow_recipe` —
  `Ran 34 tests`, `OK`.
- `git diff --check` — exit `0`.
- Authored change budget: below the cached 1200-line limit; no commit, push, or
  PR was performed.

## Implementation Status

- [x] Direct Standard planning trail created with Trello card #82.
- [x] Contract tests written before recipe/docs edits.
- [x] Update recipe skill, README, brief, catalog docs, changelog, and evidence.
- [x] Run focused GREEN and final validation.
