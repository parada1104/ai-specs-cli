# Tasks: agent-assisted-recipe-config

Depth: full

Branch / worktree: `change/agent-assisted-recipe-config` /
`.worktrees/agent-assisted-recipe-config/`

Plan refs: `explore.md`, `proposal.md`, `design.md`,
`specs/agent-assisted-recipe-config/spec.md`

**Stop for human authorization before production-code apply.** This file is the
implementation plan only — do not write production code or tests while
authoring it.

---

## Tracker

- **card_id**: `6a72b44828a5b2547f679116`
- **shortLink**: `GjfV4sKA`
- **url**: https://trello.com/c/GjfV4sKA

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~400–900 (skills/docs dominate if no new CLI; higher if helper+tests) |
| 400-line budget risk | Medium–High if non-interactive CLI ships in-scope |
| Chained PRs recommended | Conditional — split skill/docs vs helper/CLI if over budget |
| Suggested split | PR1 skill playbook + docs + content tests → PR2 helper/apply idempotence tests (if authorized) |
| Delivery strategy | Prefer smallest slice that meets acceptance; expand CLI only with evidence |
| Chain strategy | feature-branch-chain to `development` when over budget |

```text
Decision needed before apply: Yes (authorization gate)
Chained PRs recommended: Conditional (size / CLI choice)
Chain strategy: feature-branch-chain when over budget
400-line budget risk: Medium–High
```

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | Skill playbook + literacy cross-links + docs | PR1 | Meets NL entry + documentation |
| 2 | Apply/idempotence/preserve tests (+ thin helper if needed) | PR1 or PR2 | Closes surgical apply acceptance |
| 3 | Sync/verify/report checklist + any init-contract clarification | PR1/PR2 | Reconcile sync policy |
| 4 | Fixture evidence (topology-aware recommend) without Alquimia hardcoding | with Unit 2 | Generalizable grounding |

---

## Planning depth

- **Classification**: `full` (explore → proposal → design → spec → tasks).
- **Why full**: new agent capability; cross-cutting literacy/config/sync; card deferred design to exploration; sync-policy reconciliation with existing init-contract.
- **Delta coverage**: ADDED requirements in
  `specs/agent-assisted-recipe-config/spec.md` (NL entry, grounded recommend,
  idempotent apply, preserve overrides, sync+verify, closing report, docs/validation).
- **Authorization**: PENDING until maintainer green-lights apply.

## Non-goals (apply MUST NOT)

- Implement override lock provenance / force-update (#63).
- Replace interactive human `configure-recipes`.
- MCP-wrap the CLI.
- Hardcode Melón/Alquimia paths or repo names.
- Weaken read-only `ai-specs recipe init` into a mutating command.
- Invent per-project CLI shims under `ai-specs/bin/`.

## Open decisions to resolve at authorization (or first apply spike)

1. Skill-only vs authorize a thin non-interactive apply/inspect CLI helper.
2. Spec strategy: add-only assisted-configure requirements vs also amend
   `recipe-init-contract` Post-write wording for clarity.
3. MVP evidence recipes: `worktree-flow` only vs + `trello-mcp-workflow`.
4. Whether recommendation needs machine-readable stdout for tests.

---

## Implementation (red-green-refactor) — after authorization

Phases are ordered for TDD where code exists; skill/doc tasks note content
verification. Do not start until authorized.

### Phase 0 — Confirm delivery slice

- [ ] 0.1 With authorizer: lock skill-only vs helper/CLI; lock MVP recipe
      evidence set; note decision in Engram / PR body.
- [ ] 0.2 Re-verify worktree: `git rev-parse --show-toplevel`, branch
      `change/agent-assisted-recipe-config`, no writes on protected `development`.

### Phase 1 — RED tests for apply/preserve/idempotence (if helper or write path touched)

**Reqs:** Idempotent canonical config apply; Preserve overrides (non-clobber).

- [ ] 1.1 RED: surgical apply writes recommended keys only; unmentioned keys
      survive; comments preserved (extend existing `recipe-config-write` tests
      or adjacent new tests).
- [ ] 1.2 RED: re-apply identical values is effectively no-op / no spurious churn.
- [ ] 1.3 RED: override file under `overrides/` unchanged across assisted
      apply+sync fixture (report-only if drift detected).

### Phase 2 — GREEN apply path / playbook machinery

**Reqs:** NL entry; Grounded recommendation; Sync and verify; Closing report.

- [ ] 2.1 Implement authorized slice: skill playbook steps matching
      inspect → recommend → apply → sync/verify → report.
- [ ] 2.2 Wire cross-links in `harness-lifecycle` (configure as assisted path,
      not only interactive wizard).
- [ ] 2.3 If helper/CLI authorized: implement minimal inspect/apply surface
      reusing `update_recipe_config`; no full TOML rewrite.
- [ ] 2.4 Encode sync+verify as mandatory post-apply steps in the playbook
      (and helper exit contract if any).
- [ ] 2.5 Define closing report fields in skill (assumptions, drift, version
      gaps, sync/doctor outcome).

### Phase 3 — Grounding evidence (generalizable)

**Reqs:** Grounded recommendation (topology scenario).

- [ ] 3.1 Add or extend fixture for topology signals (submodule-style) without
      consumer-specific paths; recommendation/playbook cites resolved topology
      when configuring topology-aware recipes.
- [ ] 3.2 Ensure Melón/Alquimia remains documentation/example only.

### Phase 4 — Docs + init-contract clarity

**Reqs:** Documentation; Read-only init remains non-mutating.

- [ ] 4.1 Document assisted flow in literacy skill(s) and light project docs
      pointer.
- [ ] 4.2 If needed for clarity: additive spec note distinguishing propose-only
      init vs assisted apply (avoid silently breaking init-contract intent).
- [ ] 4.3 Promote delta into canonical `openspec/specs/` during apply as per
      project sync norms.

### Phase 5 — Validation

**Reqs:** Validation coverage.

- [ ] 5.1 Focused tests green.
- [ ] 5.2 `./tests/run.sh` and `./tests/validate.sh` green before commit/PR.
- [ ] 5.3 Record verify evidence in change folder when entering verify phase.

---

## Acceptance traceability

| Card acceptance | Spec requirement | Tasks |
|---|---|---|
| NL request | Natural-language entry | 2.1, 4.1 |
| Grounded recommendation | Grounded recommendation before apply | 2.1, 3.1 |
| Idempotent canonical update | Idempotent canonical config apply | 1.1–1.2, 2.3 |
| Preserve config/overrides | Unmentioned keys + Preserve overrides | 1.1, 1.3 |
| Run/verify sync | Sync and verify after apply | 2.4, 5.* |
| Report assumptions/drift/gaps | Closing report | 2.5 |
| Documented + validation | Documentation and validation coverage | 4.*, 5.* |
