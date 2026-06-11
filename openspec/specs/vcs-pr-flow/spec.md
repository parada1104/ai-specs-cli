# vcs-pr-flow Specification: Multi-Provider VCS Flow

## Purpose

Provide provider-backed `vcs-pr-flow` recipes that mirror the same semantics across GitHub,
GitLab, and Bitbucket: explicit branch pushes, review-gated merging, and worktree cleanup.
The bound recipe id is the provider identity; only `base_branch` is configurable per project.

## Requirements

### Requirement: VCS Sibling Recipe Manifests

Each VCS sibling recipe (`git-pr-flow`, `gitlab-mr-flow`, `bitbucket-pr-flow`) MUST declare
`vcs-pr-flow`, an `on-sync` `validate-config` hook, a bundled host-specific merge workflow
skill, a host-specific create command, README doc provision, and **only** `base_branch` as
config (no `provider` key).

#### Scenario: GitHub manifest validates
- GIVEN the `git-pr-flow` catalog recipe is loaded
- WHEN recipe schema validation runs
- THEN the recipe is valid and declares `vcs-pr-flow`
- AND `base_branch` defaults to `main`
- AND no `provider` field exists in `[config]`

#### Scenario: GitLab manifest validates
- GIVEN the `gitlab-mr-flow` catalog recipe is loaded
- WHEN recipe schema validation runs
- THEN the recipe is valid and declares `vcs-pr-flow`
- AND `base_branch` defaults to `development`
- AND no `provider` field exists in `[config]`

#### Scenario: Bitbucket manifest validates
- GIVEN the `bitbucket-pr-flow` catalog recipe is loaded
- WHEN recipe schema validation runs
- THEN the recipe is valid and declares `vcs-pr-flow`
- AND `base_branch` defaults to `development`
- AND no `provider` field exists in `[config]`

### Requirement: Materialized Assets

Sync MUST materialize provider assets without changing sibling provider recipe assets when
only one provider recipe is enabled.

#### Scenario: GitLab sync provisions assets
- GIVEN `gitlab-mr-flow` is enabled
- WHEN `ai-specs sync` runs
- THEN the GitLab skill, command, and README exist in generated locations

#### Scenario: Bitbucket sync provisions assets
- GIVEN `bitbucket-pr-flow` is enabled
- WHEN `ai-specs sync` runs
- THEN the Bitbucket skill, command, and README exist in generated locations

### Requirement: Provider Binding Semantics

When multiple recipes provide `vcs-pr-flow`, the system MUST require an explicit
`[[bindings]]` selection; without it, sync MUST warn and leave `vcs-pr-flow` unbound.
The bound **recipe id** is the provider identity.

#### Scenario: Ambiguous providers stay unbound
- GIVEN multiple VCS provider recipes are enabled without `[[bindings]]`
- WHEN sync resolves capabilities
- THEN a warning names the ambiguity
- AND no implicit `vcs-pr-flow` binding is selected

#### Scenario: Explicit binding selects host
- GIVEN multiple VCS provider recipes are enabled with a binding to `bitbucket-pr-flow`
- WHEN sync resolves capabilities
- THEN `vcs-pr-flow` is bound to Bitbucket assets and brief rules

### Requirement: Runtime Brief VCS Bullet

`agents-render.py` MUST derive the Runtime Flow VCS provider bullet from the bound
`vcs-pr-flow` recipe id, not from a `provider` config value.

#### Scenario: GitHub binding renders gh hint
- GIVEN `bindings.vcs-pr-flow = "git-pr-flow"` and `base_branch = "development"`
- WHEN `agents-render.py` renders the brief
- THEN the Runtime Flow section includes `VCS/PR provider: GitHub` and `gh` CLI

#### Scenario: Stale provider config ignored
- GIVEN a manifest still sets `[recipes.gitlab-mr-flow.config] provider = "github"`
- WHEN sync validates and renders with binding to `gitlab-mr-flow`
- THEN sync warns that `provider` is an unknown config key
- AND the rendered brief still identifies GitLab from the binding recipe id

### Requirement: Runtime Checks and Docs

Provider skills and commands MUST check CLI install/auth before PR/MR creation, stop with
actionable blockers on failure, and README MUST document enablement, config (`base_branch`
only), explicit bindings, runtime prerequisites, explicit push behavior, and no auto-merge
policy.
