# Tracker bridge (ODD)

Depth: light

This change follows ODD by explicit user decision; this file only satisfies the plan-build-gate classification contract and links the board card.

## ODD tracker

- Authoritative task file: `odd/tasks/tracker-recipe-default-mapping.md`
- Trello: #130 (https://trello.com/c/vm5gVI9P)

## Plan

1. Recipe-owned default lifecycle mapping: `review_list` (default Review), `done_list` (default Done) config fields; reconcile expectations for events `review` and `merge`.
2. SKILL documents recipe-supported events (delivery, review, merge).
3. Dogfood: `ai-specs sync` in this repo propagates defaults + reconcile block; `--reconcile` resolves without per-project config.

## Tracker

- **card_id**: 6aab1f85f9522bc30e37f201
- **url**: https://trello.com/c/vm5gVI9P
