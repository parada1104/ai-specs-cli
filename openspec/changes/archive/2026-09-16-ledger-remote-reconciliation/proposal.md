# Proposal: ledger remote reconciliation

## Objective

Make remote reconciliation of the tracker ledger deterministic and provider-neutral: MCP transports observations, recipes declare provider adaptation, and the Go gate owns validation and comparison. The agent coordinates and presents explicit decisions; nothing is inferred.

## Problem

Ledger provider snapshots were inert (`Item.State`/`Item.Provider` written by `link`, read by nothing), and identity evidence (`Evidence.Conflict`) compares only item ids, so provider state/list drift (for example: card closed locally while still `In Progress` on Trello, as observed in the PR #247 pilot) could never be detected.

## Scope

- T1: provider-neutral Go comparator `ledger.Reconcile` (fail-closed: invalid clock/window/declarations, duplicates, unbound identity; deterministic ordering; legacy `Evidence` semantics untouched).
- T2: explicit off-hot-path CLI comparison `--ledger --reconcile <observation.json> --reconcile-event <event>`; recipe-declared `[config.reconcile]` (scope_field, max_age_seconds, expectations) as first-class validated structured config; read-only sidecar; mutation guards; bounded tomllib acquisition; freshness overflow bound.
- T3: pending-decision protocol (notify + explicit user decision; headless leaves pending; no inferred consent; unrelated work continues; `agree` is never delivery proof).
- T4: trust-root regeneration + full validation + docs/CHANGELOG reconciliation.

## Out of scope

New Jira/OpenProject providers, a new agent loop, automatic provider writes, network on edit hooks, a second grader, closed-item/post-merge binding (named gap: reports `unbound-identity`), `Published`-list mapping (board has Review/Done only).

## Acceptance criteria

Missing, stale, unavailable, malformed or out-of-window observations never agree; unsupported mappings report `unmapped-event`/`unconfigured` and never agree; the sidecar never changes the graded exit code, never writes the store, and never records a decision; configuration travels through sync without false warnings.

## Tracker

- **card_id**: `6aaad60a09eb8ba066120ba3`
- **url**: https://trello.com/c/UfTdfS1Y
