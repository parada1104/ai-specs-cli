# Tracker bridge (ODD)

Depth: light

This change follows ODD by explicit user decision; this file only satisfies the plan-build-gate classification contract and links the board card.

## ODD tracker

- Authoritative task file: `odd/tasks/tracker-snapshot-on-close.md`
- Trello: #131 (https://trello.com/c/J9aNJFGr)

## Plan

1. S1 snapshot-on-close: `WriteClose` records the observed provider payload (state/list) when present; bare closes keep the current behavior.
2. Focused Go tests (RED→GREEN), docs line in the Trello skill if the write payload contract changes.

## Tracker

- **card_id**: 6aab1f85b36328d49ad56bd5
- **url**: https://trello.com/c/J9aNJFGr
