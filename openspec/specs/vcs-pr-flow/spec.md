# vcs-pr-flow Specification: Multi-Provider VCS Flow

## Purpose

Provide provider-backed `vcs-pr-flow` recipes that mirror the same semantics across GitHub,
GitLab, and Bitbucket: explicit branch pushes, review-gated merging, and worktree cleanup.
The bound recipe id is the provider identity; `base_branch`, `expected_owner`, and
`auto_switch_account` are configurable per project.

## Requirements

### Requirement: VCS Sibling Recipe Manifests

Each VCS sibling recipe (`git-pr-flow`, `gitlab-mr-flow`, `bitbucket-pr-flow`) MUST declare
`vcs-pr-flow`, an `on-sync` `validate-config` hook, a bundled host-specific merge workflow
skill, a host-specific create command, README doc provision, `base_branch`, `expected_owner`,
and `auto_switch_account` as config (no `provider` key).

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `expected_owner` | string | no | `""` | Expected VCS account username for this repo |
| `auto_switch_account` | boolean | no | `false` | Auto-switch CLI account if mismatch detected (gh only) |

#### Scenario: GitHub manifest validates
- GIVEN the `git-pr-flow` catalog recipe is loaded
- WHEN recipe schema validation runs
- THEN the recipe is valid and declares `vcs-pr-flow`
- AND `base_branch` defaults to `main`
- AND `expected_owner` defaults to `""` (empty string)
- AND `auto_switch_account` defaults to `false`
- AND no `provider` field exists in `[config]`

#### Scenario: GitLab manifest validates
- GIVEN the `gitlab-mr-flow` catalog recipe is loaded
- WHEN recipe schema validation runs
- THEN the recipe is valid and declares `vcs-pr-flow`
- AND `base_branch` defaults to `development`
- AND `expected_owner` defaults to `""` (empty string)
- AND `auto_switch_account` defaults to `false`
- AND no `provider` field exists in `[config]`

#### Scenario: Bitbucket manifest validates
- GIVEN the `bitbucket-pr-flow` catalog recipe is loaded
- WHEN recipe schema validation runs
- THEN the recipe is valid and declares `vcs-pr-flow`
- AND `base_branch` defaults to `development`
- AND `expected_owner` defaults to `""` (empty string)
- AND `auto_switch_account` defaults to `false`
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

### Requirement: Runtime Checks and Docs

Provider skills and commands MUST check CLI install/auth before PR/MR creation, run
config-gated account-match preflight when `expected_owner` is set, stop with actionable
blockers on failure, and README MUST document enablement, config (`base_branch`,
`expected_owner`, `auto_switch_account`), explicit bindings, runtime prerequisites,
explicit push behavior, and no auto-merge policy.

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

The `git-pr-flow` README and `docs/recipes-catalog.md` section for `git-pr-flow` MUST
document `base_branch`, `expected_owner`, and `auto_switch_account` for config.
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

### Requirement: Pre-merge archive artifacts

The system MUST archive and record SDD/OpenSpec artifacts before a VCS PR/MR is merged. The archive boundary MUST occur while the change is still on the review branch, not after the merge commit lands on the base branch.

#### Scenario: Archive runs before merge

- GIVEN a provider-backed PR/MR is ready to merge
- WHEN the archive step runs for the change
- THEN the change artifacts are persisted before merge completes
- AND the archive records the pre-merge state as the source of truth

#### Scenario: Post-merge archive is rejected

- GIVEN a PR/MR has already been merged into the base branch
- WHEN the archive step tries to treat the merged state as the archive boundary
- THEN the system rejects that interpretation
- AND the archive must reference the pre-merge branch state instead

#### Scenario: Provider behavior stays aligned

- GIVEN GitHub, GitLab, or Bitbucket provider flows are enabled
- WHEN the pre-merge archive rule is rendered into workflow guidance
- THEN the provider guidance matches the same archive-before-merge contract
- AND no provider introduces a different timing rule

#### Scenario: Hidden ceremony remains hidden

- GIVEN the user follows the normal plan/build flow
- WHEN the archive rule is applied
- THEN no new slash command or extra user-facing mode is introduced
- AND the archive step remains part of the existing invisible workflow

### Requirement: Post-merge branch and worktree cleanup

After a VCS PR/MR is merged, provider merge-workflow skills MUST instruct the
agent to remove the feature worktree and delete the local feature branch when
the user requests merge cleanup. Squash and rebase merges MUST use force-delete
(`git branch -D`) because feature tips are not ancestors of the base branch.

#### Scenario: Squash merge allows local branch cleanup

- GIVEN a feature branch was merged with squash
- WHEN post-merge cleanup runs
- THEN the skill uses `git branch -D` (not `-d`) for the local branch
- AND removes the worktree from outside the worktree directory

#### Scenario: Provider skills stay aligned on cleanup

- GIVEN GitHub, GitLab, or Bitbucket provider flows are enabled
- WHEN post-merge cleanup guidance is rendered
- THEN each provider skill documents worktree removal and local branch deletion
- AND no provider omits cleanup as an optional afterthought

### Requirement: Auth Preflight Gating

The auth preflight MUST only activate when `expected_owner` is set to a non-empty string.
When `expected_owner` is unset or empty, the recipe behaves identically to before this change —
no additional CLI calls, no account checks, no blocking.

#### Scenario: No expected_owner set — preflight skipped
- GIVEN a recipe has `expected_owner = ""` (default)
- WHEN the preflight step is reached
- THEN no auth status command is run
- AND no account comparison occurs
- AND the workflow proceeds as before

