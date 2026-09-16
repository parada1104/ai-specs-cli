# tracker-observation-step (card #132)

Goal: the observation producer contract is pinned — the SKILL documents the recipe-default lifecycle events and the payload shape it produces is exactly what the gate accepts.

## Tasks

- [x] T1: Go parity contract `TestLedgerReconcileSkillObservationShapeIsCurrent` — SKILL json payload keys == Go closed shape (both directions; catches silent drift)
- [x] T2: Python doc contract — SKILL states review_list/done_list defaults, review/merge events, override-only config (stacked on #130)
- [x] Focused suites green (trello recipe 14 OK; gate + ledger OK)
- [ ] T3: full validation at deliverable boundary + commit

## Notes

- Stacked on `feat/tracker-recipe-default-mapping` (needs its SKILL/config changes).
- No new agent loop, no CLI acquisition verb; the producer remains the documented MCP read + mktemp payload, now contract-pinned.

## Tracker

- **card_id**: 6aab1f8627ffa6bf4779df11
- **url**: https://trello.com/c/9T5utq44
