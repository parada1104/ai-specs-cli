# Go 07 SX0e — Shared reconcile-stamp planner

## Objective
Expose the pure stamp planner for native S11.
Reuse the root TOML normalizer without adding dependencies.
Preserve gate acquisition until its Python retirement slice.

## Tracker
- **card_id**: `6ac5c1b7947ff3696e25fd14`
- **url**: https://trello.com/c/lUGb2KwH
- **list**: In Progress

## Scope
- Base: epic/go-single-binary at 6bb8e4b.
- Worktree: .worktrees/go-07-sx0e-stamps.
- Branch: change/go-07-sx0e-stamps.
- Move stamp types, planner, and pure tests into gate/shared.
- Keep command and acquisition wrappers in main.
- Update recipe_toml.go references to the exported source type only.
- Expose existing PlainValue behavior in internal/toml with a focused test.
- Rebuild canonical gate checksums.
- Reconcile the passive SX0d-conflicts closure document.

## Constraints
- Parent and Pi subagents only. No Herdr or Orca.
- User authorizes commit, push, PR, and merge into the epic after checks.
- Do not modify development or publish releases.
- Budget: 1200 authored additions plus deletions, including moves and evidence.
- Expected total: approximately 580 lines plus closure evidence.
- Do not migrate gate Python readers or add parser dependencies here.
- The planner move uses baseline and post-move checks, not artificial behavior RED.
- The public normalizer seam requires test-first RED and GREEN.
- PlainValue preserves existing handling of tables and arrays.
- PlainValue does not recursively normalize arbitrary Go maps.
- S11 must normalize each reconcile field value before invoking the shared planner.
- Run full validation before committing.

## Tasks
- [x] T1: Extract pure stamp planner and adapt tests. Baseline and post-move checks passed.
- [x] T2: Expose the existing TOML normalizer with RED and GREEN evidence. All focused checks passed.
- [x] T3: Rebuild gate artifacts and verify canonical checksums and focused suites. All checks passed.
- [ ] T4: Run full validation, independent verification, and applicable native review. **In progress**.
- [ ] T5: Commit, publish, merge into the epic, close the tracker, and clean the worktree.

## Acceptance criteria
- Planner order, used-field defaults, and JSON shape stay unchanged.
- Main command and acquisition tests retain their behavior.
- Root normalization converts raw expectation table arrays into the planner JSON shape.
- Existing Python reader constants and execution behavior remain unchanged.
- Gate and root tests, vet, canonical checksums, and full validation pass.
- Both parity modes produce zero unexplained deltas.
- Authored diff stays within 1200 lines.

## Checks
- Gate baseline and post-move: go test -count=1 ./... and go vet ./....
- Root normalizer RED/GREEN: go test ./internal/toml -run PlainValue -count=1 -v.
- Root suite: go test ./cmd/... ./internal/... and go vet ./cmd/... ./internal/....
- scripts/build-gate.sh with Go 1.24.13; generate sums separately and run scripts/verify-gate-sums.sh.
- ./tests/validate.sh with durable external exit evidence and bounded waits.
- Native review follows the user-owned switch and uses this slice base.

## Progress
- Parent verified this feature worktree at 6bb8e4bdc0426e18341a5b90a35e35484a03e376.
- SX0d-conflicts PR330 merged. Its card is Done and ledger is closed.
- SX0d feature cleanup verified remote absence.
- Parent inspected existing normalizer behavior and its three local references.
- Parent linked the new SX0e card before source writes.
- T1 moved three planner types and two pure tests into shared. Main acquisition and command tests stayed in main.
- Gate baseline, post-move, vet, focused planner and command checks, and root tests passed.
- Normalized planner, JSON tags, moved tests, and command wrapper match the base.
- T2 added PlainValue delegating to the unchanged private normalizer and a raw TOML expectations test.
- Compile RED reported undefined PlainValue. Identity-stub RED reported []*Table instead of []any. The real wrapper passed GREEN.
- Focused TOML tests, the root suite, vet, and formatting passed. The arbitrary-map pass-through boundary is covered.
- Canonical Go 1.24.13 rebuilt four platforms. Two builds produced identical digests; both checksum comparisons passed.
- Native version 0.24.0, selftest, fresh gate/root tests, and both vet commands passed.
- A relative-output build attempt failed. Its two directories are preserved under ignored dist/; parent confirmed no tracked artifacts or stray roots.
- Authored diff: 700 lines including moves and documents, before this passive update.
- Full validation exited 0 in 40 minutes: 2482 tests, zero unittest skips, and zero deltas in 58 parity runs.
- Independent verifier returned ACCEPT. Fresh shared and PlainValue tests and four checksum checks passed.
- An external probe confirmed per-field normalization feeds the shared planner. Raw maps silently omit used defaults; S11 must commit a call-site regression test.
- Parent fresh PlainValue test passed. Writer vet and native selftest evidence stands; the verifier did not rerun those commands.
- LSP confirmed six files clean. Semgrep flagged the unchanged interpreter call at recipe_toml.go:386; acquisition remains outside this slice.
- Commits and native review remain pending.

## Python retirement boundary
- This slice enables S11 to use shared planning with root-native TOML values.
- It does not remove gate embedded Python readers.
- Gate parser constraints and the module cycle require a later bounded retirement decision.
- S14 connects orchestration. S16 removes verified legacy paths.

## Next step
Commit the verified work unit and run native review against base 6bb8e4b. Freeze source and trust-manifest bytes.
