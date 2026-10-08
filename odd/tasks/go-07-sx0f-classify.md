# Go 07 SX0f.1 — Shared classify extraction

## Objective
Expose the classify decision core and its helpers through gate/shared.
Keep the gate command and envelope tests in package main.
Preserve behavior exactly; this is an extraction.

## Tracker
- **card_id**: `6ac71e5dd5291f5ebfdf0011`
- **url**: https://trello.com/c/lxyHbMYO
- **list**: In Progress

## Scope
- Base: epic/go-single-binary at 8b82105.
- Worktree: .worktrees/go-07-sx0f-classify.
- Branch: change/go-07-sx0f-classify.
- Move gate/classify.go into gate/shared with exported symbols.
- Move the internals tests with the code; keep the envelope CLI tests in main against the shared entry point.
- Qualify the classify symbol references in the two remaining main-side actuator files.
- Update the gate dispatch for --plan-classify.
- Regenerate the canonical four-platform trust digests.

## Constraints
- Parent and Pi subagents only. No Herdr or Orca.
- User authorizes commit, push, PR, and merge into the epic after checks.
- Do not modify development or publish releases.
- Budget: 1200 authored additions plus deletions, including both sides of moved content and documents.
- Expected total: 400-500 lines.
- Do not move the template or hook actuators in this unit. SX0f.2 owns the template actuator.
- Do not modify production Python. Python remains the differential oracle.
- Do not wire the sync CLI. S14 owns orchestration.
- Preserve every behavior, message, and exit code.
- Extraction units use baseline and post-move evidence with normalized body comparison; do not fabricate behavior RED for a move.
- Run full validation before committing.

## Tasks
- [x] T1: Move classify into shared, adapt the tests, qualify main-side callers, and update the dispatch. Normalized body comparison against base is byte-identical.
- [x] T2: Rebuild gate artifacts with the canonical toolchain and verify checksums and focused suites. Four platforms rebuilt with go1.24.13; digests regenerate and verify.
- [ ] T3: Run full validation, independent verification, and applicable native review. **In progress**.
- [ ] T3: Run full validation, independent verification, and applicable native review.
- [ ] T4: Commit, publish, merge into the epic, close the tracker, and clean the worktree.

## Acceptance criteria
- The shared package exposes the classify decision core, its state constants, and the three helpers its callers need.
- Gate behavior, output, and exit codes are unchanged.
- The moved internals tests cover the same behavior from shared.
- The envelope CLI tests stay in main and pass against the shared entry point.
- No duplicate classify definition remains in package main.
- Canonical four-platform digests regenerate and verify.
- Gate and root tests, vet, and full validation pass.
- Both parity modes produce zero unexplained deltas.
- Authored diff stays within 1200 lines.

## Checks
- Baseline and post-move: go test -count=1 ./... inside the gate module.
- Focused: go test -count=1 ./shared/ -run Classify, and the remaining classify tests in main.
- Root suite: go test ./cmd/... ./internal/... and go vet ./internal/sync/.
- scripts/build-gate.sh with an absolute output directory, then generate sums and run scripts/verify-gate-sums.sh.
- ./tests/validate.sh with durable exit records and bounded waits.
- Native review follows the user-owned switch and uses this slice base.

## API contract
- ClassifyManagedOverride stays the pure decision core, exported.
- The state constants, the input and result types, and the managed-entry type are exported.
- Sha256Bytes and ReadRegularFile are exported because the actuators in main still call them.
- RunPlanClassify stays the command entry point the dispatch calls.
- No new module dependency is introduced in either direction.
- The gate JSON contract and exit codes are preserved exactly.

## Progress
- Parent verified this worktree at 8b821054501643038d8e8242ae7e1b30c9430a1d.
- Parent mapped the move set: classify.go is 159 lines and everything in it is unexported today.
- The template and hook actuators in main call classify symbols, so they need qualification edits only.
- The trust manifest needs regeneration because the gate source changes.
- Parent linked this unit card before source writes.
- No source edits, checks, commits, or review results exist yet.
- T1 moved classify into shared with exported symbols and no main-side alias. The two internals tests moved with the code; the four envelope CLI tests stayed in main against the shared entry point.
- The parent approved widening the allowed surface to the two actuator test files so the 29 Sha256Bytes call sites could be qualified instead of leaving a duplicate alias in main.
- Parent confirmed focused tests pass in both packages, the trust digests match the built binaries, and gate/classify.go is gone.
- Authored diff: 868 lines including both sides of the move and this document.
- Full validation, commits, and native review remain pending.

## Python retirement boundary
- sync.go and syncagent.go still invoke Python materialize. S14 replaces those exec seams.
- Template and hook bridge invocation, printing, and lock writes stay Python until their own units.
- SX0f.2 extracts the template actuator; the hook actuator follows it.
- S13, S14, and S16 still cover the remaining materialize paths.

## Next step
Delegate T3 full validation with durable exit evidence and bounded waits. Freeze source and digest bytes.
