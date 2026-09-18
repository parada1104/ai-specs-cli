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
- [ ] T4 — Run focused tests and `./tests/validate.sh`; record RED/GREEN and final evidence.

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
