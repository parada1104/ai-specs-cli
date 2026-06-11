# Delta for vcs-pr-flow

## ADDED Requirements

### Requirement: Bound VCS Workflow Rules Stay Isolated

The system MUST emit `workflow_rules` brief fragments only from the recipe bound to `vcs-pr-flow`.
Fragments from other enabled VCS sibling recipes MUST NOT appear when a binding exists.

#### Scenario: One bound recipe among three enabled
- GIVEN `git-pr-flow`, `gitlab-mr-flow`, and `bitbucket-pr-flow` are enabled
- AND `vcs-pr-flow` is bound to `gitlab-mr-flow`
- WHEN the brief is rendered
- THEN only GitLab workflow rules appear
- AND GitHub and Bitbucket workflow rules do not appear

#### Scenario: Single enabled bound recipe
- GIVEN only `git-pr-flow` is enabled and bound
- WHEN the brief is rendered
- THEN the GitHub workflow rules appear
- AND no other VCS workflow rules are added

#### Scenario: No VCS binding exists
- GIVEN VCS sibling recipes are enabled
- AND `vcs-pr-flow` is unbound
- WHEN the brief is rendered
- THEN no VCS workflow rule fragments are emitted

### Requirement: Git PR Flow Docs Omit Provider

The `git-pr-flow` README and `docs/recipes-catalog.md` section for `git-pr-flow` MUST document `base_branch` only for config.
Neither document MAY include a `provider` config row.

#### Scenario: README contract
- GIVEN `catalog/recipes/git-pr-flow/README.md`
- WHEN the docs contract is checked
- THEN the config table includes `base_branch`
- AND it does not include `provider`

#### Scenario: Catalog contract
- GIVEN `docs/recipes-catalog.md`
- WHEN the `## git-pr-flow` section is checked
- THEN the config table includes `base_branch`
- AND it does not include `provider`

### Requirement: Test and Validation Commands Pass

The implementation MUST pass `./tests/run.sh` and `./tests/validate.sh` with the change applied.

#### Scenario: Focused run passes
- GIVEN the change is applied
- WHEN `./tests/run.sh` runs
- THEN it exits successfully

#### Scenario: Full validation passes
- GIVEN the change is applied
- WHEN `./tests/validate.sh` runs
- THEN it exits successfully

## MODIFIED Requirements

### Requirement: Runtime Brief VCS Bullet

The renderer MUST derive the Runtime Flow VCS provider bullet from the bound `vcs-pr-flow` recipe id, not from a `provider` config value.
If the bound recipe id is unknown to the VCS label table, it MUST emit a `⚠ ai-specs:` warning to stderr and render `VCS PR (custom)`.
It MUST append `base branch: \`<base_branch>\`` when `base_branch` is configured or defaulted.
(Previously: The bullet only mapped known recipe ids and appended base branch.)

#### Scenario: GitHub binding renders gh hint
- GIVEN `bindings.vcs-pr-flow = "git-pr-flow"` and `base_branch = "development"`
- WHEN the brief is rendered
- THEN the Runtime Flow section includes `VCS/PR provider: GitHub` and `gh` CLI
- AND includes `base branch: \`development\``

#### Scenario: Unknown recipe id warns and falls back
- GIVEN `bindings.vcs-pr-flow = "custom-pr-flow"`
- WHEN the brief is rendered
- THEN stderr includes `⚠ ai-specs:`
- AND the Runtime Flow section uses `VCS PR (custom)`

#### Scenario: Multiple unknown ids each warn
- GIVEN two render passes bind different unknown `vcs-pr-flow` ids
- WHEN each brief is rendered
- THEN each pass emits one `⚠ ai-specs:` warning
- AND each pass uses `VCS PR (custom)`

#### Scenario: Stale provider config ignored
- GIVEN a manifest still sets `[recipes.gitlab-mr-flow.config] provider = "github"`
- WHEN sync validates and renders
- THEN sync warns that `provider` is an unknown config key
- AND the rendered brief still identifies GitLab from the binding recipe id
