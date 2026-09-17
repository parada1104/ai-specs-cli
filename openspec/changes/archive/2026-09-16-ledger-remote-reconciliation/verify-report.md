# Verify report: ledger remote reconciliation

## Commands and observed outcomes

| Check | Outcome |
|---|---|
| `./tests/validate.sh` (full suite) | exit 0 — `Ran 2026 tests`, `OK (skipped=2)`; skips pre-existing environment gates |
| `go -C catalog/recipes/worktree-flow/gate test ./... -count=1` | ok (main ~31s, ledger ~2.7s), post-correction |
| `bash scripts/build-gate.sh` | exit 0 — darwin/arm64, darwin/amd64, linux/amd64, linux/arm64 + native current (go1.24.13) |
| `bash scripts/verify-gate-sums.sh <dist sums> <committed>` | exit 0 — 4 digests match; parent reproduced |
| Native review | lineage `review-9ea19d90df2d6041`: 4 lenses (risk/resilience/readability/reliability); one candidate-caused CRITICAL (R1-001) corrected in `bc37c53`; targeted validator approved; acknowledgement burned |

## TDD evidence (per task)

- T1: comparator RED (25-case table against stub) → GREEN; correction round RED (12 silent-agreement regressions) → GREEN (`reconcile_test.go`, 36 table cases + 3 determinism tests).
- T2: CLI wiring RED (9 behavior tests) → GREEN; boundary corrections (overflow bound 9223372036 s, mutation-flag refusal, disabled-recipe rejection, bounded parser, conflict-snapshot suppression, observation budget) each RED → GREEN; recipe structured-config RED (12 failures) → GREEN.
- T3: contract test pinning decision inputs for all 14 reachable non-agree outcomes (no behavioral change needed; contract already held).

## Known opens (follow-ups, non-blocking)

- Closed-item/post-merge binding reports `unbound-identity` (explicit closed-item selection contract pending).
- `recipe configure --set` cannot update an existing dotted-style `[config.reconcile]` block (fails safe).
- `Published` list mapping unresolved (board has Review/Done only).
- 7 non-blocking advisory findings from native review (R1-002, R2-noobs-test-control, R2-structured-list-limit, R3-empty-event-agreement, R3-runtime-list-bound, R4-empty-event-agreement, R4-unbounded-manifest-input).

## Defects diagnosed during the run (harness, out of scope here)

`gentle_review assess` masks actionable native stderr as `EMPTY_OUTPUT`; `rctx2` repository context invalidation by uncommitted workspace drift (fix: keep candidate tree clean between context minting and capture).
