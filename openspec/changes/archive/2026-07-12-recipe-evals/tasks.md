# Tasks: recipe-evals

## Phase 1 — Harness infrastructure

- [x] **T1.1** — `tests/evals/` layout + `run.sh` (`eval_*.py` discovery)
- [x] **T1.2** — `lib/harness.py` scenario loader + claude subprocess wrapper
- [x] **T1.3** — Dry smoke tests (`eval_harness_smoke.py`)

## Phase 2 — plan-build-flow AC3

- [x] **T2.1** — AC3 scenario fixture (`ac3_plan_stops_before_apply`)
- [x] **T2.2** — Live eval module with N-of-M trials + skip gate

## Phase 3 — Spec, docs, verify

- [x] **T3.1** — `openspec/specs/recipe-evals/spec.md`
- [x] **T3.2** — Update `CLAUDE.md` slow-tier docs
- [x] **T3.3** — Map plan-build-flow AC3–AC7 to eval scenarios in spec note
- [x] **T3.4** — `./tests/run.sh` + `./tests/validate.sh`
- [ ] **T3.5** — Open PR to `development`
