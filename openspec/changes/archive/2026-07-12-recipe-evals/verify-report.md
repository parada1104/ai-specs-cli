# Verify Report — recipe-evals

**Change**: recipe-evals
**Verdict**: PASS (dry tier)
**Date**: 2026-07-12

## Requirements

| Requirement | Verdict | Evidence |
|-------------|---------|----------|
| Separate discovery tier | PASS | `eval_*.py` naming; run.sh glob |
| Opt-in live execution | PASS | live module skipped without EVALS_LIVE |
| Scenario fixture format | PASS | `eval_harness_smoke.test_scenario_fixture_loads` |
| AC3 scenario | PASS | fixture + gated live runner |
| Unit suite unaffected | PASS | `./tests/run.sh` OK |

## Test evidence

- `tests/evals/run.sh` — 5 tests OK (1 skipped live)
- `./tests/run.sh` — 764 OK
- `./tests/validate.sh` — 764 OK
- `tests/evals/run.sh` — 5 OK (1 skipped live)
