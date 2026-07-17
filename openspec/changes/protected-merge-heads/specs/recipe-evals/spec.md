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
