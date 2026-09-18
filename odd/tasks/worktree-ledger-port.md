# Worktree Ledger port

## Objective
Add the Worktree domain port to the autonomous Go ledger core and route worktree-flow cleanup through its deterministic golden/evidence contract, starting with the confirmed squash-merge false negative.

## Problem
A linked worktree whose branch was squash-merged is reported as `unmerged` because `cleanupOnePass` only evaluates local graph/patch/tree equivalence. The existing stale-branch path already has a provider-neutral merge-commit observation, but that evidence is not represented at the Worktree domain boundary. Cleanup therefore bypasses the ledger-port architecture and duplicates an incomplete decision path.

## Why now
The repository already defines an autonomous ledger core with Tracker as one domain port and provider recipes as adapters. Worktree-flow is the next domain port. The confirmed orphaned worktree (`feat/tracker-ledger-followups`) has a merged PR (#261), merge commit `90c19a3`, and a branch tip that is not an ancestor of `development`; dry-run reports `(unmerged)` while the worktree is clean.

## Scope
- Define the smallest provider-neutral Worktree port inside the existing `ledger` Go package; do not create a second ledger framework or a new store/witness/checkpoint surface.
- Normalize local worktree/branch state and merge evidence into deterministic Worktree observations and golden outcomes.
- Reuse the existing PR merge-commit evidence for linked worktrees, accepting a candidate only when the recorded merge commit is an ancestor of one of the already-resolved base candidates.
- Route cleanup classification through the Worktree port while retaining immediate protected-branch, worktree-held, dirty, detached, remote-deletion, ledger-close, and mutation safety checks.
- Add focused Go regressions and golden/corpus coverage for regular merge, squash merge, missing evidence, dirty, detached, protected, and unmerged cases.
- Amend the canonical tracker-ledger/worktree contracts to allow the Worktree domain port while keeping Tracker and Worktree semantics separate.
- Keep Bash/Python as thin acquisition/materialization bridges; do not rewrite launchers or introduce provider-specific Go vocabulary.

## Architecture decision
The repository's current port architecture is a documented boundary rather than a Go interface. The Worktree port therefore lives as pure `ledger/worktree.go` types and evaluation, reusing the package's deterministic-predicate discipline and existing `CloseBranchItem` lifecycle seam. Cleanup remains the Go actuator and acquisition host; it feeds normalized observations to the port and preserves all immediate destructive safety checks. PR merge evidence is read-only acquisition and never an automatic provider mutation.

## Constraints
- One Go binary, one trust root, stdlib only.
- Ledger core remains authoritative for deterministic evaluation; Worktree port supplies domain mapping and observations.
- Missing or ambiguous merge evidence preserves the candidate; no guessed cleanup.
- No provider writes; `gh` is read-only acquisition only where the existing cleanup contract already permits it.
- Preserve unrelated worktrees and existing cleanup behavior.
- TDD enabled by project configuration; runner: `./tests/validate.sh`.
- Delivery strategy: ask-on-risk; implementation branch/worktree is `.worktrees/worktree-ledger-port` / `feat/worktree-ledger-port`.

## Tracker

- **card_id**: `6a93705377a8419254748437`
- **url**: https://trello.com/c/XwdM0Dmi/113-guardian-convert-premergeguardian-into-an-automated-gate-hook-ci-align-parser-with-sdd-report-format
- **note**: Existing architecture/ledger-ports card reused because Trello MCP is unavailable for a new card.

## Tasks

- [x] T1 — Define the Worktree port contract and deterministic golden evaluator with RED/GREEN tests.
- [x] T2 — Integrate linked-worktree squash merge evidence into Go cleanup through the Worktree port.
- [x] T3 — Add golden/corpus and recipe contract coverage; reconcile docs without duplicating logic.
- [x] T4 — Run focused and full validation, rebuild/verify the Go trust root, and record evidence.

## Acceptance criteria

- The Worktree ledger port is a domain adapter over the existing autonomous core, not a parallel ledger framework.
- A clean linked worktree whose branch was squash-merged is classified as merged when its PR merge commit is provably in the base.
- Missing, malformed, unmerged, dirty, detached, protected, or ambiguous candidates remain preserved with deterministic reasons.
- Local merge proof and PR merge-commit evidence are normalized once and consumed by cleanup without duplicate decision logic.
- Existing cleanup and Tracker Ledger parity remain green.
- `./tests/validate.sh`, focused Go tests, and trust-root verification pass, with failed/skipped checks recorded honestly.

## Progress and evidence

- Status: implementation and verification complete; PR delivery pending.
- Exploration: confirmed PR #261 merge commit `90c19a3` is in `development`, branch tip `7b3e353` is not, worktree is clean, and cleanup dry-run reports `skipped tracker-ledger-followups (unmerged)`.
- Architecture: autonomous ledger core → domain ports; Tracker is existing port; Worktree is this change's port. The Worktree evaluator is pure and does not read Tracker witness/store/checkpoints.
- T1/T2 evidence: `ledger/worktree.go` adds normalized `WorktreeObservation`/`WorktreeOutcome` and `EvaluateWorktree`; linked and stale cleanup paths share local + PR merge-commit acquisition across all resolved base candidates. Focused Go tests are green, including squash-merge, out-of-base, missing-`gh`, and base-candidate regressions.
- T3 evidence: six JSON golden fixtures with a non-vacuous mutation guard; tracker-ledger and worktree-flow specs plus capabilities/recipe docs now describe independent domain ports and read-only PR evidence.
- Independent verification: focused ledger and all-Go suites passed; it found provider CLI acquisition inherited the process cwd for submodule passes. A RED/GREEN correction now pins `gh` to the owning `repoRoot`, with a hermetic cwd regression covering empty and populated merge evidence.
- T4 evidence: `./tests/validate.sh` passed with 2139 tests, 0 failures, and 2 skips; `go -C catalog/recipes/worktree-flow/gate test -count=1 ./...`, `gofmt -l`, `go -C catalog/recipes/worktree-flow/gate vet ./...`, and `git diff --check` passed. `bash scripts/build-gate.sh <absolute-dist>` with canonical Go 1.24.13 rebuilt all four assets; `bash scripts/verify-gate-sums.sh <generated> catalog/recipes/worktree-flow/bin/SHA256SUMS` passed with 4 digest entries. The bounded review correction now binds provider PR evidence to the current head OID; targeted native validation approved it.
- Remaining verification note: the two skipped Python tests were not named by the validation runner; canonical OpenSpec validation was run and matches the development baseline exactly (32 passed, 23 pre-existing failures). New untracked Go/JSON files were directly read and exercised by Go tests; `gofmt -l` is clean.
- Native review: the earlier lineage `review-8a6f1cbf8ba5c186` was abandoned after reviewer transport failure. Fresh high-risk lineage `review-955802d8e2d73a97` identified stale PR evidence, accepted the bounded head-OID correction, passed targeted validation, and was acknowledged with native authority burned.
- Delivery commit: `9d7a3d8` (`feat(worktree): port cleanup decisions through ledger`).
- Next step: push `feat/worktree-ledger-port` and create the pull request.
