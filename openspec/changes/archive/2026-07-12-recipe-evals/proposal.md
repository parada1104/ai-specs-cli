# Proposal: recipe-evals

## Intent

Close the behavioral verification gap for catalog recipes: materialization tests
prove filesystem wiring, but not that an agent following the skill/commands
actually stops before apply, closes changes, or degrades gracefully.

## Scope

### In Scope
- Repo-internal `tests/evals/` harness (Approach 1 from exploration #1147)
- `claude -p` headless runner with filesystem/git assertions
- Opt-in live gate (`EVALS_LIVE=1`); dry smoke always passes offline
- First scenario: plan-build-flow AC3
- Spec + docs for slow tier

### Out of Scope
- Per-PR CI gate (no workflows yet; nightly/pre-release cadence)
- LLM-as-judge transcript layer (AC6 soft checks) — follow-up
- Multi-runtime matrix (OpenCode/Pi spike card)
- Shipping evals as catalog recipe (3b)

## Success Criteria

- [x] `tests/evals/run.sh` passes offline
- [x] `./tests/run.sh` unchanged and still excludes evals
- [x] AC3 scenario fixture + live runner stub
- [x] `./tests/run.sh` and `./tests/validate.sh` pass
