# Recipe behavior evals (slow tier)

Runtime behavior verification for catalog recipes using headless agent
invocation. **Not** part of `./tests/run.sh` — uses `eval_*.py` naming so
`unittest discover -p 'test_*.py'` never loads this directory.

## When to run

- Nightly or pre-release (not per-PR): needs billed LLM calls, inherent flakiness.
- Local opt-in: `EVALS_LIVE=1 ./tests/evals/run.sh`

## Requirements

- A supported runtime on `PATH`: `claude`, `opencode`, `pi`, or `omp`
- Select with `EVALS_RUNTIME` (otherwise first available is auto-detected)
- Optional model override: `EVALS_MODEL`
- Defaults:
  - `claude` → `opus`
  - `opencode` / `pi` / `omp` → `opencode-go/glm-5.2`
    (alternate: `opencode-go/deepseek-v4-flash`)
- Optional: `EVALS_MAX_TURNS`, `EVALS_TRIALS` (default trials=1; use 3 for N-of-M)

## Layout

```
tests/evals/
  lib/           # harness + project fixtures
  scenarios/     # per-recipe scenario folders
  eval_*.py      # unittest modules (dry + live)
  run.sh         # discover eval_*.py only
```

## Scenario contract

- Prompts are **natural user requests** (no `/plan`, `/build`, or "haz un plan")
- `scenario.toml` may set `mode = "plan" | "build"`
- Plan-mode runs must not modify production paths listed in
  `forbidden_path_globs`
- Fixtures seed a tiny app and copy the recipe skill into the runtime discovery
  path (`.claude/skills`, `.opencode/skills`, `.pi/skills`, …)

## First client

`plan-build-flow` AC3 (ambient plan stops before apply). AC4/AC5 stubs exist
for build/archive follow-ups.
