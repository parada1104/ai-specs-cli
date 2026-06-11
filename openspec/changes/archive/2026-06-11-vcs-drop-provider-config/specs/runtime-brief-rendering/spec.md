## MODIFIED Requirements

### Requirement: VCS provider bullet uses binding recipe id

The Runtime Flow VCS supplemental bullet MUST be emitted from the bound `vcs-pr-flow`
recipe id using a fixed id→label map. It MUST NOT read `provider` from
`recipes[<bound-id>].config`.

#### Scenario: Recipe id drives provider label
- GIVEN `resolved-config` has `bindings.vcs-pr-flow = "git-pr-flow"`
- AND `recipes.git-pr-flow.config` contains only `base_branch`
- WHEN `_section_runtime_flow` renders
- THEN the VCS bullet names GitHub and the `gh` CLI
- AND does not require a `provider` config key

#### Scenario: Base branch still configurable
- GIVEN `bindings.vcs-pr-flow = "gitlab-mr-flow"`
- AND `recipes.gitlab-mr-flow.config.base_branch = "main"`
- WHEN `_section_runtime_flow` renders
- THEN the VCS bullet includes `base branch: \`main\``
