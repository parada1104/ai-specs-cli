# ODD Feature: go-apply-orphans

## Objective

Move deletion of regenerable orphan recipe/dependency cache directories to the Go gate's `--apply-orphans` actuator while retaining Python's lock-file serialization and fail-open bridge.

## Scope and constraints

- Go receives already-resolved cache roots and orphan decision inputs; it must not create cache roots, alter arbitrary paths, or write `ai-specs/.ai-specs.lock`.
- Preserve the existing `clean_orphans` output, directory-only deletion, first-error stop semantics, and Python `remove_recipe_lock_entries` + `write_lock` behavior. Never replay a partially applied deletion blindly on an uncertain Go outcome; verify the failure contract in WU1 before bridging.
- Go plan remains authoritative for orphan names; Python retains the existing full legacy path only for safe acquisition/validation failures before Go applies anything.
- Exclude `update_recipe_config` TOML writer and unrelated copy/template/hook actuators.
- Worktree `.worktrees/go-apply-orphans`, branch `feat/go-apply-orphans`, base `development` at `94b6ac8`.
- Configured TDD: enabled by `ai-specs/ai-specs.toml` `[recipes.tdd-flow]`, exact runner `./tests/validate.sh`; record real RED, GREEN, and final full-suite results. Focused Go/Python tests may shorten feedback, not replace the configured runner.
- Delivery strategy: single PR for the change per prior user preference; no push, PR, or merge without explicit instruction. Forecast roughly 500–800 authored changed lines across Go actuator and Python bridge/tests (reassess actual count before commit); review candidate is each work-unit commit or PR slice under the current RDD switch.

## Tracker

- **card_id**: `6ab49a73125bec5f97cd7b45`
- **shortLink**: `csoAF8fa`
- **url**: https://trello.com/c/csoAF8fa/148-go-06-migrate-orphan-cache-deletion-actuator-to-go
- **list**: In Progress

## Tasks

- [x] **WU1 — Go cache deletion actuator.** Route: delegated writer (4-file mapping and multi-file write triggers). Test first; wire `--apply-orphans` to an input contract with resolved roots, compute or consume the Go orphan plan, delete only validated direct child directories, stop on first filesystem failure, return a structured outcome that distinguishes pre-apply failures from partial application. No lock writes. Edit surfaces: `catalog/recipes/worktree-flow/gate/orphans_apply.go`, `catalog/recipes/worktree-flow/gate/orphans_apply_test.go`, `catalog/recipes/worktree-flow/gate/main.go`, gate binary/digest outputs when needed. Checks: focused RED observed (`go test -run TestApplyOrphans .` undefined symbols); focused GREEN and parent rerun `go test -run 'TestApplyOrphans|TestPlanOrphans' .` PASS; `gofmt`, `go vet`, four-arch build/digest verification and full `./tests/validate.sh` PASS (2331 tests, 2 skipped). Configured full runner was not run at RED; RED evidence is focused only. Status: implemented and verified; work-unit commit follows this checkoff; native risk/outcome: pending.
- [ ] **WU2 — Python bridge and parity.** Route: delegated writer (multi-file write trigger). Test first; call verified Go actuator from `clean_orphans`, keep cache root acquisition, existing printed messages and Python lock serialization. Preserve full Python fallback only before side effects; partial/uncertain apply must fail closed without rerunning full legacy deletes. Edit surfaces: `lib/_internal/recipe-materialize.py`, `tests/test_materialize_bridge.py` (and `tests/test_external_dirs.py` only if necessary). Checks: RED/GREEN exact runner, focused bridge and external-dir suites, full `./tests/validate.sh`, parent spot check. Status: pending; commit: pending; native risk/outcome: pending.

## Progress

- 2026-09-24: read-only scout mapped `orphans_plan.go`, `recipe-materialize.py`, `lock.py` and parity tests. Selected dedicated worktree with user authorization; created GO-06 tracker card.
- WU1 writer added `orphans_apply.go`, tests, CLI flag and four-arch digest update. Outcome is `applied`, `pre_apply_failed` or `partial` with `removed`/`remaining`; exit 0/3 for structured results, 2 for malformed input. A failed `RemoveAll` is marked uncertain even on its first target. Python must fail closed on missing/invalid output after invocation. Worker observed full suite green; parent reran focused Go tests green. Full-suite RED was not run; preserve this TDD evidence gap.

## Acceptance

Go-primary and Python fallback produce the same successful removals, stdout and lock state; malformed inputs cannot delete outside the three intended cache roots; non-directory children are preserved; failure after a Go side effect cannot trigger replay; tests and digest verification pass.

## Next step

Commit verified WU1, assess the committed candidate under RDD, then start WU2.
