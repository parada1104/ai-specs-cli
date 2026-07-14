# Delta for vcs-pr-flow

## MODIFIED Requirements

### Requirement: VCS Sibling Recipe Manifests

Each VCS sibling recipe (`git-pr-flow`, `gitlab-mr-flow`, `bitbucket-pr-flow`) MUST declare
`vcs-pr-flow`, an `on-sync` `validate-config` hook, a bundled host-specific merge workflow
skill, a host-specific create command, README doc provision, and config fields `base_branch`,
`expected_owner`, and `auto_switch_account` (no `provider` key).

#### Scenario: GitHub manifest validates
- GIVEN the `git-pr-flow` catalog recipe is loaded
- WHEN recipe schema validation runs
- THEN the recipe is valid and declares `vcs-pr-flow`
- AND `base_branch` defaults to `main`
- AND `expected_owner` defaults to `""`
- AND `auto_switch_account` defaults to `false`
- AND no `provider` field exists in `[config]`

#### Scenario: GitLab manifest validates
- GIVEN the `gitlab-mr-flow` catalog recipe is loaded
- WHEN recipe schema validation runs
- THEN the recipe is valid and declares `vcs-pr-flow`
- AND `base_branch` defaults to `development`
- AND `expected_owner` defaults to `""`
- AND `auto_switch_account` defaults to `false`
- AND no `provider` field exists in `[config]`

#### Scenario: Bitbucket manifest validates
- GIVEN the `bitbucket-pr-flow` catalog recipe is loaded
- WHEN recipe schema validation runs
- THEN the recipe is valid and declares `vcs-pr-flow`
- AND `base_branch` defaults to `development`
- AND `expected_owner` defaults to `""`
- AND `auto_switch_account` defaults to `false`
- AND no `provider` field exists in `[config]`

(Previously: only `base_branch` was declared as config.)

### Requirement: Runtime Checks and Docs

Provider skills and commands MUST check CLI install/auth before PR/MR creation, stop with
actionable blockers on failure, and README MUST document enablement, config (`base_branch`,
`expected_owner`, `auto_switch_account`), explicit bindings, runtime prerequisites, explicit
push behavior, and no auto-merge policy.

(Previously: README documented `base_branch` only for config.)

### Requirement: Git PR Flow Docs Omit Provider

The `git-pr-flow` README and `docs/recipes-catalog.md` section for `git-pr-flow` MUST document
`base_branch`, `expected_owner`, and `auto_switch_account` for config.
Neither document MAY include a `provider` config row.

#### Scenario: README contract
- GIVEN `catalog/recipes/git-pr-flow/README.md`
- WHEN the docs contract is checked
- THEN the config table includes `base_branch`, `expected_owner`, and `auto_switch_account`
- AND it does not include `provider`

#### Scenario: Catalog contract
- GIVEN `docs/recipes-catalog.md`
- WHEN the `## git-pr-flow` section is checked
- THEN the config table includes `base_branch`, `expected_owner`, and `auto_switch_account`
- AND it does not include `provider`

(Previously: config tables documented `base_branch` only.)

## ADDED Requirements

### Requirement: Auth Preflight Config Fields

Each VCS sibling recipe (`git-pr-flow`, `gitlab-mr-flow`, `bitbucket-pr-flow`) MUST declare
two optional config fields in its `recipe.toml`:

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `expected_owner` | string | no | `""` | Expected VCS account username for this repo |
| `auto_switch_account` | boolean | no | `false` | Auto-switch CLI account if mismatch detected (gh only) |

#### Scenario: Config schema entry for expected_owner
- GIVEN any VCS sibling recipe's `recipe.toml`
- WHEN the `[config.expected_owner]` section is parsed
- THEN `required` is `false`
- AND `type` is `"string"`
- AND `default` is `""`

#### Scenario: Config schema entry for auto_switch_account
- GIVEN any VCS sibling recipe's `recipe.toml`
- WHEN the `[config.auto_switch_account]` section is parsed
- THEN `required` is `false`
- AND `type` is `"boolean"`
- AND `default` is `false`

### Requirement: Auth Preflight Gating

The auth preflight MUST only activate when `expected_owner` is set to a non-empty string.
When `expected_owner` is unset or empty, the recipe behaves identically to before this change —
no additional CLI calls, no account checks, no blocking.

#### Scenario: No expected_owner set — preflight skipped
- GIVEN a recipe has `expected_owner = ""` (default)
- WHEN the preflight step is reached
- THEN no auth status command is run for account-match
- AND no account comparison occurs
- AND the workflow proceeds as before

#### Scenario: expected_owner set — preflight runs
- GIVEN a recipe has `expected_owner = "myuser"`
- WHEN the preflight step is reached
- THEN the provider's auth status command is executed
- AND the active account is compared against `"myuser"`

### Requirement: Account-Match Preflight on VCS Sibling Recipes

When `expected_owner` is set, each bound VCS provider recipe MUST run an account-match
preflight after install/auth checks and before push/create. GitHub MAY auto-switch when
`auto_switch_account = true` and gh >= 2.50.0; GitLab and Bitbucket MUST block on mismatch.

#### Scenario: git-merge-workflow includes account preflight
- GIVEN the `git-merge-workflow` skill
- WHEN the skill is read
- THEN its Runtime Preflight section includes the account-match check with gh auth switch support

#### Scenario: gitlab-merge-workflow includes account preflight
- GIVEN the `gitlab-merge-workflow` skill
- WHEN the skill is read
- THEN its Runtime Preflight section includes the account-match check with block-on-mismatch

#### Scenario: bitbucket-merge-workflow uses bb auth show
- GIVEN the `bitbucket-merge-workflow` skill
- WHEN the skill is read
- THEN its Runtime Preflight authentication check uses `bb auth show` (not `bb auth status`)
- AND the account-match preflight is included with block-on-mismatch

#### Scenario: pr-create.md includes account match step
- GIVEN the `git-pr-flow` command `pr-create.md`
- WHEN the command is read
- THEN it contains a "Runtime Preflight: Account Match" section
- AND the section includes `gh auth status`, version check, and conditional switch/block logic

#### Scenario: mr-create.md includes account match step
- GIVEN the `gitlab-mr-flow` command `mr-create.md`
- WHEN the command is read
- THEN it contains a "Runtime Preflight: Account Match" section
- AND the section includes `glab auth status` and block-on-mismatch logic

#### Scenario: bb-pr-create.md includes account match step
- GIVEN the `bitbucket-pr-flow` command `bb-pr-create.md`
- WHEN the command is read
- THEN it contains a "Runtime Preflight: Account Match" section
- AND the section uses `bb auth show` (not `bb auth status`)
- AND the section includes block-on-mismatch logic
