# Go 07 SX0c — Shared copy-apply extraction

## Objective
Extract the gate copy-apply authority into the shared Go package.
Add a native root authority that S12 can call in process.
Preserve the Python oracle until S14 connects orchestration.

## Tracker
- **card_id**: `6ac681095f3e34bc5b0664f4`
- **url**: https://trello.com/c/2v7tC2pY
- **list**: Review
- **ledger_item**: `f07c0ba359f8fbe6`

## Scope
- Base: epic/go-single-binary at 1c2cc32.
- Worktree: .worktrees/go-07-sx0c.
- Branch: change/go-07-sx0c.
- Move gate copyapply production code and its tests into gate/shared.
- Export the command entry point and the names the gate and its tests must reach.
- Update the gate dispatch to the shared entry point.
- Rebuild canonical gate checksums after the gate source change.
- Deferred to S12 as its declared first unit: the root copy authority in internal/sync and its Python-oracle differential. This extraction only makes the authority importable. See the deferred scope note below.

## Constraints
- Parent and Pi subagents only. No Herdr or Orca.
- User authorizes commit, push, PR, and merge into the epic after checks.
- Do not modify development or publish releases.
- Budget: 1200 authored additions plus deletions, including moved lines, tests, and documents.
- Expected total: 650–750 lines. Stop before exceeding the budget.
- The gate copy-apply file is 883 lines. A full move counts both sides, so it exceeds the budget and approaches the size where the reliability lens fails.
- Measured correction: only the three primitive tests must move with the primitives. The eleven command tests reach the command through its envelope, so they stay in main with small edits once the entry point and its result type are exported.
- SX0c.1 moved statMode, copyFileStat, and copyTree with their three focused tests into shared. The gate command layer now calls the exported primitives.
- SX0c.2 moves the copy types, decision, execution, and command entry point into shared, and updates the eleven command tests in main to the exported names.
- Combined authored total stays well under 1200 lines, so both commits ship in one pull request as one work unit.
- Each step must keep both packages compiling and tested.
- Do not add dependencies or speculative abstractions.
- Do not extract classify or the template and hook actuators in this slice. Those are separate units.
- Do not modify production Python. Python remains the differential oracle.
- Do not wire the sync CLI. S14 owns orchestration and caller diagnostics.
- The gate module must not gain a dependency on the root module.
- Behavior tests require observed RED, GREEN, then proportionate refactoring. Extraction units use baseline and post-move evidence with normalized body comparison instead; do not fabricate behavior RED for a move.
- Run full validation before committing.

## Tasks
- [x] T1: Move the copy primitives and their focused tests into gate/shared. Baseline and post-move checks passed with normalized body identity.
- [x] T2: Move the gate copy-apply command layer into gate/shared, export its entry point, and update the dispatch. Baseline and post-move checks passed with normalized body identity.
- [x] T3: Rebuild gate artifacts with the canonical toolchain and verify checksums, gate tests, and root tests. Four-platform digests regenerated and verified twice.
- [x] T4: Run full validation, independent verification, and applicable native review. Functional verification passed; its PARTIAL verdict was a declared-scope mismatch, now corrected below. Native review approved the candidate.
- [ ] T5: Commit, publish, merge into the epic, close the tracker, and clean the worktree. **In progress**.

## Acceptance criteria
- The shared package exposes the copy-apply entry point the gate dispatches to.
- Gate behavior, output, and exit codes stay unchanged after the move.
- Moved tests keep covering file, directory, and tree copy internals from the shared package.
- The root authority composes the shared entry point with native acquisition instead of duplicating copy logic. Deferred to S12; not part of this extraction.
- Per-item copy semantics, source-missing handling, and envelope ordering are preserved unchanged by the move and were smoke-tested on the built binary.
- Python-oracle differential compares real authority outcomes, not recorded expectations.
- Canonical four-platform digests regenerate and verify.
- Gate and root tests, vet, and full validation pass.
- Both parity modes produce zero unexplained deltas.
- Authored diff stays within 1200 lines.

