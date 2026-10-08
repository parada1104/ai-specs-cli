# Go 07 SX0f.1 — Shared classify extraction

## Objective
Expose the classify decision core and its helpers through gate/shared.
Keep the gate command and envelope tests in package main.
Preserve behavior exactly; this is an extraction.

## Tracker
- **card_id**: `6ac71e5dd5291f5ebfdf0011`
- **url**: https://trello.com/c/lxyHbMYO
- **list**: Review
- **ledger_item**: `ba1b99c47327047c`

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
- [x] T3: Run full validation, independent verification, and applicable native review. Functional verification returned ACCEPT. Native review was unavailable after three attempts and is recorded as such, not as approval.
- [ ] T4: Commit, publish, merge into the epic, close the tracker, and clean the worktree. **In progress**.

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
- Full validation exited 0 in 37 minutes: Python suite 2482 tests with zero skips, gate and root Go packages ok, 29 fixtures per gate mode with zero deltas.
- Independent verification returned ACCEPT: the shared bodies are byte-identical to base after export-name and package-qualifier normalization, no classify definition or alias remains in package main, the four envelope CLI tests still exercise the dispatch, and the hashing and regular-file semantics are unchanged.
- Work-unit commit: b2d9d60bd57de76b15dc525c9c3786f0ccaff64c. Authored total: 874 lines.
- Native review is unavailable for this candidate; see the outcome section below.

## Native review outcome: UNAVAILABLE
- Attempt 1: inspect returned the narrow 11-path candidate; start returned consent-binding-stale with native_invocation_attempted false and lineage_created false, offered the wide ambient candidate (base e773514, about 250 paths), and took 8162 seconds of wall clock, so the binding was already past its window.
- Attempt 2: inspect and start in one call with a fresh key returned the same native_invocation_attempted false and lineage_created false.
- Assess with the explicit base returned a real assessment: risk medium, 874 changed lines, reviewDue true, reason slice_budget_reached, plus a continuation to run bound status with that base. That status returned action start with the narrow candidate.
- Attempt 3: start after that status timed out at 420 seconds with no envelope. The follow-up target-scoped status then timed out at 300 seconds, where the same call had returned instantly minutes earlier.
- No lineage ever existed, so there is nothing to abandon or reset and reset_eligible is false. The friction is recorded in the global native-review-protocol skill and in Engram under native-review/frictions.
- Consequence: no native approval exists for this candidate. The risk-gated fallback applies, writer self-verification plus a separate independent verifier, and both are satisfied. The user explicitly authorized delivery with this fallback.
- Do not claim native approval for this unit, here or in the pull request.

## Python retirement boundary
- sync.go and syncagent.go still invoke Python materialize. S14 replaces those exec seams.
- Template and hook bridge invocation, printing, and lock writes stay Python until their own units.
- SX0f.2 extracts the template actuator; the hook actuator follows it.
- S13, S14, and S16 still cover the remaining materialize paths.

## Next step
Delegate T3 full validation with durable exit evidence and bounded waits. Freeze source and digest bytes.
