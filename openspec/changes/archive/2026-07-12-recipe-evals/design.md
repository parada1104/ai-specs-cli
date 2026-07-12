# Design: recipe-evals

## Technical Approach

Add `tests/evals/` as a third test tier. Reuse existing materialize helpers
pattern from recipe unit tests. Subprocess `claude -p` with pinned flags
(`--permission-mode acceptEdits`, `--max-turns`, JSON output for cost metadata).

## File Changes

| File | Action |
|------|--------|
| `tests/evals/lib/harness.py` | Scenario loader + claude runner |
| `tests/evals/lib/project_fixture.py` | Minimal ai-specs.toml writer |
| `tests/evals/eval_harness_smoke.py` | Dry smoke (always on) |
| `tests/evals/eval_plan_build_flow_live.py` | AC3 live eval (gated) |
| `tests/evals/scenarios/plan-build-flow/ac3_*` | First scenario |
| `openspec/specs/recipe-evals/spec.md` | New internal capability spec |
| `CLAUDE.md` | Document slow tier command |

## Testing Strategy

- Dry: `tests/evals/run.sh` in default env (no API key)
- Full unit: `./tests/run.sh` ensures no accidental eval discovery
- Live (manual): `EVALS_LIVE=1 EVALS_TRIALS=3 tests/evals/run.sh`
