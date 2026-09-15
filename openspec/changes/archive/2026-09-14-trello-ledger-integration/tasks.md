# Tasks: trello-ledger-integration

Inputs: `proposal.md` (L1–L7), `specs/tracker-ledger/spec.md` (delta), `design.md` (DW1–DW5 closed), `openspec/config.yaml` (`strict_tdd: true`).
Commands: focused `./tests/run.sh`, final `./tests/validate.sh`, Go focused `go -C catalog/recipes/worktree-flow/gate test ./...`, trust root `scripts/build-gate.sh` → `scripts/verify-gate-sums.sh`.

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~1,500–2,200 authored (Go writer+tests ≈ 500–700; bridge+hosts+corpus ≈ 600–800; lookup+docs ≈ 250–400). Generated trust root (`bin/SHA256SUMS`, `dist/worktree-gate-current`) excluded from the authored count. |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 (Phase 1 Go writer + trust root) → PR 2 (Phase 2 bridge + hosts + `tracker.none` + corpus) → PR 3 (Phase 3 witness lookup + docs) |
| Delivery strategy | exception-ok (explicitly accepted by the user after the high-risk forecast) |
| Chain strategy | size-exception |

```text
Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: size-exception
400-line budget risk: High
```

The 1200-line session review budget is also exceeded by every honest estimate; the three phases below are the review units named in the proposal and must not be merged into one diff. **Phase 1 must not split from its trust-root regeneration** — a reviewer must never see a changed binary without matching digests.

## Tracker

- **card_id**: `6aa703fdcf61a90ec702d58b`
- **url**: https://trello.com/c/ie4mQykZ/127-feature-trello-ledger-integration-make-bound-tracker-a-real-ledger-adapter
- **list**: In Progress · parent epic card 125

## Work Units

