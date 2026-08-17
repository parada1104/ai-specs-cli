# Tasks: plan-build-phase-compatibility-direct

Depth: standard

## Tracker

- **card_id**: `6a83510b0a6de6bf2ab607ed`
- **card**: #82 — Direct plan-build phase compatibility and plan presentation
- **url**: https://trello.com/c/5ClJQNEW/82-direct-plan-build-phase-compatibility-and-plan-presentation

## Scope

- [x] Document Full logical phase mapping, optional host-advertised executors,
      inline fallback, and stop/preserve semantics for malformed results.
- [x] Document single-session preflight composition and artifact-derived plan
      presentation with explicit accept/adjust/stop.
- [x] Preserve Standard/Light behavior, artifact readiness, worktree,
      archive, verify, topology, and PR gates.
- [x] Add deterministic contract tests and record RED/GREEN/final evidence.
- [x] Run focused tests and `./tests/validate.sh` without commit, push, or PR.

## Review Workload Forecast

- Estimated authored lines: ~500–700, below the cached 1200-line budget.
- Delivery: one direct worktree; no new runtime framework or dependency.
