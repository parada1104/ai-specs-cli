# Verification Report: topology-aware `gate_scope`

## Result

PASS for the focused gate-scope contract. The full repository validation command was attempted but exceeded the harness timeout; the apply run recorded 1306 tests with one pre-existing Trello tracking `warn`/`always` consistency failure, then the dogfood manifest was aligned to `warn` and focused suites were rerun successfully.

## Commands and outcomes

- `bash -n catalog/recipes/worktree-flow/hooks/worktree-gate.sh` — PASS.
- `python3 tests/test_worktree_flow_recipe.py` — PASS, 16 tests.
- `python3 tests/test_worktree_gate_hook.py` — PASS, 50 tests.
- `python3 tests/test_trello_mcp_workflow_recipe.py` — PASS, 9 tests.
- `bash tests/validate.sh` — attempted; py_compile and bash syntax passed, but the harness timed out during the long full unittest run. The only failure observed before timeout was the pre-existing `openspec/config.yaml` tracking `gate_mode: warn` versus dogfood `ai-specs.toml` `gate_mode = "always"`; the worktree manifest was aligned to `warn` for consistency.

## Behavioral coverage

- `gate_scope` missing/empty defaults to `auto`; invalid values are rejected with the exact enum.
- Valid stamped values and `WORKTREE_GATE_SCOPE` precedence are covered; invalid/missing stamps fail safe to `auto`.
- `gate_mode=off` exits before scope/topology evaluation.
- Proven initialized superrepo and subrepo primary checkouts are classified separately.
- Central canonical `<superrepo>/openspec/changes/**` is allowed under all valid scopes when topology is proven.
- Superrepo non-central production writes remain blocked.
- Subrepo production writes remain blocked on protected branches.
- Linked subrepo worktrees remain allowed.
- Uninitialized modules, ambiguous duplicate registrations, nested registrations, symlink escapes, and unresolved relationships do not grant the central exception.
- Structured path and shell write candidates share the scope decision path and exact protected-branch matching.
- Existing stale generated hooks lacking the scope stamp are preserved with explicit remove-and-sync guidance; current hooks are not warned.

## Review

Fresh reliability review found no runtime blocker. It identified a prior test-coverage gap; explicit hermetic assertions were added for the scope matrix and topology failure cases, bringing the gate suite to 50 passing tests.

## Unavailable signals

Coverage, linter, type-checker, and formatter are not configured by `openspec/config.yaml`; none were run.

## Residual limitation

The full suite was not observed to completion in this harness because the command exceeded the timeout. Focused affected suites and syntax/compile portions passed; the unrelated Trello tracking consistency mismatch was corrected in the dogfood manifest for the rerun.
