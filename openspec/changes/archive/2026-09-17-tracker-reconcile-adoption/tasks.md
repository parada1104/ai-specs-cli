# ODD tracking bridge: tracker reconciliation adoption

Depth: light

This was an Organic Driven Development change. This bridge preserves the tracker link and delivery evidence for the repository guardian; it is not an SDD proposal/spec/design cycle.

## Scope

1. Ship Trello recipe-owned lifecycle defaults with optional project overrides.
2. Record the provider snapshot on explicit ledger close.
3. Keep the observation payload contract aligned between the Trello SKILL and Go gate.
4. Report an explicitly closed item after close without making closed rows primary for new work (D17).
5. Validate the complete lifecycle against a real Trello card.

## Acceptance

- Recipe defaults propagate on sync and existing project values remain overrides.
- Missing/stale/invalid observations do not agree.
- Close records the observed provider state and the post-close grade reports the closed row.
- Closed rows remain excluded from normal primary selection.
- Full validation and independent verification pass.

## Tracker

- **card_id**: 6aab1f85f9522bc30e37f201 (#130)
- **card_id**: 6aab1f85b36328d49ad56bd5 (#131)
- **card_id**: 6aab1f8627ffa6bf4779df11 (#132)
- **card_id**: 6aab1f86ed05fa12af190cdb (#133 acceptance)
- **pr**: https://github.com/parada1104/ai-specs-cli/pull/249
