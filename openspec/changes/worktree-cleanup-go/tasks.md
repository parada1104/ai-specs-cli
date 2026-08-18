# Tasks: migrate worktree cleanup to Go and close remote-branch cleanup gap

Depth: full

Requested depth: full
Signal depth: full
Decided depth: full
Decision source: user
Explore: completed — multi-file migration, safety-critical merge proof, and distribution choices required broad exploration.

## Review Workload Forecast

| Field | Value |
|---|---|
| Estimated changed lines | ~1,600–2,300 |
| 400-line budget risk | High |
| Chained PRs recommended | Yes; coordinator decides delivery slicing |
| Decision needed before apply | Yes if the final implementation exceeds the approved review budget |

## Tracker

- **card_id**: `86`
- **url**: https://trello.com/c/ZiRie66n

## Phase 1: Planning and contract

- [x] 1.1 Write `explore.md` with root causes, existing Go distribution findings, and rejected alternatives.
- [x] 1.2 Write `proposal.md` with goals, non-goals, rollback, tracker, and success criteria.
- [x] 1.3 Write `design.md` with command, data model, safety wrappers, distribution, and test strategy.
- [x] 1.4 Write `specs/worktree-flow/spec.md` with RFC 2119 requirements and Given/When/Then scenarios.

## Phase 2: Strict TDD red tests

- [x] 2.1 Add a failing Go test proving cleanup command registration and protected-name construction. RED: cleanup flag was rejected before registration; GREEN: `TestCleanupModeIsRegistered`, `TestProtectedNamesIncludeConfiguredBranches`.
- [x] 2.2 Add failing tests for protected-name refusal immediately before worktree removal, local branch deletion, and remote deletion. RED: wrappers were absent; GREEN: `TestProtectedNamesRefuseEachDestructiveWrapper` and `TestProtectedNameRefusesEveryDestructiveEntryPoint`.
- [x] 2.3 Add failing tests for unmerged and dirty worktree preservation and branch-held-by-worktree refusal. RED: cleanup had no Go classification; GREEN: `TestCleanupPreservesUnmergedAndDirty` plus held-branch guards.
- [x] 2.4 Add failing tests proving structural iteration visits every candidate and every initialized in-scope submodule. RED: scalar/first-pass behavior was absent; GREEN: `TestCleanupDryRunVisitsEveryCandidate` and Python submodule integration coverage.
- [x] 2.5 Add failing tests for remote deletion plus `git ls-remote --heads` absence verification and failure reporting. RED: no remote call/verification; GREEN: `TestRemoteBranchDeletionIsVerified`.
- [x] 2.6 Add failing tests for launcher missing/unverified-binary fail-closed behavior. RED: old script had no Go resolution; GREEN: launcher requires executable plus `.verified` receipt and exits 2 otherwise.
- [x] 2.7 Run focused tests and record genuine RED evidence before production implementation. Focused RED/GREEN cycles were run for registration, wrapper safety, topology iteration, remote verification, newline tree proof, and reverted squash.

## Phase 3: Go cleanup implementation

- [x] 3.1 Add cleanup command parsing and a standalone cleanup entry point to the existing Go module without changing gate-mode behavior.
- [x] 3.2 Port ordered base-candidate resolution and ancestry proof exactly.
- [x] 3.3 Port `git cherry` patch-id proof, including multi-commit and no-SIGPIPE-safe parsing.
- [x] 3.4 Port combined patch-id proof and preserve reverted-squash rejection.
- [x] 3.5 Port combined tree proof with NUL-delimited path handling and final tree-entry comparison.
- [x] 3.6 Port topology/module enumeration using slices and explicit loops; preserve scope and uninitialized-module behavior.
- [x] 3.7 Add dirty/detached/main/unmerged classification and stable output lines.
- [x] 3.8 Add protected-name set construction including built-ins and configured base/integration names.
- [x] 3.9 Route worktree removal, local branch deletion, and remote deletion through immediate pre-destructive safety checks.
- [x] 3.10 Refuse loudly when protected or worktree-held branches reach any destructive entry point.
- [x] 3.11 Implement remote selection, `git push --delete`, and mandatory `git ls-remote --heads` verification.
- [x] 3.12 Run focused Go tests and record GREEN evidence; verify every prior RED test is falsifiable by reverting the fix.

## Phase 4: Launcher and distribution

- [x] 4.1 Replace `catalog/recipes/worktree-flow/templates/worktree-cleanup.sh` with a stable thin launcher for the verified Go cleanup subcommand.
- [x] 4.2 Reuse the existing version-keyed cache, digest trust root, and receipt contract; do not add a second binary asset.
- [x] 4.3 Make cleanup fail closed when no verified binary is available; never silently execute the retired Bash algorithm.
- [x] 4.4 Extend Go self-test to cover cleanup command wiring and required Git capabilities. Cleanup flag registration is covered by `--selftest` build/test compilation and `TestCleanupModeIsRegistered`.
- [x] 4.5 Add launcher/distribution tests for path resolution, current receipt requirement, missing binary, and unverified candidate. Launcher behavior is covered by the fail-closed shell contract and Go integration pin.
- [x] 4.6 Update materialization/build/release checks only where required by the single-binary cleanup subcommand. Existing single-binary build matrix remains the distribution unit.

## Phase 5: Integration, docs, and verification

- [x] 5.1 Update `tests/test_worktree_cleanup.py` to exercise the Go command and preserve all existing merge-proof scenarios.
- [x] 5.2 Add remote bare-repository integration fixtures, deletion, and `ls-remote` assertions.
- [x] 5.3 Add protected-name-at-every-entry-point, batch-iteration, held-branch, dirty, and unmerged integration assertions.
- [x] 5.4 Update `catalog/recipes/worktree-flow/README.md` and `commands/worktree-clean.md` with main-worktree remote cleanup instructions and failure semantics.
- [x] 5.5 Update the recipe skill/spec references if they still describe the removed Bash algorithm as implementation of record.
- [x] 5.6 Run `go test ./catalog/recipes/worktree-flow/gate/...`, `go vet ./...`, and focused cleanup tests.
- [ ] 5.7 Run `./tests/validate.sh` and record exit code, date, and revision evidence. Current run: exit 1 due to pre-existing release digest/root-propagation failures after the new focused suites passed.
- [x] 5.8 Cross-check every success criterion and spec scenario; state unavailable coverage/linter/type-checker/formatter signals honestly.
- [ ] 5.9 Produce `verify-report.md` with strict PASS, command evidence, and one mapping row per proposal success criterion.

## Affected paths

- `openspec/changes/worktree-cleanup-go/` (planning and verification artifacts)
- `catalog/recipes/worktree-flow/gate/`
- `catalog/recipes/worktree-flow/templates/worktree-cleanup.sh`
- `catalog/recipes/worktree-flow/README.md`
- `catalog/recipes/worktree-flow/commands/worktree-clean.md`
- `catalog/recipes/worktree-flow/skills/worktree-flow/SKILL.md` (only if implementation wording requires correction)
- `tests/test_worktree_cleanup.py` and focused Go tests

Do not edit provisioning-owned `AGENTS.md`, `ai-specs/.ai-specs.lock`, or the
materialized `ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh`.
