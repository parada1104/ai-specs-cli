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

## Delivery strategy
- **Strategy**: `single-pr` while the measured candidate remains below the native review budget; split into chained PRs only if implementation evidence exceeds it.
- **Forecast**: approximately 400–650 authored changed lines for the tag-conflict seam alone, excluding generated binaries/digests.
- **Current progress**: T1 implementation and independent verification are complete; T1 is ready to commit; T2 bridge/parity work is now in progress.

## Verification evidence
- **Exploration**: `check_tag_conflicts` is advisory-only; its Python core is pure and preserves first-seen tag order. Go currently lacks top-level recipe tag/conflict acquisition.
- **Scope decision**: `merge_config` deferred because it needs a richer `[config.*]` schema acquisition/validation surface and shares no decision logic with tag conflicts.
- **RED evidence**: T1 focused Go tests failed before implementation with missing planner/acquisition/JSON behavior; T2 pending.
- **GREEN evidence**: T1 focused and full Go gate tests passed in the worker; independent `go test -count=1 ./...` passed; T2 pending.
- **Final evidence**: pending T3.

## Next step
Commit the closed T1 work unit, then let the bounded writer implement T2's fail-open bridge and parity coverage.
