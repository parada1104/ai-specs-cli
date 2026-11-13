# Go 07 S12.3 — Native hook actuation authority

## Objective
Add the native root authority for the hook actuator in `internal/sync`.
Delegate the decisions to the shared actuator extracted in delivery A and reuse the
shared classify core instead of re-porting classification.
Verify against the real Python oracle. Keep the sync CLI unwired until S14.

## Tracker
- **Unavailable.** The Trello API key is missing or expired in this environment, so no
  card could be created or linked for this slice.
- Host check at base `a183ec9`:
  `bash catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh --root <worktree> --checkpoint pre-merge`
  printed `tracker-card-gate: tracker-ledger: no verified gate binary resolved; failing open (run ai-specs sync / ai-specs doctor)` and exited 0.
- The tracker host is advisory by design and its `identity_unavailable` rule is to report
  and proceed without recording a decision. `tracker.none` is presentation and evidence
  only and is never written by the host, so none was created here.
- Close-out follow-up: link the S12.3 card in this section once the tracker is reachable.

## Scope
- Base: `a183ec9` on `epic/go-single-binary` (S12.1 copy, S12.2 templates and docs, and
  S12.3 delivery A's shared hook actuator all merged).
- Worktree: `.worktrees/go-07-s12-hooks`. Branch: `change/go-07-s12-hooks`.
- Add the native hook actuation authority: destination resolution, rendering through the
  shared package, classification through the shared core, the write plus mode, the record
  payload, and the refresh backup/rollback path, returning record payloads without writing
  a lock or manifest entry of its own.
- The shared package is frozen: delivery A owns it and this delivery must not change it.
- Do not wire the sync CLI: S14 owns orchestration and caller diagnostics.

## Constraints
- Budget: 1200 additions plus deletions, including tests and this document.
- Behavior tests require observed RED, GREEN, then proportionate refactoring.
- Do not modify production Python: Python remains the differential oracle.
- Reuse the shared classify core and the shared actuator; no second port.
- Do not modify the gate module in this delivery. Its four digests are already canonical for
  the merged sources; if a gate change became necessary, stop and report instead.
- Run the full validation before committing; the parent owns the commit, the native review,
  the pull request and the merge.

## Tasks
- [x] T1: Scout the Python oracle and the merged delivery A API; record the exact entry points. **Done**; the anchors are recorded in Progress.
- [x] T2: Add the native authority contract tests and observe RED. **Done**; RED appeared as a build failure on undefined `HookRequest` and `MaterializeHook`.
- [x] T3: Implement, then observe GREEN and run discrimination probes. **Done**; two probes each broke a named scenario and were restored.
- [x] T4: Differential coverage against the real Python entry points, including the refresh
      backup/rollback path and the rel-path escape refusals. **Done**; nine scenarios plus refusal and
      refresh-rollback tests.
- [x] T5: Run the focused suites and the full validation; the parent then commits and runs
      the native review. **Done**; full validation exit 0 and independent verification PASS.
- [ ] T6: Parent publishes and merges into the epic, then cleans the worktree.

## Acceptance criteria
- Native outcomes match the Python oracle per scenario: write, seed, skip, user-modified
  warning, managed-stale refresh, refresh backup/rollback, policy refusals and rel-path escapes.
- Exact Python strings preserved for lines, warnings and info messages, and the eight
  placeholders stay byte-identical.
- Classification goes through the shared core, with no second port.
- The refresh path keeps its all-or-nothing behavior, never writes through a symlink, and
  repairs a partial backup.
- Root tests, `go vet` and the full validation pass, with zero parity deltas in both gate modes.
- The complete authored candidate stays within the 1200-line budget.

## Checks
- Focused RED/GREEN: `go test ./internal/sync/ -count=1 -v` (hook-focused run).
- Root suite: `go test ./cmd/... ./internal/...` and `go vet ./internal/sync/`.
- Gate suite unchanged: `go test ./...` inside the gate module.
- Bridge: `python3 -m unittest tests.test_hook_gate_bridge` with and without `WORKTREE_GATE_BIN`.
- Refresh E2E: the `GateRefreshCli` cases in `tests/test_sync_pipeline.py`.
- Full: `./tests/validate.sh` with durable logs and exit evidence.
- Native review uses the exact committed slice base, not the whole epic.

## Progress
- Slice prepared: worktree `change/go-07-s12-hooks` at base `a183ec9`, which already contains
  delivery A's shared hook actuator (`shared.MaterializeHook`, `HookActuatorRequest`,
  `HookActuatorResult`).
- Python oracle anchors recorded: dispatcher `materialize_hook_script`
  (`lib/_internal/recipe-materialize.py:2085`) and the Python body
  `_python_materialize_hook_script` (`:2233`), which the differential tests must drive.
- Tracker unavailable; see the Tracker section for the host verdict and for why no
  `tracker.none` was written.
- Delivery B implemented by a delegated writer: `internal/sync/hookactuator.go` (84 lines) maps a
  request onto `shared.HookActuatorRequest` and delegates to `shared.MaterializeHook(in, nil)`; the
  nil refresh seam resolves to the shared production writer (`shared/hookactuator.go:270-271`), so
  the root re-ports no destination resolution, rendering, classification, lock or manifest logic.
- Tests: `internal/sync/hookactuator_test.go` (686 lines) with four top-level tests and thirteen
  subtests, including nine differential scenarios (h1 fresh write, h2 managed-current, h3
  no-provenance, h4 user-modified, h5 managed-stale refresh, h6 refresh backup, h7 refresh rollback,
  h8 dangling-symlink guard, h9 refresh-symlink guard) that drive the real Python body
  `_python_materialize_hook_script` through python3 and `lib/_internal/recipe-materialize.py`,
  comparing wrote, destination identity, record fields, on-disk bytes, mode, backup path and hash,
  every emitted line and refusal presence.
- RED was observed before implementing (build failure on the undefined API), and two discrimination
  probes each broke a named scenario (a nil managed entry broke h2, and `Refresh: false` broke the
  refresh backup) before being restored.
- Independent verification: PASS. The wrapper re-ports nothing, the nil seam cannot change behavior
  versus the exported writer, the differential is non-vacuous and drives the true entry points, and
  the three declared risks do not affect this delivery: the normalized refusal still compares the
  post-state while the refusal strings stay pinned byte-exactly in
  `TestMaterializeHookRefusalsAndBoundary`; the CRLF divergence is latent because no CRLF hook source
  exists and hashing folds CRLF on both sides; and the rel-path guard is declared Go-only hardening
  with no differential scenario depending on it. The RED and probe chronology is not independently
  verifiable from the tree and rests on the writer's report.
- Focused: `go test ./internal/sync/ -count=1` ok, the verbose hook run reports thirteen subtests,
  `go vet` and `gofmt` clean, the root module is ok for sixteen packages, and the bridge tests stay
  green and unmodified (27 ran plus one class skip).
- Measured 770 additions plus deletions, inside the 1200-line budget.
- Full validation: `./tests/validate.sh` exit 0, `Ran 2380 tests in 1607.302s`, `OK (skipped=164)`,
  and parity 29 fixtures with zero deltas in both gate modes. Durable evidence:
  /Users/robert/.cache/s12-hookB-validate.log and .exit.
- Native review and publication remain pending.

## Rollback
Revert this delivery's new `internal/sync` files and this document together.
Delivery A's shared API stays, and nothing else depends on delivery B yet.