#### Scenario: expected_owner set — preflight runs
- GIVEN a recipe has `expected_owner = "myuser"`
- WHEN the preflight step is reached
- THEN the provider's auth status command is executed
- AND the active account is compared against `"myuser"`

### Requirement: GitHub (gh) Auth Preflight

When the bound provider is `git-pr-flow` and `expected_owner` is set, the preflight MUST:

1. Check gh version (>= 2.50.0 required for `gh auth switch`)
2. Detect active account via `gh auth status`
3. Extract repo owner from `git remote get-url origin`
4. Compare active account to `expected_owner`
5. Auto-switch with `gh auth switch --user <expected_owner>` when `auto_switch_account = true`,
   otherwise block with guidance

#### Scenario: gh account matches — proceed
- GIVEN `expected_owner = "robert"` and the active gh account is `robert`
- WHEN the preflight runs
- THEN no switch is attempted
- AND the workflow proceeds to push/create

#### Scenario: gh account mismatch with auto_switch enabled
- GIVEN `expected_owner = "robert"`, active gh account is `other`, and `auto_switch_account = true`
- AND gh version >= 2.50.0
- WHEN the preflight runs
- THEN `gh auth switch --user robert` is executed
- AND `gh auth status` is re-checked
- AND if the account is now `robert`, the workflow proceeds

#### Scenario: gh account mismatch with auto_switch disabled
- GIVEN `expected_owner = "robert"`, active gh account is `other`, and `auto_switch_account = false`
- WHEN the preflight runs
- THEN the workflow is blocked with current/expected account and switch guidance

### Requirement: GitLab (glab) Auth Preflight

When the bound provider is `gitlab-mr-flow` and `expected_owner` is set, the preflight MUST
detect active account via `glab auth status`, compare to `expected_owner`, and block with
guidance (glab has no `auth switch`).

#### Scenario: glab account matches — proceed
- GIVEN `expected_owner = "robert"` and the active glab account is `robert`
- WHEN the preflight runs
- THEN the workflow proceeds to push/create

#### Scenario: glab account mismatch — block
- GIVEN `expected_owner = "robert"` and the active glab account is `other`
- WHEN the preflight runs
- THEN the workflow is blocked with manual login and env-var guidance

### Requirement: Bitbucket (bb) Auth Preflight

When the bound provider is `bitbucket-pr-flow` and `expected_owner` is set, the preflight MUST
detect active account via `bb auth show` (NOT `bb auth status`), compare to `expected_owner`,
and block with guidance (bb has no `auth switch`).

#### Scenario: bb auth show replaces bb auth status
- GIVEN the `bitbucket-pr-flow` recipe's bb-pr-create.md command
- WHEN the authentication verification step is reviewed
- THEN the command used is `bb auth show` (not `bb auth status`)

#### Scenario: bb account mismatch — block
- GIVEN `expected_owner = "myworkspace"` and the active bb account is `other`
- WHEN the preflight runs
- THEN the workflow is blocked with manual login guidance

### Requirement: Remote URL Owner Parsing

The owner extraction from `git remote get-url origin` MUST handle SSH and HTTPS formats
for GitHub, GitLab, and Bitbucket. Malformed URLs MUST warn but NOT block.

#### Scenario: SSH remote URL parsed correctly
- GIVEN `git remote get-url origin` returns `git@github.com:acme/widget.git`
- WHEN the owner is extracted
- THEN the result is `acme`

#### Scenario: Malformed remote URL does not block
- GIVEN `git remote get-url origin` returns an unrecognized format
- WHEN the owner extraction runs
- THEN a warning is logged
- AND the preflight continues with `expected_owner` if set, or skips

### Requirement: Command File Preflight Steps

Each VCS create command MUST include a "Runtime Preflight: Account Match" step between the
existing authentication check and the push step.

#### Scenario: pr-create.md includes account match step
- GIVEN the `git-pr-flow` command `pr-create.md`
- WHEN the command is read
- THEN it contains a "Runtime Preflight: Account Match" section with gh auth status,
  remote owner extraction, version check, and conditional switch/block logic

#### Scenario: mr-create.md includes account match step
- GIVEN the `gitlab-mr-flow` command `mr-create.md`
- WHEN the command is read
- THEN it contains a "Runtime Preflight: Account Match" section with glab auth status
  and block-on-mismatch logic

#### Scenario: bb-pr-create.md includes account match step
- GIVEN the `bitbucket-pr-flow` command `bb-pr-create.md`
- WHEN the command is read
- THEN it contains a "Runtime Preflight: Account Match" section using `bb auth show`

### Requirement: Skill Runtime Preflight Updates

Each provider merge-workflow skill MUST include the account-match preflight in its
"Runtime Preflight" section, positioned after install/auth checks and before workflow steps.

#### Scenario: git-merge-workflow includes account preflight
- GIVEN the `git-merge-workflow` skill
- WHEN the skill is read
- THEN its Runtime Preflight section includes account-match check with gh auth switch support

#### Scenario: bitbucket-merge-workflow uses bb auth show
- GIVEN the `bitbucket-merge-workflow` skill
- WHEN the skill is read
- THEN its Runtime Preflight authentication check uses `bb auth show` (not `bb auth status`)

### Requirement: No Regression When Config Unset

Recipes that do not set `expected_owner` (empty string or absent) MUST behave identically
to their pre-change behavior. No additional CLI calls, no account checks, no delays.

#### Scenario: Unset expected_owner — no extra calls
- GIVEN a recipe with no `expected_owner` override in the manifest
- WHEN the create command runs
- THEN no account-check call is made for the preflight
- AND the command proceeds directly to push/create

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
