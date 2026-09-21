# ODD Feature: materialization-sync-tag-conflicts

## Objective
Migrate the advisory `check_tag_conflicts` grader and its catalog acquisition seam from Python to the Go worktree-gate binary, preserving behavior while retaining a fail-open Python bridge and parity coverage.

## Problem and decision
Rank 3 of the Python-to-Go strangler is closing the materialization/sync domain. The next coherent seam is tag-conflict grading. `merge_config` was explored with this seam but is intentionally a separate follow-up: its `[config.*]` schema acquisition, structured-table validation, required fields, enums, and special `gate_impl` error path would make the combined candidate borderline or over the native review budget without shared logic.

## Scope
- Read top-level `[recipe]` `id`, `tags`, and `conflicts_with` from catalog `recipe.toml` files.
- Add a pure Go tag-conflict decision core and `--resolve-tag-conflicts` JSON command.
- Bridge `lib/_internal/recipe-materialize.py` through Go with a fail-open Python fallback.
- Preserve first-seen tag ordering, duplicate-id/self guard, one-sided symmetric fatal detection, advisory-only materialization behavior, and existing warning text.
- Add Go unit tests, Python bridge tests, parity tests, and regenerate `SHA256SUMS` with canonical Go 1.24.13.

## Non-goals
- `merge_config` migration; plan it as the next materialization/sync seam.
- Python primitive `check_conflicts` migration.
- Materialization write actuator, `render_override_bytes`, or `stamp_recipe_reconcile_defaults`.
- Informational advisory findings from PR #281 and prior changes.

## Constraints
- Base: `development` at `828ed8a`.
- Worktree: `.worktrees/materialization-sync-graders`.
- Branch: `feat/materialization-sync-graders`.
- Integration branch: `development`; delivery is PR-based and never a direct push to `development`.
- Native review candidates stay below roughly 1000 changed lines; choose one coherent PR unless the measured candidate exceeds that budget.
- TDD is enabled by project configuration. Runner: `./tests/validate.sh`.
- Generated technical artifacts remain in English.

## Tracker

- **card_id**: `6ab05972dda18707faf63cf3`
- **shortLink**: `YU1oMrpK`
- **url**: https://trello.com/c/YU1oMrpK/144-go-03-migrate-tag-conflict-grader-to-go
- **list**: In Progress

## Tasks

### T1 — Implement the pure Go tag-conflict seam
- **Route**: delegated direct writer; mapping trigger fired (four-file exploration), multi-file implementation.
- **Status**: done
- **Allowed behavior**: add the Go model/reader/core, CLI flag and JSON envelope, and focused Go tests. Preserve TOML recipe IDs rather than directory names and preserve first-seen tag order.
- **Acceptance**:
  - warning and fatal conflicts match Python semantics;
  - duplicate IDs and repeated self-tags do not produce conflicts;
  - one-sided `conflicts_with` declarations produce fatal severity;
  - missing recipe files remain skipped at acquisition;
  - command output is deterministic JSON and does not change exit behavior for advisory conflicts.
- **Checks**: RED focused Go test(s), GREEN `go test ./...` in `catalog/recipes/worktree-flow/gate`, then parent spot check.
- **Worker evidence**: RED focused tests failed on missing planner/acquisition/JSON behavior; GREEN focused and full gate tests passed, including a real-catalog Python parity run.
- **Implementation note**: Go emits `severity` in the tag-conflict envelope so the bridge can preserve warning versus fatal message text; Python `TagConflict.to_dict()` does not include that field.
- **Parent verification**: PASS — independent `go test -count=1 ./...`; contract and scope checks found no blockers. RED evidence remains worker-reported; T2 owns Python parity.
- **Native review**: lineage `review-107276927d827f12`; candidate measured 601 changed lines from `origin/development`; approved 4/4 lenses; acknowledgement completed and authority burned. Five informational findings recorded as non-blocking and deliberately not turned into scope or fixes: `R3-dup-id-coverage`, `R3-flag-exclusive`, `R3-path-join`, `R3-silent-omit` (WARNING but informational), `R3-unstripped-tags`.

### T2 — Add the fail-open Python bridge and parity coverage
- **Route**: delegated direct writer; multi-file bridge/test change.
- **Status**: in progress
- **Allowed behavior**: bridge the existing wrapper/call path with `GO_TAG_CONFLICTS_BRIDGE_FALLBACK`, keep the Python core as fallback, preserve warning/fatal message contracts, and add `_forbid_python_authority` parity coverage.
- **Acceptance**:
  - the verified gate binary is used when available;
  - acquisition or Go failures warn and fall back without breaking materialization;
  - Go and Python conflict shapes and ordering agree;
  - advisory tag conflicts never alter the materialization exit code.
