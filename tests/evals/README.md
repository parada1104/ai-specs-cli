# Recipe behavior evals (slow tier)

Runtime behavior verification for catalog recipes using headless agent
invocation. **Not** part of `./tests/run.sh` — uses `eval_*.py` naming so
`unittest discover -p 'test_*.py'` never loads this directory.

## When to run

- Nightly or pre-release (not per-PR): needs API key, billed LLM calls, inherent flakiness.
- Local opt-in: `EVALS_LIVE=1 ./tests/evals/run.sh`

## Requirements

- `claude` CLI on `PATH`
- `ANTHROPIC_API_KEY` (or `CLAUDE_API_KEY` where supported)
- Optional: `EVALS_MAX_TURNS`, `EVALS_TRIALS` (default trials=1; use 3 for N-of-M)

## Layout

```
tests/evals/
  lib/           # harness + project fixtures
  scenarios/     # per-recipe scenario folders
  eval_*.py      # unittest modules (dry + live)
  run.sh         # discover eval_*.py only
```

## First client

`plan-build-flow` AC3–AC7 (behavioral gap vs materialization tests AC1/2/8/9/10).
