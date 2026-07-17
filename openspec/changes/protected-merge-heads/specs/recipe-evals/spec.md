# Spec Delta: protected-merge-heads (recipe-evals)

## MODIFIED Requirements

### Requirement: First client scope

v2 SHALL treat `plan-build-flow` as the first live client and SHALL add
`vcs-pr-flow` sibling recipes (`git-pr-flow`, `gitlab-mr-flow`, `bitbucket-pr-flow`)
as the second client family for protected/feature head merge cleanup behavior.

LLM-as-judge (AC6) and catalog-packaged eval recipe remain out of scope.

#### Scenario: VCS sibling scenarios are discoverable
- **GIVEN** scenario trees under `tests/evals/scenarios/{git-pr-flow,gitlab-mr-flow,bitbucket-pr-flow}/`
- **WHEN** dry harness smoke (or equivalent unit checks) load a representative
  scenario from each tree
- **THEN** each loads with the matching `recipe_id`
- **AND** prompts pass the natural-prompt guard

#### Scenario: README documents VCS eval client
- **GIVEN** `tests/evals/README.md`
- **WHEN** the second-client documentation is read
- **THEN** it SHALL list the vcs-pr-flow sibling scenario set and how to select
  them via `EVALS_SCENARIOS` / `EVALS_LIVE`

### Requirement: Live runners are capability-scoped

Live entrypoints SHALL NOT mix capability clients in one process. At minimum:

- `tests/evals/run-live.sh` runs only `eval_plan_build_flow_live`
- `tests/evals/run-live-vcs.sh` runs only `eval_vcs_pr_flow_live`

#### Scenario: VCS live script is VCS-only
- **GIVEN** `tests/evals/run-live-vcs.sh`
- **WHEN** the script body is read
- **THEN** it SHALL invoke `eval_vcs_pr_flow_live`
- **AND** SHALL NOT invoke `eval_plan_build_flow_live`

### Requirement: Non-Claude live defaults use API for Cursor

For `opencode`, `pi`, and `omp`, the harness default model SHALL use the
OpenCode provider id `cursorapi` (display name "API for Cursor"), e.g.
`cursorapi/composer-2.5`. Those runtimes SHALL NOT use `anthropic/*` models or
an Anthropic API-key path. `claude` SHALL use the Claude Code subscription via
the `claude` CLI (default model id `opus`).

`EVALS_MODEL` / `EVALS_MODEL_<RUNTIME>` may override, but for OpenCode-family
overrides MUST start with `cursorapi/` or the harness SHALL error.

#### Scenario: Default models prefer cursorapi for OpenCode-family
- **GIVEN** `DEFAULT_MODELS` in the eval harness
- **WHEN** defaults for `opencode`, `pi`, and `omp` are read
- **THEN** each value SHALL start with `cursorapi/`

#### Scenario: anthropic override rejected for opencode
- **GIVEN** `EVALS_MODEL=anthropic/claude-sonnet-4-6`
- **WHEN** `default_model("opencode")` runs
- **THEN** it SHALL raise an error mentioning `cursorapi/`
