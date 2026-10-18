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
- **Status**: done
- **Allowed behavior**: bridge the existing wrapper/call path with `GO_TAG_CONFLICTS_BRIDGE_FALLBACK`, keep the Python core as fallback, preserve warning/fatal message contracts, and add `_forbid_python_authority` parity coverage.
- **Acceptance**:
  - the verified gate binary is used when available;
  - acquisition or Go failures warn and fall back without breaking materialization;
  - Go and Python conflict shapes and ordering agree;
  - advisory tag conflicts never alter the materialization exit code.
- **Checks**: RED bridge/parity test(s), GREEN `./tests/validate.sh` plus built gate binary, then parent spot check.
- **Worker evidence**: RED bridge tests failed with the missing fallback symbol; GREEN bridge and existing materialization tests passed.
- **Post-digest verification**: after rebuilding the gate binary and regenerating `SHA256SUMS`, `shasum` matched all four `SHA256SUMS` lines. `python3 -m unittest tests.test_tag_conflict_bridge` ran 9 tests with no skips against the digest-verified default binary. `python3 -m unittest tests.test_recipe_materialize tests.test_materialize_bridge` ran 84 tests OK.
- **Native review**: lineage `review-655fe6bad4a86a63`; candidate measured 516 changed lines from the explicit base `origin/feat/materialization-sync-tag-conflicts-t1` / base tree `bd4a771efd0a80f11b307f523f7a9f79bd5de507`; approved 4/4 lenses; acknowledgement completed and authority burned. Approval was reached after one reviewer-risk admission retry; no approval was ever inferred from an unavailable or partial run. Four informational findings recorded as non-blocking and deliberately not turned into scope or fixes: `R3-envelope-field-validation` (WARNING but informational), `R3-fallback-stderr-noise`, `R3-timeout-path-unproved`, `R4-malformed-envelope-crash`.
- **Task closure**: functional verification, native review, digest verification, and full canonical validation are complete; T2 is `done`.
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
- **Status**: done
- **Worker evidence**: `SHA256SUMS` regenerated with canonical Go 1.24.13 and digest-verified; the full canonical suite ran green under the 900-second bound (2263 tests, `OK (skipped=2)`, zero failures/errors, exit 0); changed files remain within the authorized surfaces; the work-unit evidence commit `54d2336` records the closure.
- **Parent verification**: PASS — full validation exit 0 at 689 seconds wall clock; the alarm did not fire and no processes were killed.

## Blockers

### B1 — `./tests/validate.sh` completion (resolved)
- The earlier attempt timed out after 600s inside the unittest subprocess-spawning sync tests; the preceding static `py_compile`, `bash -n`, and `gofmt` phases passed. A clean `HEAD` snapshot reproduced both the hang and six unrelated runtime-brief ownership failures, classifying them as pre-existing and outside this change.
- The 600s timeout was the binding constraint, not the suite. Re-running the canonical suite under a macOS-compatible 900-second bound resolved it:
  - Command: `perl -e '$SIG{ALRM}=sub{exit 142}; alarm 900; exec @ARGV' ./tests/validate.sh`
  - Exit 0; 689 seconds wall clock; the unittest timer reported 687.083s.
  - 2263 tests ran with `OK (skipped=2)`, zero failures and zero errors.
  - The alarm did not fire; no processes were killed.
  - The worktree was clean before and after the run.
- Impact: B1 is closed and the full suite is green. No failure was waived, hidden, or treated as passed.

