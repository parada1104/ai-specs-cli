# Tasks: plan-build-flow-v2

Source spec: `openspec/changes/plan-build-flow-v2/specs/plan-build-flow/spec.md`
Source design: `openspec/changes/plan-build-flow-v2/design.md`

## Phase 1 — Recipe surface (1.0.0, skill-only)

- [x] **T1.1** — Remove `commands/plan.md` and `commands/build.md` from recipe manifest.
- [x] **T1.2** — Rewrite bundled skill for ambient `auto_invoke` (no slash verbs).
- [x] **T1.3** — Update README and `[provides.brief]` workflow rules for skill-only flow.
- [x] **T1.4** — Promote v2 requirements into `openspec/specs/plan-build-flow/spec.md`.

## Phase 2 — Tests and docs

- [x] **T2.1** — Rewrite `tests/test_plan_build_flow_recipe.py` for zero commands + ambient brief.
- [x] **T2.2** — Update `docs/recipes-catalog.md` plan-build-flow entry for v2.

## Phase 3 — Verify and deliver

- [x] **T3.1** — Run `./tests/run.sh`.
- [x] **T3.2** — Run `./tests/validate.sh`.
- [ ] **T3.3** — Open PR to `development`.
