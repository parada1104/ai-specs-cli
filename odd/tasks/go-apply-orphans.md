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
- Delivery strategy: one PR for the change was the prior preference, but the actual `development...HEAD` diff is 1,238 authored changed lines (1,193 additions + 45 deletions) across 7 paths. This exceeds both the ~400-line workload trigger and the prior 1,200-line PR budget by 38; a new explicit delivery-size decision is required before PR preparation. No push, PR, or merge without instruction. Native review candidates were the individual work-unit commits, not the accumulated branch.

## Tracker

- **card_id**: `6ab49a73125bec5f97cd7b45`
- **shortLink**: `csoAF8fa`
- **url**: https://trello.com/c/csoAF8fa/148-go-06-migrate-orphan-cache-deletion-actuator-to-go
- **list**: In Progress

## Tasks

- [x] **WU1 — Go cache deletion actuator.** Route: delegated writer (4-file mapping and multi-file write triggers). Test first; wire `--apply-orphans` to an input contract with resolved roots, compute or consume the Go orphan plan, delete only validated direct child directories, stop on first filesystem failure, return a structured outcome that distinguishes pre-apply failures from partial application. No lock writes. Edit surfaces: `catalog/recipes/worktree-flow/gate/orphans_apply.go`, `catalog/recipes/worktree-flow/gate/orphans_apply_test.go`, `catalog/recipes/worktree-flow/gate/main.go`, gate binary/digest outputs when needed. Checks: focused RED observed (`go test -run TestApplyOrphans .` undefined symbols); focused GREEN and parent rerun `go test -run 'TestApplyOrphans|TestPlanOrphans' .` PASS; `gofmt`, `go vet`, four-arch build/digest verification and full `./tests/validate.sh` PASS (2331 tests, 2 skipped). Configured full runner was not run at RED; RED evidence is focused only. Status: committed `b4bc8b17aa92651b76a65016d64813276beefc38`, verified; ASSESS high, native review unavailable (risk lens `stopReason:length` twice), lineage `review-1a68992d6c9770b1` remains open with no verdict or approval. Independent verifier: focused Go tests and vet PASS, digests 4/4 match; parent focused test PASS.
- [x] **WU2 — Python bridge and parity.** Route: delegated writer (multi-file write trigger). Test first; call verified Go actuator from `clean_orphans`, keep cache root acquisition, existing printed messages and Python lock serialization. Preserve full Python fallback only before side effects; partial/uncertain apply must fail closed without rerunning full legacy deletes. Edit surfaces: `lib/_internal/recipe-materialize.py`, `tests/test_materialize_bridge.py` (and `tests/test_external_dirs.py` only if necessary). Checks: RED focused suite failed (7 failures/2 errors); RED `./tests/validate.sh` exit 1 (same new-test failures). GREEN focused bridge/external-dir 75 tests PASS (parent rerun PASS); GREEN `./tests/validate.sh` exit 0, 2341 tests OK (2 skipped), `git diff --check` clean. Status: initial commit `5f7185d6ce08b01b22f0178092b3036005106a21`, bounded correction commit `e10d94f015afbd6ee29d9b2ee0edb81dbdbd6ffb`, verified. Initial native review-risk returned `stopReason:length` with no verdict; lineage `review-1c870cd9168e7590` open. Independent verifier's applied+uncertain/nonempty-remaining and invalid UTF-8 findings corrected: focused RED 25 tests, 2 failures/1 error; GREEN 25 OK; full `./tests/validate.sh` GREEN 2344 OK (2 skipped); parent reran 25 OK. Narrow correction-only native lineage `review-1b6334f33b0fc016`: 4/4 lenses, approved and acknowledged/burned; three informational advisories, no blockers. This approval covers only the correction, not the two earlier work units.

## Progress

- 2026-09-24: read-only scout mapped `orphans_plan.go`, `recipe-materialize.py`, `lock.py` and parity tests. Selected dedicated worktree with user authorization; created GO-06 tracker card.
- WU1 writer added `orphans_apply.go`, tests, CLI flag and four-arch digest update. Outcome is `applied`, `pre_apply_failed` or `partial` with `removed`/`remaining`; exit 0/3 for structured results, 2 for malformed input. A failed `RemoveAll` is marked uncertain even on its first target. Python must fail closed on missing/invalid output after invocation. Worker observed full suite green; parent reran focused Go tests green. Full-suite RED was not run; preserve this TDD evidence gap.
- WU1 committed `b4bc8b1` (605 changed lines). Native ASSESS high; exact scoped START created `review-1a68992d6c9770b1`, but review-risk produced no text (`stopReason:length`) twice, including one fresh-STATUS retry. No verdict/approval, lineage open; do not replay unchanged. ASSESS with `nativeReviewOutcome=unavailable` required independent verification: separate verifier found no blocking issue, reran Go orphan tests and vet, confirmed digests. Its low-severity note: pre-apply means no completed removal, not literally no operations; WU2 must distinguish a proven structured pre-apply failure from absent/malformed response. Explicit maintainer disposition is needed before merge.
- WU2 writer bridged `clean_orphans` to the verified `--apply-orphans` binary, retained Python lock serialization/output, and added 10 bridge cases. Test-first full-suite RED observed (2341 tests, 7 failures/2 errors confined to new cases); GREEN full suite 2341 OK (2 skipped). Parent focused rerun 75 tests OK. Safe fallback is limited to pre-invocation unavailability or clean structured pre-apply without uncertain entries; timeout/malformed/partial/uncertain fail closed without lock prune. No Go or digest changes.
- WU2 committed `5f7185d` (579 lines); native high-risk review `review-1c870cd9168e7590` stalled on review-risk `stopReason:length` without verdict. ASSESS(unavailable) required separate verifier. Verifier reran 75 tests and diff check green; flagged that `applied`+uncertain or remaining was wrongly accepted for lock prune, and undecodable subprocess stdout escaped uncaught. Bounded correction commit `e10d94f` added three fail-closed tests: focused RED 25 tests with 2 failures/1 error, focused GREEN 25 OK, final full suite 2344 OK (2 skipped), diff check clean, parent focused 25 OK. Correction-only lineage `review-1b6334f33b0fc016` approved 4/4 and acknowledgement burned; nonblocking advisories: R2-test-lock-assert-gap, R3-outcome-key-absence-unproved, R3-undecodable-fixture-escape-portability. WU1/WU2 original lineages remain open/unapproved.

## Acceptance

Go-primary and Python fallback produce the same successful removals, stdout and lock state; malformed inputs cannot delete outside the three intended cache roots; non-directory children are preserved; failure after a Go side effect cannot trigger replay; tests and digest verification pass.

## Next step

Implementation and verification are complete locally. Before PR preparation, resolve the 1,238-line delivery-size decision and obtain explicit maintainer disposition for WU1 lineage `review-1a68992d6c9770b1` and WU2 lineage `review-1c870cd9168e7590` (both open, no verdict); do not infer approval from the correction-only receipt. No push or PR has been authorized.