- **Checks**: RED bridge/parity test(s), GREEN `./tests/validate.sh` plus built gate binary, then parent spot check.
- **Worker evidence**: RED bridge tests failed with the missing fallback symbol; GREEN bridge and existing materialization tests passed.
- **Post-digest verification**: after rebuilding the gate binary and regenerating `SHA256SUMS`, `shasum` matched all four `SHA256SUMS` lines. `python3 -m unittest tests.test_tag_conflict_bridge` ran 9 tests with no skips against the digest-verified default binary. `python3 -m unittest tests.test_recipe_materialize tests.test_materialize_bridge` ran 84 tests OK.
- **Native review attempt**: explicit base-ref candidate from `c024707fd4df7b941b854dfacae837f7529a1bb0` measured 496 changed lines. START returned the known stale/wide consent-binding-expired envelope twice despite fresh inspect+START sequences and created no lineage, so native review was declared unavailable — never recorded as approved. `gentle_review assess` with `nativeReviewOutcome: unavailable` returned high risk and required independent verification; that independent verifier already passed the post-digest 9-test bridge suite and the 84-test materialization suite.
- **Task closure**: functional verification is complete but the task stays `in progress`; the canonical `./tests/validate.sh` command did not complete (see Blockers).
- **Implementation note**: the Go envelope maps back onto `TagConflict` so the existing warning loop remains unchanged; degraded runs emit one `GO_TAG_CONFLICTS_BRIDGE_FALLBACK` warning and retain the Python authority.

### T3 — Normalize artifacts and close the work unit
- **Route**: parent orchestration with delegated verification as required by the verification trigger.
- **Acceptance**:
  - `SHA256SUMS` is regenerated with canonical Go 1.24.13 and the rationale is recorded;
  - full validation is green;
  - changed files are limited to the authorized surfaces;
  - native review is run on the work-unit candidate if the user-owned RDD switch requires it;
  - one Conventional Commit records the completed work unit and its evidence.
- **Checks**: `scripts/build-gate.sh`, digest verification, `./tests/validate.sh`, status/diff review, native risk assessment/review path.
- **Status**: pending

## Blockers

### B1 — `./tests/validate.sh` does not complete (pre-existing)
- `./tests/validate.sh` timed out after 600s inside the unittest subprocess-spawning sync tests. The preceding static `py_compile`, `bash -n`, and `gofmt` phases passed.
- A clean `HEAD` snapshot reproduced both the hang and six unrelated runtime-brief ownership failures, classifying them as pre-existing and outside this change.
- Impact: this blocks any claim of full-suite green. It must not be silently treated as passed, and T2/T3 cannot close on a green-suite claim while B1 is open.

## Delivery strategy
- **Strategy**: one PR with two work-unit review slices — T1 at 601 changed lines and T2 at 496 changed lines. Each slice stays under the roughly 1000 native candidate budget; the aggregate branch is 1089 changed lines and must never be reviewed as a single candidate.
- **Forecast**: approximately 400–650 authored changed lines for the tag-conflict seam alone, excluding generated binaries/digests.
- **Current progress**: T1 is committed as `c024707` (Go tag-conflict planner); T2 is committed as `dee435d` (fail-open Python bridge plus regenerated `SHA256SUMS`). T1 native review is approved and acknowledged (601 lines, 4/4 lenses). T2 functional verification and independent high-risk verification are complete (post-digest 9-test bridge suite, 84-test materialization suite). T2 native review could not start and is recorded as unavailable, not approved. Full validation is still blocked by B1 (pre-existing).

## Verification evidence
- **Exploration**: `check_tag_conflicts` is advisory-only; its Python core is pure and preserves first-seen tag order. Go currently lacks top-level recipe tag/conflict acquisition.
- **Scope decision**: `merge_config` deferred because it needs a richer `[config.*]` schema acquisition/validation surface and shares no decision logic with tag conflicts.
- **RED evidence**: T1 focused Go tests failed before implementation with missing planner/acquisition/JSON behavior; T2 bridge tests failed before implementation with missing bridge symbols.
- **GREEN evidence**: T1 focused and full Go gate tests passed in the worker; independent `go test -count=1 ./...` passed. T2 focused bridge and existing materialization tests passed; post-digest `shasum` matched all four `SHA256SUMS` lines, `python3 -m unittest tests.test_tag_conflict_bridge` ran 9 tests with no skips against the digest-verified default binary, and `python3 -m unittest tests.test_recipe_materialize tests.test_materialize_bridge` ran 84 tests OK. Full validation is blocked by B1 (pre-existing) and is not claimed green.
- **Native review evidence**: T1 lineage `review-107276927d827f12` approved 4/4 lenses on a 601-line candidate and its authority is burned. T2's explicit `c024707fd4df7b941b854dfacae837f7529a1bb0` base-ref candidate (496 lines) never produced a lineage: START re-offered the stale/wide consent-binding-expired envelope twice after fresh inspect+START, so native review is unavailable. T2 was not approved by inference; `gentle_review assess` with `nativeReviewOutcome: unavailable` returned high risk and required independent verification, which already passed the post-digest 9-test bridge suite and the 84-test materialization suite.
- **Final evidence**: pending T3.

## Next step
T2 functional verification and the independent high-risk verification are complete, so the remaining functional gate is B1: T2/T3 cannot claim full-suite green or closure while `./tests/validate.sh` still fails on the pre-existing timeout and unrelated runtime-brief ownership failures. Native T2 review is unavailable but is not silently treated as approved, and delivery stays blocked by B1 plus ordinary PR policy. Do not close T2 or proceed to T3's green-suite claim until B1 is resolved or explicitly waived.
