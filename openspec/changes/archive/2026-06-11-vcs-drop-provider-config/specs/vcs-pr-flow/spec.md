## MODIFIED Requirements

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

### Requirement: Provider Binding Semantics

When multiple recipes provide `vcs-pr-flow`, the system MUST require an explicit
`[[bindings]]` selection; without it, sync MUST warn and leave `vcs-pr-flow` unbound.
The bound **recipe id** is the provider identity; there is no separate `provider` config.

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
`vcs-pr-flow` recipe id, not from a `provider` config value. It MUST append
`base branch: \`<base_branch>\`` when `base_branch` is configured or defaulted.

#### Scenario: GitHub binding renders gh hint
- GIVEN `bindings.vcs-pr-flow = "git-pr-flow"` and `base_branch = "development"`
- WHEN `agents-render.py` renders the brief
- THEN the Runtime Flow section includes `VCS/PR provider: GitHub` and `gh` CLI
- AND includes `base branch: \`development\``

#### Scenario: Non-GitHub binding omits gh-only hint
- GIVEN `bindings.vcs-pr-flow = "gitlab-mr-flow"`
- WHEN `agents-render.py` renders the brief
- THEN the output MUST NOT include a GitHub-specific `gh` CLI hint from config.provider
- AND MUST identify GitLab/glab from the recipe id mapping

#### Scenario: Stale provider config ignored
- GIVEN a manifest still sets `[recipes.gitlab-mr-flow.config] provider = "github"`
- WHEN sync validates and renders
- THEN sync warns that `provider` is an unknown config key
- AND the rendered brief still identifies GitLab from the binding recipe id
