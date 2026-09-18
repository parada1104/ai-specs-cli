# Tracker ledger follow-ups

## Objective
Finish the small post-0.23.0 tracker-ledger follow-ups while keeping provider mapping recipe-owned and project configuration limited to local list names.

## Problem
The remote reconciliation mapping is conceptually provider-owned, but its generated manifest representation is too visible and the recipe configuration writer cannot safely update existing structured tables. Published lifecycle mapping also needs an optional, non-inferred path. The explicit close-report behavior is already shipped and must remain D17-safe.

## Why now
The user authorized follow-ups 1–3 and selected the internal recipe-owned mapping boundary.

## Scope
- Verify and regression-pin the shipped closed-item/post-merge close-report behavior; extend only the safe remote comparison path if current evidence confirms a remaining gap.
- Make `recipe configure --set` update existing structured configuration tables without corrupting or duplicating TOML; preserve explicit values and fail closed on malformed input.
- Add optional Trello `published_list` configuration; when configured, the recipe-owned `merge` expectation targets Published, while projects without it retain `merge` → Done.
- Keep `config.reconcile` recipe-owned: project-facing configuration exposes list values, while sync/materialization owns the generated mapping.

## Constraints
- No provider writes or automatic Trello mutations.
- No inferred Published list; absent `published_list` remains unconfigured/pending.
- Preserve D17: closed rows never become the primary for new work.
- Keep the generic Go comparator provider-neutral.
- TDD mode: enabled by project configuration; runner: `./tests/validate.sh`.
- Delivery strategy: ask-on-risk; use one feature branch and reviewable work-unit commits.

## Tracker

- **card_id**: `6aab1f8627ffa6bf4779df11`
- **url**: https://trello.com/c/9T5utq44
- **note**: Follow-up to the completed live tracker reconciliation acceptance card.

## Tasks

- [x] T0 — Reconcile current release evidence and preserve the shipped explicit close-report behavior.
- [x] T1 — Add regression coverage and, only if required by current behavior, safely bind a closed observation to its matching local item without changing Primary/D17.
- [x] T2 — Fix structured-table updates in `recipe configure --set` and add RED/GREEN regression tests.
- [x] T3 — Add optional `published_list` and recipe-owned conditional merge mapping; keep absent configuration on `merge` → Done and document the user-facing boundary.
- [x] T4 — Resolve verification blockers: update the structured-schema regression assertion and regenerate the canonical gate trust root for the Go source change.
- [x] T5 — Rerun focused checks and full validation, inspect generated output, and update this document and Engram evidence. Delivery commit remains pending explicit authorization.

## Acceptance criteria

- A closed item can be reported/reconciled only through a deterministic, locally corroborated binding; ambiguous or missing matches remain non-agreeing.
- Existing structured TOML tables are updated in place without duplicate keys, lost comments, or unsafe partial writes.
- `published_list` is optional; no default is fabricated, configured projects reconcile `merge` against Published, and unconfigured projects retain `merge` → Done.
- `config.reconcile` remains recipe-owned and is not required as a hand-authored project contract.
- Focused tests and `./tests/validate.sh` pass.

## Size exception authorization

The user explicitly chose one PR for this branch despite the 865-line authored diff. This exception is for delivery shape only; all focused/full verification and trust-root checks remain required.

## Progress

- Worktree: `.worktrees/tracker-ledger-followups`
- Branch: `feat/tracker-ledger-followups`
- Base: `development` at `03e227c`
- Current state: T1–T5 implemented and verified; no source blockers remain. Delivery commit remains pending explicit authorization.
- Evidence: focused Python suite — 119 tests OK; Go packages — OK; `./tests/validate.sh` — 2127 tests OK, 2 skipped; canonical trust-root verification with `bash scripts/verify-gate-sums.sh dist/SHA256SUMS catalog/recipes/worktree-flow/bin/SHA256SUMS` — OK.
- Failed/skipped check recorded: the initially listed bare `bash scripts/verify-gate-sums.sh` invocation exits 2 with usage because the script requires two arguments; the corrected contract passed. `dist/` remains generated/ignored.

## Next step
Close the work unit through the authorized delivery flow, then resume the next ledger or Go strangler slice.
