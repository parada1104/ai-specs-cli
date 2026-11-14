# Go 07 S12.3 — Shared hook actuator extraction

## Objective
Expose the existing gate hook actuator through an importable shared package so the
native sync authority can reuse it. Preserve the gate JSON command contract.
This is delivery A of S12.3. The root authority follows in delivery B.

## Tracker
- **Unavailable.** The Trello API key is missing or expired in this environment, so
  no card could be created or linked for this slice.
- Host check at base `8efcc7a`:
  `bash catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh --root <worktree> --checkpoint pre-merge`
  printed `tracker-card-gate: tracker-ledger: no verified gate binary resolved; failing open (run ai-specs sync / ai-specs doctor)` and exited 0.
- The tracker host is advisory by design (`Always returns 0: the Tracker host is
  advisory`), and its `identity_unavailable` rule is to report and proceed without
  recording a decision. `tracker.none` is presentation and evidence only and is never
  written by the host, so none was created here.
- Close-out follow-up: link the S12.3 card in this section once the tracker is reachable.

## Scope
- Base: `8efcc7a` on `epic/go-single-binary` (S12.1 copy, S12.2 templates and docs merged).
- Worktree: `.worktrees/go-07-s12-hook-core`. Branch: `change/go-07-s12-hook-core`.
- Move the hook actuator decision core out of package main into
  `catalog/recipes/worktree-flow/gate/shared/hookactuator.go`, and leave
  `gate/hookgateactuator.go` as the thin JSON wrapper with its historical names aliased.
- Regenerate only the four canonical gate checksum entries that the extraction requires.

## Constraints
- Budget: 1200 additions plus deletions, including both sides of moved code.
- Never remove or weaken tests to fit the budget; the extraction is a refactor whose
  checks are the existing gate tests, so no new RED cycle is claimed for it.
- The gate JSON contract is unchanged: the same stdin fields, the same stdout fields and
  order, refusals keep their stdout error envelope plus exit 2, and I/O errors keep their
  stderr diagnostics plus exit 2.
- No production Python change, no CLI wiring, no lock write, no dependency change.
- Do not modify `development` and do not publish a release.
- Parent owns checksum regeneration with go1.24.13, the commit, push, PR, merge and cleanup.

## Tasks
- [ ] T1: Extract the decision core into `gate/shared/hookactuator.go` without behavior
      change, leaving a thin wrapper and the historical aliases behind.
- [ ] T2: Regenerate the four `SHA256SUMS` digest lines with go1.24.13 and verify them
      with `scripts/verify-gate-sums.sh`.
- [ ] T3: Run the gate suite, the root suite, the hook bridge tests and the full
      validation; then commit the work unit and complete the applicable native review.
- [ ] T4: Publish and merge into the epic, then clean the worktree safely. Delivery B
      must not start before this shared API exists.

## Acceptance criteria
- Gate stdin/stdout envelope fields, JSON order and exit codes remain unchanged.
- The eight hook placeholders, the warning strings and the refusal strings stay byte-identical.
- The refresh backup/rollback path keeps its all-or-nothing behavior, including the
  symlink-never-written guard and the partial-backup repair.
- Existing gate tests pass against the shared implementation; the root build and the full
  validation pass.
- The four trust-root digests match a rebuild through `scripts/build-gate.sh` under go1.24.13.
- The complete authored candidate stays within the 1200-line budget.

## Checks
- Gate: `go -C catalog/recipes/worktree-flow/gate test ./... -count=1` and `go vet ./...` there.
- Bridge: `python3 -m unittest tests.test_hook_gate_bridge` (33 test methods in 4 classes; 27 run plus a class-level skip without a gate binary, all 33 with `WORKTREE_GATE_BIN` set).
- Refresh E2E: the `GateRefreshCli` cases in `tests/test_sync_pipeline.py`, plus
  `tests.test_tracker_card_gate_hook`.
- Root: `go test ./cmd/... ./internal/...`, `gofmt -l` and `git diff --check`.
- Trust root: `scripts/build-gate.sh` plus `scripts/verify-gate-sums.sh`.
- Full: `./tests/validate.sh` with durable logs and exit evidence.
- Native review uses the exact committed slice base, not the whole epic.

## Progress
- Slice prepared: worktree `change/go-07-s12-hook-core` created at base `8efcc7a`.
- The trust root was verified against a fresh canonical rebuild before any gate edit:
  `scripts/build-gate.sh` with go1.24.13 plus `scripts/verify-gate-sums.sh` reported
  `ok — 4 digest entries match the committed trust root`.
- Baseline focused run on the untouched tree:
  `go -C catalog/recipes/worktree-flow/gate test -run Hook ./... -count=1` reported `ok`.
- Tracker unavailable; see the Tracker section for the host verdict and for why no
  `tracker.none` was written.
- Delivery A implemented by a delegated writer: `gate/shared/hookactuator.go` (430 lines)
  carries the moved decision core, and `gate/hookgateactuator.go` is now the thin JSON
  wrapper (136 lines, down from 480) with the historical aliases. The refresh write seam
  is forwarded from main as a function value, because a cross-package variable cannot be
  reassigned by the unchanged test. Measured 900 additions plus deletions, inside the
  1200-line budget.
- Trust root regenerated with go1.24.13: exactly four digest lines replaced in
  `bin/SHA256SUMS`, and `scripts/verify-gate-sums.sh` reports `ok` against an independent
  rebuild.
- Independent verification: PASS. The moved bodies are textually identical apart from the
  mechanical package and visibility changes; every refusal, warning and message string is
  byte-identical, and the I/O diagnostics are reassembled with the same bytes by the
  wrapper; the stdout field names and order and the exit codes are unchanged; the shared
  helpers are reused with no second port; and the new nil guard cannot fire on any
  existing path. Gate suite `ok` (46.8s), `go vet` clean, root module `ok`, and the bridge
  tests pass with and without a real gate binary.
- The nil guard and the forwarded seam are the only new code; no behavior change on any
  existing path was found. Full validation and the native review remain pending.

## Rollback
Revert this delivery's gate extraction, its four checksum entries and this document
together. Delivery B must not integrate before this shared API exists.
