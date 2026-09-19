# Jinna recipe capability baseline

## Objective
Harden the Jinna OpenProject recipe as a standalone ai-specs product for consumer projects and define the shared provider-neutral capability baseline that future Tracker adapters must honor.

## Product boundary
- The catalog recipe is product source under `catalog/recipes/jinna-mcp-recipe/`.
- `ai-specs/ai-specs.toml` in this repository is dogfood consumer state, not the recipe contract.
- This change must not enable or bind Jinna in the repository's own dogfood manifest.
- Jinna-to-ledger lifecycle integration, OpenProject status mapping, and provider writes are a later adapter phase.

## Scope
- Align the capability documentation/schema/tests with the existing `vcs-pr-flow` pattern: shared capability id and provider-neutral baseline; concrete recipes provide adapters.
- Validate the standalone Jinna recipe through an isolated consumer fixture covering add, configure, sync, doctor, and MCP materialization.
- Document bounded behavior for slow or unavailable self-hosted OpenProject services; preserve explicit local/remote MCP boundaries and no hidden fallback/replay.
- Preserve the existing installer, managed-cache, checksum, environment-reference, and release-provenance contracts.

## Non-goals
- Adding `tracker` capability to Jinna in this phase.
- Changing the shared Go ledger or dogfood witness.
- Replacing Trello or implementing Jinna↔Trello synchronization.
- Requiring a live personal OpenProject instance for deterministic tests.

## Tracker

- **card_id**: `6aadc6cf0110b2427fdcd905`
- **url**: https://trello.com/c/4sMmC4vO/137-recipe-jinna-openproject-provider-standalone-cli-product-baseline
- **list**: In Progress

## Tasks

- [x] T1 — Define the provider-neutral capability baseline and add contract coverage using the existing VCS capability pattern.
- [x] T2 — Add or extend an isolated consumer fixture for the standalone Jinna recipe lifecycle.
- [x] T3 — Reconcile recipe docs/skill guidance for slow remote services without introducing hidden fallback behavior.
- [x] T4 — Run focused tests and `./tests/validate.sh`; record RED/GREEN and final evidence.
- [x] T5 — Add the recipe entrypoint parity matrix across CLI, wizard, Hub/TUI, sync, and doctor; normalize only proven divergent seams.

## Progress and evidence

- T1 complete. `docs/capabilities.md` now defines a provider-neutral capability baseline using `vcs-pr-flow` as the precedent: shared capability id and neutral config keys, adapter-specific assets, and no new manifest field.
- T1 RED: `python3 -m unittest tests.test_capability_baseline -v` failed with 1 failure and 3 errors because the baseline section was absent.
- T1 GREEN: the same focused suite passed with 13 tests. Adjacency suites passed with 93 tests across Playwright, ledger mode, Trello recipe, and catalog checks.
- No Jinna recipe or dogfood manifest files changed.
- T2 complete. `tests/test_jinna_consumer_recipe.py` runs the real CLI against a temporary consumer and temporary CLI home with a fake PATH provider; it covers init, recipe add, sync, doctor, materialized README/skill/MCP outputs, environment references, passive provider invocation, and dogfood-manifest immutability.
- T2 baseline GREEN: the new integration contract passed on first run; `tests.test_jinna_consumer_recipe` passed 1 test, the combined Jinna/baseline/recipe-add suite passed 38 tests, and the existing Jinna provider suite passed 66 tests with 2 skips.
- T3 complete. README and SKILL now state that sync/doctor are local-only, health/whoami are opt-in live diagnostics, the 30-second timeout covers local MCP startup/transport rather than the remote API SLA, and slow/unavailable service behavior is not hidden by retry/fallback/replay.
- T3 RED: `python3 -m unittest tests.test_jinna_runtime_boundaries -v` failed with 2 documentation failures before the boundary sections existed.
- T3 GREEN: the focused boundary suite passed 2 tests; combined Jinna/baseline/consumer checks passed 16 tests and the existing provider suite passed 66 tests with 2 skips.
- T4 complete. Focused product/baseline suite passed 82 tests with 2 expected opt-in release-smoke skips; adjacent schema/add/materialize/doctor/catalog suites passed 272 tests; `./tests/validate.sh` passed with exit 0; `git diff --check` passed.
- Final commit range: `495bcd2`, `bc279f9`, `71bd213`, plus this evidence update. The worktree remained clean after validation, and `ai-specs/ai-specs.toml` was unchanged.
- Follow-up T5 authorized: cover every recipe entry surface and converge them on the same domain behavior before treating the recipe product as interface-complete.
- T5 complete. `tests/test_recipe_entrypoint_parity.py` covers catalog list/init/add, non-interactive recipe configure inspect, sync, doctor, top-level help, configure-recipes help/dispatch, Hub help/dispatch, and Hub label/dispatch parity. The Hub remains a presenter delegating whole-project `configure-recipes`; no alternate domain path was introduced.
- T5 RED: the focused suite initially had 1 help failure and 1 missing-helper error for the unadvertised `recipe configure` and stale Hub label.
- T5 GREEN: focused parity suite passed 11 tests; Hub/TUI/Jinna/recipe init/list/configure adjacency passed 96 tests; harness/config wizard adjacency passed 33 tests; `./tests/validate.sh` passed with 2163 tests and 142 skips; `git diff --check` passed.

## Acceptance criteria

- The shared capability baseline is explicit and provider-neutral; it does not embed Trello or Jinna fields.
- A clean consumer fixture can materialize `jinna mcp` using environment references only, without mutating this repository's dogfood manifest.
- Missing, incompatible, slow, or unavailable provider paths remain explicit and non-destructive.
- Existing Jinna installer and materialization tests remain green.
- The later Jinna-to-ledger adapter boundary is documented without implementing it here.

## Delivery

- Worktree: `.worktrees/jinna-recipe-capability-baseline`
- Branch: `feat/jinna-recipe-capability-baseline`
- Test runner: `./tests/validate.sh`
- Strategy: one focused product change; do not enable Jinna in the canonical consumer project.
