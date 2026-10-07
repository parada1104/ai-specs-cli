# Go 07 SX0d.1 — Shared conflict graders

## Objective
Expose pure tag and primitive conflict graders for S11.
Preserve the existing acquisition and CLI behavior.
This prerequisite does not retire Python yet.

## Tracker
- **card_id**: `6ac5a9f026b568a5fbead392`
- **url**: https://trello.com/c/ho2xO00d
- **list**: In Progress

## Scope
- Base: epic/go-single-binary at 3ab9d2e.
- Worktree: .worktrees/go-07-sx0d-conflicts.
- Branch: change/go-07-sx0d-conflicts.
- Move pure graders and their types into shared/recipeconflicts.go.
- Keep acquisition and command wrappers in package main.
- Move only pure tests; keep acquisition and command tests in main.
- Reconcile the passive S10 delivery snapshot with observed merge and cleanup evidence.
- Rebuild gate artifacts and update the checksum manifest when required.

## Constraints
- Parent and Pi subagents only. No Herdr or Orca.
- User authorizes commit, push, PR, and merge into the epic after checks.
- Do not change development or publish releases.
- Review budget: 1200 authored additions plus deletions, including moved lines.
- Estimate: 890 extraction lines plus task and closure evidence.
- Stop before exceeding the budget. Do not remove tests or compress code to fit.
- Do not modify TOML acquisition semantics or add dependencies.
- Mechanical move: no new behavior requires artificial RED. Observe baseline and post-move tests instead.
- Run full validation before committing.

## Tasks
- [x] T1: Extract pure conflict graders and adapt their tests. Baseline and post-move checks passed.
- [x] T2: Rebuild the gate and verify checksums, gate tests, and root tests. Canonical reproducible digests passed.
- [ ] T3: Run full validation, both parity modes, independent checks, and applicable native review. **In progress**.
- [ ] T4: Commit, publish, merge into the epic, close the tracker, and clean the worktree.

## Acceptance criteria
- Shared graders preserve order, conflict outcomes, and JSON shape.
- Main acquisition and CLI tests retain their behavior.
- Existing Python subprocess acquisition stays unchanged and is recorded as active debt.
- Gate tests, vet, root tests, checksum verification, and full validation pass.
- Both gate modes produce zero unexplained parity deltas.
- Authored diff stays within 1200 lines.

## Checks
- Baseline and post-move: gate go test ./... and root go test ./cmd/... ./internal/....
- Gate go vet ./... and root go vet ./cmd/... ./internal/....
- scripts/build-gate.sh and scripts/verify-gate-sums.sh, following their actual command contract.
- ./tests/validate.sh with durable external exit evidence and tool waits under 20 minutes.
- Native review only under the user-owned switch.

## Progress
- S10 PR329 merged as 3ab9d2e99fb5be45c66bc93ecb17e82436fc67ad.
- S10 card Done and ledger closed were observed. Feature cleanup verified remote absence.
- Parent verified this clean feature worktree and linked its existing new card.
- Pure graders and tests moved to shared. Acquisition, command wrappers, and embedded Python constants remain unchanged.
- Baseline and post-move gate and root tests passed. Both vet commands passed; gofmt is clean.
- Normalized grader-body comparisons were identical. No behavior RED was applicable to this mechanical move.
- T2 authored diff: 1113 lines, including untracked files and both sides of moved content.
- Canonical Go 1.24.13 built four platforms. A second build produced identical digests; checksum verification passed twice.
- Gate version and selftest passed. Gate and root tests and vet passed.
- One checksum check used a missing generated path and exited 2. The corrected generated-sums invocation passed.
- Full validation exited 0: 2482 tests, zero skips, and 29 fixtures in each gate mode with zero deltas.
- Independent verifier returned ACCEPT. Fresh shared tests and four checksum checks passed.
- Ten moved declarations and six pure tests match the base modulo exported names. Four Python reader constants remain byte-identical.
- Parent fresh shared test passed. Verified authored diff was 1116 lines before this passive evidence update.
- LSP confirmed seven files clean. Semgrep flagged the unchanged interpreter call at recipe_toml.go:386, identical to base line 395.
- The existing acquisition finding remains outside this extraction scope. Commit and native review remain pending.

## Python retirement boundary
- recipe_toml.go still executes Python for four readers.
- This slice exposes pure Go graders only.
- Stamps acquisition requires a later cohesive slice within the same budget.
- S14 connects materialize orchestration. S16 removes legacy Python after route checks.

## Next step
Commit the verified work unit and run native review against base 3ab9d2e. Freeze gate source and checksum bytes.
