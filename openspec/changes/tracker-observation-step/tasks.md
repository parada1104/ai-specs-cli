# Tracker bridge (ODD)

Depth: light

This change follows ODD by explicit user decision; this file only satisfies the plan-build-gate classification contract and links the board card.

## ODD tracker

- Authoritative task file: `odd/tasks/tracker-observation-step.md`
- Trello: #132 (https://trello.com/c/9T5utq44)

## Plan

1. SKILL documents the recipe-default lifecycle events for observation (delivery, review, merge) so the agent produces observations against the mapping it will be compared with.
2. Parity contract: a Go test pins the SKILL-documented observation payload key set against the Go closed shape, so skill and gate cannot drift.
3. Focused suites.

## Tracker

- **card_id**: 6aab1f8627ffa6bf4779df11
- **url**: https://trello.com/c/9T5utq44
