# ODD delivery record: tracker reconciliation adoption

## Objective

Make the Trello tracker audit usable without project-specific reconcile ceremony: the recipe owns safe lifecycle defaults, explicit overrides remain supported, close records the observed provider snapshot, and the ledger can report the row it explicitly closed.

## Delivered

- Recipe defaults for delivery, review, and merge lifecycle events.
- Sync propagation of recipe-declared reconcile mapping and referenced defaults, absent keys only.
- Snapshot-on-close and close-only post-grade reporting with D17 preserved.
- SKILL/Go observation payload parity contract.
- Live Trello acceptance on card #133.

## Non-goals

No new provider adapter, provider mutation from reconciliation, Published-list mapping, or SDD phase artifacts. The live acceptance run is verification of the existing recipe, not recipe incubation dogfood.

## Tracker

- **PR**: https://github.com/parada1104/ai-specs-cli/pull/249
- **Cards**: #130, #131, #132, #133 on https://trello.com/b/BTfTuT6W