## Checks
- Focused RED/GREEN: go test ./internal/sync/ -run CopyApply -count=1 -v.
- Gate and root suites: go test ./... in the gate module and go test ./cmd/... ./internal/... at the root.
- vet in both modules, and gofmt on changed files.
- scripts/build-gate.sh with the canonical Go toolchain, then generate sums and run scripts/verify-gate-sums.sh.
- ./tests/validate.sh with durable exit records and bounded waits.
- Native review follows the user-owned switch and uses this slice base.

## API contract
- The shared package holds the copy-apply types and the exported entry point.
- The gate keeps only flag parsing and its dispatch call.
- The root authority presents a copy request built from native acquisition and returns the shared outcome.
- No new module dependency is introduced in either direction.
- Existing gate exit-code and JSON contracts are preserved exactly.

## Progress
- Parent verified epic 1c2cc32f73ddd7c040c4f8f3306e6438aaff1831 after PR332 merged.
- Parent created this feature worktree and registered it.
- Parent inspected the board and linked this new card before source writes.
- Parent measured the gate copy files and planned a two-unit split so each stayed inside the budget and the review size limit.
- T1 completed the primitive extraction in 464 authored lines, regenerated the canonical four-platform digests, and verified them.
- Measuring the command tests showed they reach the command through its envelope, so they can remain in main. The combined extraction fits one work unit and one pull request.
- T2 moved the command layer into shared, exported RunApplyCopy, CopyRequest, CopyItem, CopyResult, and ApplyCopyDecision, and pointed the gate dispatch at the shared entry point. The gate source file was deleted because it moved whole.
- All eleven command tests stayed in main with five small edits; no test moved, weakened, or deleted, and no helper was duplicated.
- Normalized comparison of the moved command bodies against base 1c2cc32 is identical.
- Parent confirmed the focused command and primitive tests pass, the committed digests match the built binaries, and the slice totals 959 authored lines including this document.
- Full validation, commits, and native review remain pending.
## Deferred scope and findings
- Verifier returned PARTIAL on 1d6e4a8 solely because the Scope section still declared the root authority and its differential. The Constraints and API contract already placed that wiring with S12 and S14. Parent corrected the declared scope instead of expanding the slice; adding it would push the slice past 1200 authored lines.
- S12's first unit is the root copy authority in internal/sync plus its Python-oracle differential. This slice makes that possible.
- Non-blocking follow-ups: CopyRequest, StatMode, CopyFileStat, and CopyTree have no caller outside shared; they can be unexported when S12 shapes the root API. internal/projectcache.CopyTree duplicates the same copytree parity and is pre-existing; consolidate it on this shared authority later.
- Full validation on 1d6e4a8: exit 0 after 40m27s, 2482 tests with no skips, 29 fixtures per gate mode, zero deltas in all 58 runs.
- Independent verification: normalized bodies identical, main dispatch is a one-line change, eleven command tests and three primitive tests exist with no weakening, digests match, and the built binary answers --apply-copy with the documented envelope and exit codes.
- Verifier measured 964 authored lines with --no-renames against the parent's 959. Both are inside the 1200 budget.
- Native review review-fa216f2ee6b36829 approved the committed candidate after one reliability run and listed no advisories. Exact acknowledgement consumed its authority.
- Work-unit commit: 1d6e4a8e587202619cebd087f0a1d81ee7ea77fe. Scope-correction commit: 243619d09c605120c1a01e71b3ced636ec7ae4b7.
- Rollback restores the gate copy-apply file and its dispatch, removes the shared copy files, and restores the prior trust manifest. No Python source changes exist to revert.

## Python retirement boundary
- sync.go and syncagent.go still invoke Python materialize. S14 replaces those exec seams.
- The copy-apply bridge and its Python fallbacks remain the oracle until their verified retirement.
- Template and hook actuators still need a shared classify extraction before their own slices.
- S12, S13, S14, and S16 still cover the remaining materialize paths.
- Do not claim Python retirement from an importable native library alone.

## Next step
Commit the verified extraction as one work unit and run T4 full validation in the same base.
