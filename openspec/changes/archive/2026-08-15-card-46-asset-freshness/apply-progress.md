# Apply Progress: card-46-asset-freshness

## Implementation Evidence

## Resolved Scope And Decisions

- Cleanup source of truth is `catalog/recipes/worktree-flow/templates/worktree-cleanup.sh`; `ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh` is only a materialized target.
- `worktree-gate` remains Go for gate decisions; `worktree-cleanup.sh` remains Bash and is not ported into the Go module.
- The original `git cherry` proof remains first. Complete multi-commit squash uses combined patch-id and final tree-state evidence; unrelated base commits are tolerated, while partial and reverted changes remain unmerged.
- The preflight runs after target/CLI-version resolution and before sync writes. It verifies canonical Bash sources and invokes the existing gate acquisition seam; materialization repeats state/verification at each governed target.
- Cleanup, launcher, and legacy-gate replacements use the existing `[managed.*]` lock fields and cache-only backup namespace. Unknown/user-modified bytes are replaced, not accepted as provenance.
- Go cache acceptance requires the committed `SHA256SUMS` digest, current binary version, passing self-test, and an atomic `.verified` receipt. A rejected cache candidate is quarantined and never selected.
- `VERSION`, launcher stamps, version-keyed cache paths, `SHA256SUMS`, and `.ai-specs.lock [meta].cli_version` remain separate evidence; doctor reports drift and does not rewrite the lock.

### RED

- `python3 -m unittest tests.test_worktree_cleanup tests.test_recipe_materialize.StaleCleanupOverrideTests.test_worktree_flow_user_modified_cleanup_is_force_replaced -v`
  - RED observed: `test_removes_multi_commit_squash_merge` reported `skipped feat-multi-squash (unmerged)` instead of `would remove`.
  - RED observed: `test_worktree_flow_user_modified_cleanup_is_force_replaced` left `# customized override` in place and emitted the old preserve/remove guidance.
- `./tests/validate.sh`
  - RED observed after the new fixtures: `Ran 1663 tests in 438.106s`, `FAILED (failures=2, skipped=116)`; the two failures were the multi-commit squash fixture and forced cleanup replacement fixture above.

### GREEN

- `python3 -m unittest tests.test_worktree_cleanup -v`
  - GREEN observed: `Ran 26 tests ... OK`, including multi-commit regular/squash, partial squash, reverted squash, dirty, detached, main, topology, and no-fetch cases.
- `python3 -m unittest tests.test_gate_binary_dist -q`
  - GREEN observed: `Ran 19 tests in 0.519s`, `OK`, including stale cache re-acquisition, version/self-test mismatch rejection, no execution before digest verification, and receipt rollback.
- `python3 -m unittest tests.test_doctor_worktree_gate tests.test_override_ownership.OverrideOwnershipTests.test_doctor_reports_worktree_flow_cleanup_as_force_refreshable_error -v`
  - GREEN observed: `Ran 9 tests ... OK`.
- Focused materialization, preflight, legacy-gate, doctor, sync, release, lock, and CLI-version tests passed after implementation. Task 2.1 was not applicable because the RED fixture proved the original multi-commit algorithm incomplete; the minimal cleanup correction was applied instead.

## Final Verification

- `go version` -> `go version go1.24.13 darwin/arm64`.
- `go -C catalog/recipes/worktree-flow/gate test ./...` -> `ok ai-specs.dev/worktree-gate (cached)`.
- `go -C catalog/recipes/worktree-flow/gate vet ./...` -> exit `0`, no output.
- `python3 -m unittest tests.test_gate_binary_dist tests.test_worktree_gate_dist_config tests.test_worktree_gate_release_phase4 -q` -> `Ran 44 tests ... OK (skipped=4)`.
- `./tests/validate.sh` -> `Ran 1679 tests in 457.653s`, `OK (skipped=116)`.
- Validation includes Python compilation, Bash syntax checks, Go formatting check, Go tests, and the complete unittest suite.

## Changed Behavior

- Cleanup remains Bash and now adds a conservative combined patch-id proof for a complete multi-commit squash; partial and reverted changes remain unmerged.
- Governed worktree-flow cleanup, Go launcher, and legacy gate assets force-refresh during ordinary materialization and explicit gate refresh, with lock-backed evidence and immutable cache backups where available.
- Cache Go binaries are revalidated against `SHA256SUMS`, version, self-test, and an atomic verification receipt before launcher selection.
- Sync runs a read-only canonical-source/cache freshness preflight before consumer-project writes; materialization repeats checks at the write boundary.
- Doctor reports governed worktree-flow freshness errors without mutating project files or lock state.

## Unresolved Warnings

- The repository's existing test fixtures intentionally emit legacy recipe-version warnings and network/offline Go acquisition warnings; these are not treated as passes or failures unless the test asserts them.
- Existing fixture warnings remain: legacy recipe-version notices, intentional offline/404 acquisition warnings for `0.21.0` fixtures, and 116 documented/skipped tests. No live consumer sync, consumer mutation, commit, push, PR, archive, or Gentle AI review was run.
