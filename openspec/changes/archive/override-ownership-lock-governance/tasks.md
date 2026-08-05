# Tasks: override-ownership-lock-governance

Depth: **full**

Branch / worktree: `change/override-ownership-lock-governance` —
`.worktrees/override-ownership-lock-governance`

Plan refs: `explore.md`, `proposal.md`, `design.md`,
`specs/{sync-lock,override-ownership,recipe-schema,worktree-flow}/spec.md`

## Tracker

- **card_id**: `wdwyRFTS`
- **url**: https://trello.com/c/wdwyRFTS

**Stop for human authorization before any production code implementation.**

This file is the implementation plan only — do not write production code while
authoring/editing it. Await maintainer go-ahead before RED/GREEN apply.

---

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~500–900 (lock + classifier + materialize + doctor + tests + docs/specs) |
| 400-line budget risk | Medium–High (tests dominate) |
| Chained PRs recommended | Conditional if impl+tests exceed ~400 reviewable LOC |
| Suggested split | PR1 lock+classifier+materialize+tests → PR2 doctor+schema+docs+spec promote |
| 400-line budget risk | Medium–High (tests dominate) |

```text
Decision needed before apply: Yes (authorization gate)
Chained PRs recommended: Conditional (size)
400-line budget risk: Medium–High
```

---

## Phase 1 — Tests scaffolding (RED)

- [x] **T1.1** — Extend `tests/test_lock.py` (or sibling) for `[managed.*]`
  round-trip: write/load `sha256` + optional provenance fields; ensure skill/
  recipe/command hashes still not emitted.
  **Done when:** RED/collectable assertions for managed section exist.
- [x] **T1.2** `[P]` — RED classifier cases: missing, managed_current,
  managed_stale, user_modified, untracked match, untracked diverge
  (unit tests around new helper).
  **Req:** override-ownership classifier.
- [x] **T1.3** `[P]` — RED `materialize_template`: managed_stale + `auto`
  overwrites and updates lock; user_modified preserves + warns; untracked
  diverge preserves without seeding managed ownership.
  **Req:** sync actions + migration.
- [x] **T1.4** `[P]` — RED recipe_schema accepts `update_policy` values and
  rejects unknown.
  **Req:** recipe-schema delta.
- [x] **T1.5** `[P]` — RED doctor messaging aligned with classifier
  (user-modified vs managed-stale under confirm/never-force).
  **Req:** doctor.
- [x] **T1.6** — RED explicit refresh: delete target + sync reseeds + lock.
  **Req:** explicit refresh.
- [x] **T1.7** — Confirm Phase 1 RED evidence (failures for missing behavior,
  not import/syntax). Record command + summary.

---

## Phase 2 — Lock + classifier (GREEN)

- [x] **T2.1** — Implement `[managed.*]` in `lib/_internal/lock.py` (load/write,
  header comment). Preserve `[meta]` / `[agents.*]`; keep dropping skill hashes.
  **Done when:** T1.1 green.
- [x] **T2.2** — Implement shared classifier (util or small module); render-aware
  would-write comparison for placeholder templates.
  **Done when:** T1.2 green.

## Phase 3 — Sync + doctor + schema (GREEN)

- [x] **T3.1** — Wire `materialize_template` decision tree to classifier +
  policies; record lock on CLI write; keep hook always-overwrite regression.
  **Done when:** T1.3 + hook regression green.
- [x] **T3.2** — Update `doctor._check_stale_template_overrides` to shared
  classifier.
  **Done when:** T1.5 green.
- [x] **T3.3** — Parse optional `update_policy` on `TemplateRef` in
  `recipe_schema.py`.
  **Done when:** T1.4 green.
- [x] **T3.4** — Explicit refresh path (document `rm`+sync; optional
  `--refresh-managed` only if low-cost).
  **Done when:** T1.6 green.

## Phase 4 — Docs + canonical specs

- [x] **T4.1** — Document auto / confirm / never-force + hooks always-CLI in
  appropriate recipe/docs surface (trello + worktree-flow README or central
  sync docs).
- [x] **T4.2** — Promote deltas into canonical
  `openspec/specs/{sync-lock,override-ownership,recipe-schema,worktree-flow}/`.
- [x] **T4.3** — Verify DocRef `condition` behavior; wire or explicitly exclude
  docs from governance with a note in design follow-up / tasks evidence.

## Phase 5 — Verify

- [x] **T5.1** — Focused unit tests for lock/classifier/materialize/doctor/schema.
- [x] **T5.2** — Full `./tests/run.sh` and `./tests/validate.sh` green.
- [x] **T5.3** — Record verify evidence; ready for PR only after authorization
  + implementation complete + planning committed.

## Non-goals (do not implement)

- Resurrecting `[skills.*]` / `[recipes.*]` / `[commands]` lock hashes.
- Force-overwriting `user_modified` without explicit user delete.
- Interactive confirm TUI in v1.
- Changing recipe-overrides-runtime resolution order.
- Push/PR during planning.

## Authorization gate

**READY_FOR_AUTH: yes** — authorization received; implementation and full
validation are complete. Parent handles commit/release delivery.