## Delivery strategy
- **Strategy**: PR1 first, then a rebased replacement PR for the PR2 slice (PR1 #282 merged; PR2 replacement #284 open).
  - **PR1** — T1 commit `c024707` against `development`. PR1 was the chain base and targeted `development` directly; it is now merged as `3542417`.
  - **PR2** — T2 commit `dee435d` plus T3 evidence commit `54d2336`, originally based on PR1/T1 rather than on `development`. The original PR2 #283 could not be retargeted because the required force-with-lease was safety-blocked, so it was closed as superseded. The replacement PR2 is #284 on head branch `feat/materialization-sync-graders-final` targeting `development`; it preserves the 516-line approved PR2 candidate and the same reviewed T1 base tree `bd4a771efd0a80f11b307f523f7a9f79bd5de507`.
- **Chained-PR size note**: the two-PR split exceeds the 400-line chained-pr guideline; the user explicitly accepted the chained-pr `size:exception` for this split. The budget constrains how work is sliced only and never justifies compressing or deleting code, tests, or docs.
- **Native candidate budget**: PR1's reviewed candidate was 601 changed lines (approved; now merged as `3542417`). PR2's reviewed native base-ref is the explicit `origin/feat/materialization-sync-tag-conflicts-t1` / base tree `bd4a771efd0a80f11b307f523f7a9f79bd5de507` (the T1 commit `c024707` boundary), giving a PR2 candidate of 516 changed lines (approved). The aggregate branch is 1089 changed lines and must never be reviewed as a single candidate.
- **Open-without-merge**: PR1 #282 is merged as `3542417`. Original PR2 #283 was closed as superseded because its required force-with-lease was safety-blocked; the replacement PR2 #284 is open and unmerged on branch `feat/materialization-sync-graders-final` targeting `development`. Both native reviews (T1 and T2) are approved and acknowledged, and B1 is closed by a green full validation. Delivery still follows ordinary PR policy: replacement PR2 #284 stays unmerged until the user gives an explicit merge instruction.
- **Forecast**: approximately 400–650 authored changed lines for the tag-conflict seam alone, excluding generated binaries/digests.
- **Current progress**: T1, T2, and T3 are `done`; B1 is closed. PR1 #282 is merged as `3542417`. Original PR2 #283 is closed as superseded (force-with-lease was safety-blocked); replacement PR2 #284 is open and unmerged on head branch `feat/materialization-sync-graders-final` targeting `development`, preserving the 516-line approved PR2 candidate and the same reviewed T1 base tree `bd4a771efd0a80f11b307f523f7a9f79bd5de507`. T1 is committed as `c024707` (Go tag-conflict planner); T2 is committed as `dee435d` (fail-open Python bridge plus regenerated `SHA256SUMS`); T3 evidence is committed as `54d2336`. T1 native review is approved and acknowledged (601 lines, 4/4 lenses, lineage `review-107276927d827f12`). T2 native review is approved and acknowledged (516 lines, 4/4 lenses, lineage `review-655fe6bad4a86a63`) with authority burned; T2 functional verification is also complete (post-digest 9-test bridge suite, 84-test materialization suite). Full canonical validation is green (exit 0, 689 seconds wall clock, 2263 tests, `OK (skipped=2)`). Delivery still follows ordinary PR policy; replacement PR2 #284 remains unmerged awaiting explicit merge.

### Chain Context
```
PR1 #282: T1 Go tag-conflict planner (c024707) ........ merged as 3542417
  └── PR2 #283: T2 bridge + T3 evidence ........ closed as superseded (force-with-lease safety-blocked)
       └── PR2 replacement #284: T2 (dee435d) + T3 evidence (54d2336)
             branch feat/materialization-sync-graders-final ...... targets development 📍
```
- **PR numbers**: PR1 = #282 (https://github.com/parada1104/ai-specs-cli/pull/282, merged as `3542417`); original PR2 = #283 (https://github.com/parada1104/ai-specs-cli/pull/283, closed as superseded); replacement PR2 = #284 (https://github.com/parada1104/ai-specs-cli/pull/284, open and unmerged).
- **PR2 native base-ref**: the reviewed PR2 candidate used the explicit base `origin/feat/materialization-sync-tag-conflicts-t1` / base tree `bd4a771efd0a80f11b307f523f7a9f79bd5de507` (516 changed lines), never the aggregate 1089-line branch. The replacement PR2 #284 preserves that same 516-line approved candidate and the same reviewed T1 base tree.
- **Prior dependency**: PR1 #282 is merged; the replacement PR2 #284 preserves the original PR2 content and targets `development`.
- **Out of scope / documented gaps**: none outstanding — B1 is closed by the green full validation under the 900-second bound. Both native reviews (T1 and T2) are approved and acknowledged. PR1 #282 is merged; replacement PR2 #284 remains open and unmerged pending the user's explicit merge instruction.

## Verification evidence
- **Exploration**: `check_tag_conflicts` is advisory-only; its Python core is pure and preserves first-seen tag order. Go currently lacks top-level recipe tag/conflict acquisition.
- **Scope decision**: `merge_config` deferred because it needs a richer `[config.*]` schema acquisition/validation surface and shares no decision logic with tag conflicts.
- **RED evidence**: T1 focused Go tests failed before implementation with missing planner/acquisition/JSON behavior; T2 bridge tests failed before implementation with missing bridge symbols.
- **GREEN evidence**: T1 focused and full Go gate tests passed in the worker; independent `go test -count=1 ./...` passed. T2 focused bridge and existing materialization tests passed; post-digest `shasum` matched all four `SHA256SUMS` lines, `python3 -m unittest tests.test_tag_conflict_bridge` ran 9 tests with no skips against the digest-verified default binary, and `python3 -m unittest tests.test_recipe_materialize tests.test_materialize_bridge` ran 84 tests OK. The full canonical suite ran green under the 900-second bound (2263 tests, `OK (skipped=2)`, zero failures/errors, exit 0).
- **Native review evidence**: T1 lineage `review-107276927d827f12` approved 4/4 lenses on a 601-line candidate and its authority is burned. T2 lineage `review-655fe6bad4a86a63` approved 4/4 lenses on a 516-line candidate from explicit base `origin/feat/materialization-sync-tag-conflicts-t1` / base tree `bd4a771efd0a80f11b307f523f7a9f79bd5de507`; acknowledgement completed and authority burned after one reviewer-risk admission retry. T2's four informational findings (`R3-envelope-field-validation` WARNING but informational, `R3-fallback-stderr-noise`, `R3-timeout-path-unproved`, `R4-malformed-envelope-crash`) are recorded as non-blocking and add no scope or fixes. Independent functional verification also passed the post-digest 9-test bridge suite and the 84-test materialization suite. No approval was inferred from any partial run.
- **Final evidence**: complete. `perl -e '$SIG{ALRM}=sub{exit 142}; alarm 900; exec @ARGV' ./tests/validate.sh` exited 0 at 689 seconds wall clock with a 687.083s unittest timer, 2263 tests, `OK (skipped=2)`, and zero failures/errors; the alarm did not fire and no processes were killed; the worktree was clean before and after. Native reviews for T1 and T2 are approved and acknowledged with burned authority. PR1 #282 is merged as `3542417`; original PR2 #283 is closed as superseded; replacement PR2 #284 is open and unmerged.

## Next step
B1 is closed and T1/T2/T3 are `done`. PR1 #282 is merged as `3542417`. Original PR2 #283 is closed as superseded because its force-with-lease was safety-blocked. The replacement PR2 #284 (head branch `feat/materialization-sync-graders-final`, base `development`) is open and unmerged; it preserves the 516-line approved PR2 candidate (T2 `dee435d` plus T3 evidence `54d2336`) and the same reviewed T1 base tree `bd4a771efd0a80f11b307f523f7a9f79bd5de507`. T1 and T2 native reviews are both approved and acknowledged with burned authority (lineages `review-107276927d827f12` and `review-655fe6bad4a86a63`). Next step: await PR review and an explicit merge instruction for #284; delivery follows ordinary PR policy with no direct push to `development`.