| Unit | Start state | Finished state | Verification | Rollback |
|---|---|---|---|---|
| WU-1 (Phase 1) | `--ledger` is verdict-only; `OpenItem`/`CloseItem`/`AppendDecisionToPrimary` have no production callers; unbounded `flock` | `--write open\|link\|close\|exempt` on the existing flow, open-if-absent under a bounded lock, `ledgerSelftest` extended, four-arch trust root rebuilt | `go test ./ledger/ -race -count=3`; `scripts/verify-gate-sums.sh`; `./tests/run.sh` | Revert the Go files + regenerate `bin/SHA256SUMS` and `dist/worktree-gate-current` in the same revert commit. Nothing outside `catalog/recipes/worktree-flow/gate/**` + trust root is touched. |
| WU-2 (Phase 2) | No host passes `--evidence`; `tracker.none` writes nothing | `lib/_internal/ledger_bridge.py` builds 3-sided evidence; gate hook + guardian pass `--evidence` and issue the host `exempt` write; corpus rows pinned | `tests/test_ledger_evidence.py` + hook/guardian tests; `tests/test_tracker_ledger_parity.py` (corpus 1:1 with `DESIGN_ROWS`) | Revert bridge + host edits + new fixtures; grader still works verdict-only (today's behavior). No store schema bump to unwind. |
| WU-3 (Phase 3) | Recipe id hardcoded in 3 production sites; docs describe verdict-only | `bridge.recipe_id(root)` drives mode lookup with legacy fallback; doctor reports unhosted `work-start`; docs/README/CHANGELOG honest about the 3-of-4 gap | `tests/test_ledger_mode_config.py`, `tests/test_doctor_tracker_card.py`, `tests/test_manifest_contract_docs.py`; `./tests/validate.sh` | Revert the three lookup sites + doc rows; behavior is byte-identical to today when the witness is absent. |

---

## Phase 1 — Go write surface, bounded lock, trust root (WU-1)

Scope: `catalog/recipes/worktree-flow/gate/{main.go,ledger_cmd.go,ledger/store.go,ledger/decide.go,ledger/write_test.go,ledger/decide_test.go}` + `catalog/recipes/worktree-flow/bin/SHA256SUMS` + `dist/worktree-gate-current`. No provider call, no network, `go.mod` unchanged.

- [x] 1.1 RED — add `catalog/recipes/worktree-flow/gate/ledger/write_test.go` pinning the invariants before any production edit: retried `open` yields exactly one open item (`applied=false`/`already-open`); two same-second `open` writes end with one item and unique ids; `link` identical to current core fields + canonical provider bytes appends no duplicate; `link` with changed fields populates `Item.{ItemID,URL,NativeType,State,Provider}` and appends `DecisionLink`; `close` then `close` → `already-closed`, item stays in store; `close` with no row fails closed; `change-ambiguous` without payload `change` persists nothing; held `state.json.lock` → `ErrLockTimeout` within ~100 ms; `--decide kind=adjudicate` clears `Item.Exemption`. Run `go -C catalog/recipes/worktree-flow/gate test ./ledger/ -count=1` and record the failing output as RED evidence. <!-- sdd-owner: implementation -->
- [x] 1.2 GREEN — implement in `ledger/store.go`: bounded `withStoreLock` (5 × `LOCK_EX|LOCK_NB`, 20 ms sleep, `ErrLockTimeout`), `OpenIfAbsent`, `uniqueItemID` (`\x1fN` suffix on the stamp input until unique — do not change unlocked `OpenItem`), and the single locked `ApplyWrite` lifecycle in design order (refuse ambiguous/unavailable → open/link/close/exempt → atomic save → re-grade). Keep `exempt` on `OpenIfAbsent` + `Item.Exemption` without adding a sixth `decisionKinds` entry. Same focused test command; record GREEN. <!-- sdd-owner: implementation -->
- [x] 1.3 GREEN — wire the CLI vehicle: `--write JSON` beside `--decide` in `main.go` / `ledgerOptions`, `WriteRequest` (`Normalize`/`Validate(checkpoint)`) and payload rules in `ledger_cmd.go` (`kind` enum; `item_id` required for `link`; non-empty trimmed `reason` required for `exempt`; `provider` default `{}`; `change` required under `change-ambiguous`), mutually exclusive with `--decide` (both → exit 2, no persist), stdout sidecar `"write": {"kind","applied","reason"}` only on success, and stderr `worktree-gate: ledger --write failed:` + exit 2 with **no** stdout JSON on failure. <!-- sdd-owner: implementation -->
- [x] 1.4 GREEN — change `ledger/decide.go`: `PersistDecision`/`PersistConflict`/`AppendDecisionToPrimary` consume the bounded lock, `adjudicate` clears `Item.Exemption`, and a lock timeout is fail-closed for write/decide paths while grade paths still fail open. <!-- sdd-owner: implementation -->
- [x] 1.5 TRIANGULATE — extend `ledger_cmd_test.go`/`ledger/write_test.go` with the adversarial cases the design names: invalid payload vs unknown flag (flag-parse error on a *verdict* call still fails open, post-parse validation fails closed); byte-identical store after every failed write (compare file bytes + mtime); empty cwd vs explicit `--project-root` for writes; no `state.json.tmp.*` residue; `-race -count=3` clean. <!-- sdd-owner: implementation -->
- [x] 1.6 REFACTOR — under green, keep `Grade` pure (assert no write path reaches it from grading), reuse the existing `--decide` marshal/exit helpers rather than duplicating verdict output, and drop any speculative field. Re-run `go -C catalog/recipes/worktree-flow/gate test ./...` plus `gofmt -l catalog/recipes/worktree-flow/gate` clean. <!-- sdd-owner: implementation -->
- [x] 1.7 GREEN — extend `ledgerSelftest()` in `ledger_cmd.go` to exercise the write-request validation and the open-if-absent invariant in-process, offline, and keep the existing success marker intact. <!-- sdd-owner: implementation -->
- [x] 1.8 Verify + trust root — run `--selftest` on a built binary, then `scripts/build-gate.sh` (all four targets) and `scripts/verify-gate-sums.sh`; regenerate `catalog/recipes/worktree-flow/bin/SHA256SUMS` and `dist/worktree-gate-current` **in the same commit as 1.2–1.7**. Confirm `go.mod` and `go.sum` are unchanged. <!-- sdd-owner: implementation -->

## Phase 2 — Evidence bridge, host wiring, `tracker.none` (WU-2)

Depends on Phase 1 (needs the shipped `--write` verb and rebuilt `dist/worktree-gate-current`). Scope: `lib/_internal/ledger_bridge.py` (new), `recipe-materialize.py`, `tracker-card-gate.sh`, `premerge_guardian.py`, `tests/fixtures/tracker-ledger-corpus/**`, parity/host tests. Never grades, never calls `gh`/MCP/network.

- [x] 2.1 RED — add `tests/test_ledger_evidence.py`: `evidence_payload(root, slug)` returns the four keys `{local,remote,code,git}` with `code` = `## Tracker card_id`, `git` = the same id **iff** `pr:` is recorded else `""`, `remote` always `""`, `local` always `""` from the bridge; `tracker.none` present → `code=""` and `tracker_none_reason` = first non-empty line (else literal `tracker.none`), `None` when absent/unreadable; missing/malformed `## Tracker` → empty sides, no exception; no `gh`/MCP/socket call. Focused command: `python3 -m unittest tests.test_ledger_evidence -v`. <!-- sdd-owner: implementation -->
- [x] 2.2 GREEN — create `lib/_internal/ledger_bridge.py` with `recipe_id`, `tracker_none_reason`, `evidence_payload`, importing `trello_link.parse_tracker_section` as the only parser (bridge stays acquisition-only, foundation A8) and swallowing every read error to an empty side. <!-- sdd-owner: implementation -->
- [x] 2.3 RED — extend `tests/test_tracker_card_gate_hook.py`: with the existing `STUB_BINARY` log extended to capture `--evidence`/`--write`, assert (a) the graded argv carries `--evidence <path>`, (b) a present `openspec/changes/<slug>/tracker.none` produces a `--write kind=exempt reason=…` line **before** the grade line, (c) a `--write` that exits 2 fails the checkpoint closed even under `warn`, and (d) an empty/absent `__TRACKER_LIB_INTERNAL__` stamp skips both `--evidence` and the exempt write (fail open). <!-- sdd-owner: implementation -->
- [x] 2.4 GREEN — wire `catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh`: resolve the bridge via a new `__TRACKER_LIB_INTERNAL__` stamp (same materialize path as `__TRACKER_CLI_HOME__`), build the evidence file in tmp, issue the host-owned `exempt` write when the file exists (DW1: host owns it; the hook never creates or deletes `tracker.none`), then pass `--evidence` on the grade call. Keep `openspec/**` exempt and the exit-code contract (`0` allow/ask/dormant/unevaluable, `2` block/failed write) verbatim. <!-- sdd-owner: implementation -->
- [x] 2.5 RED/GREEN — `lib/_internal/premerge_guardian.py`: extend `tests/test_premerge_guardian.py` to assert `ledger_blockers` passes `--evidence` and the exempt write at `pr-review`/`pre-merge`/`archive-close`, honors write exit 2, and still works from a cold `AI_SPECS_HOME` with no project cache; then implement via sibling import of `ledger_bridge.py` using the `Path(__file__).with_name(...)` pattern already used by `gate_binary.py`. Leave `work-start` (`catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh`) untouched — no `--evidence`, no exempt write (L5). <!-- sdd-owner: implementation -->
- [x] 2.6 RED — add corpus fixtures `tests/fixtures/tracker-ledger-corpus/26-open-then-allow.json` … `33-write-failure-exit-2.json` for the eight design rows: open-then-allow, code-vs-ledger conflict, `tracker.none` exemption, idempotent re-open, link mismatch, same-second item-id collision, `change-ambiguous` refuse, write-failure — each with a 1:1 `DESIGN_ROWS` entry in `tests/test_tracker_ledger_parity.py` (the coverage test at `test_corpus_covers_every_design_row` is the gate). Run the parity suite against the rebuilt `dist/worktree-gate-current` and record RED. <!-- sdd-owner: implementation -->
- [x] 2.7 GREEN/TRIANGULATE — make the new rows pass; then verify the conflict predicate **fires in production shape**: the code-vs-ledger row must reach `ReasonConflict` through both the apply-start and pre-merge hosts with `remote` absent, and `test_no_store_residue_and_no_unexpected_ledger_files` must still see only `{witness.json, state.json, state.json.lock}`. Add one row that grades twice and proves byte-stable output after `recorded_at` normalization. <!-- sdd-owner: implementation -->
- [x] 2.8 REFACTOR — remove any duplicated `## Tracker` parsing introduced in 2.4/2.5 (must call the bridge, not re-parse), confirm no second predicate exists (`grep -n "card_id" lib/_internal catalog/recipes/trello-mcp-workflow/hooks`), and re-run `python3 -m unittest tests.test_ledger_evidence tests.test_tracker_card_gate_hook tests.test_premerge_guardian tests.test_tracker_ledger_parity -v`. <!-- sdd-owner: implementation -->

## Phase 3 — Witness-derived provider lookup + docs (WU-3)

Depends on Phase 2 (uses `bridge.recipe_id`). Smallest unit; behavior-identical when no witness exists.

- [x] 3.1 RED — extend `tests/test_ledger_mode_config.py` and `tests/test_doctor_tracker_card.py`: with a witness whose `recipe_id` is a non-legacy fixture recipe, the effective `ledger_mode`/`gate_mode` come from `recipes.<witness-id>.config` (and `TRACKER_LEDGER_MODE` still wins); with the witness missing/unreadable, behavior matches today exactly. <!-- sdd-owner: implementation -->
- [x] 3.2 GREEN — replace the three literal lookups with `bridge.recipe_id(root)`: `lib/_internal/premerge_guardian.py:501`, `catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh:493`, `lib/_internal/doctor.py:718` (`_tracker_ledger_in_play`), keeping `trello-mcp-workflow` only as the fallback constant. <!-- sdd-owner: implementation -->
- [x] 3.3 GREEN — doctor: add one INFO line when the witness is bound and `plan-build-flow` is not enabled — `work-start is unhosted` — while the other four checkpoints keep grading (spec "Unhosted work-start is visible"). Doctor issues no writes. <!-- sdd-owner: implementation -->
- [x] 3.4 TRIANGULATE — name the residue instead of silently leaving it: `catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh:104` still resolves config by the `trello-mcp-workflow` literal and is **out of scope by L5** (no content change under that recipe). Record it in `docs/capabilities.md` as remaining work so success criterion 8 is met honestly, and pin the count with a test-time `grep` assertion limited to the three in-scope sites. <!-- sdd-owner: implementation -->
- [x] 3.5 Docs (user-facing change → required) — update `docs/capabilities.md`, `docs/runtime-hooks.md`, `catalog/recipes/trello-mcp-workflow/README.md` + its `skills/trello-mcp-workflow/SKILL.md` `## Tracker` section, and `CHANGELOG.md`: the `--write open|link|close|exempt` verb, explicit-open-only (grading never writes), 3-of-4 evidence with `remote` **unwired**, `tracker.none` → `Item.Exemption` honored at every checkpoint, the checkpoint→host owner map (work-start plan-build-flow; apply-start/pr-review tracker card gate; pre-merge/archive-close premerge guardian), and the write-fail-closed / grade-fail-open postures. Fix the stale `tracker-card-gate.sh` header comment (`archive` is not a shell kind here) in this same unit. <!-- sdd-owner: implementation -->

## Phase 4 — Apply-owned verification (per PR, before review)

- [x] 4.1 Smoke — run `./tests/validate.sh` (py_compile + `bash -n` + gofmt + Go tests + full unittest discovery) and record the exact tail output; state explicitly that coverage, linter, type-checker, and formatter are unavailable in this repo rather than implying they passed. <!-- sdd-owner: implementation -->
- [x] 4.2 Spec-to-code walk — for each of the delta's ADDED/MODIFIED requirements, name the file or test that satisfies it (or `N/A` with reason), including "Grading never writes" (byte-identical store assertion) and the five preserved-behavior invariants: exit-code contract, `openspec/**` never blocked, no-TTY `ask` still exit 2 (L4), corrupt store → `unevaluable` + doctor ERROR, this project's `warn` posture. <!-- sdd-owner: implementation -->
- [x] 4.3 Review-size check — record authored additions + deletions per PR against the 1200-line session budget (400 canonical); if Phase 1–3 cannot each land under budget as cohesive units, stop and surface the delivery decision (`ask-on-risk`) rather than shrinking comments, tests, or docs. <!-- sdd-owner: implementation -->

## Deliberately not tasks (non-goals — do not widen)

No provider create/update/move/comment/label; no `remote` producer or MCP read (L3); no checkpoint host relocation and no content change under `catalog/recipes/plan-build-flow/**` (L5); no Jinna adapter; no second module/binary/`SHA256SUMS`/release asset; no `## Tracker` schema or provider vocabulary change; no archive rewrite or store schema bump; no compaction/retention work; no `unexempt` decision kind.
