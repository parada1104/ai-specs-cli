# Spec Delta: protected-merge-heads (vcs-pr-flow)

## ADDED Requirements

### Requirement: Protected vs feature head cleanup

Each provider merge-workflow skill (`git-merge-workflow`, `gitlab-merge-workflow`,
`bitbucket-merge-workflow`) MUST classify the PR/MR head branch before merge and
post-merge cleanup.

**Protected heads** (exact name match): `main`, `master`, `development`, `staging`,
plus the project's configured `base_branch` and (when present)
`[recipes.worktree-flow.config].integration_branch`.

**Feature heads**: every other branch name, including `release/vX.Y.Z`.

For a **protected** head, the skill MUST:

- merge without provider flags that delete the source branch
  (`--delete-branch`, `--remove-source-branch`, `--close-source-branch`)
- skip worktree remove, local `git branch -D`, and remote `git push --delete`
  for that head
- still sync the integration/base branch after merge

For a **feature** head, the skill MUST delete the remote source branch
(explicitly or via the provider delete flag) and run the existing worktree/local
branch cleanup.

#### Scenario: Protected head skips source-branch delete
- **GIVEN** a merged PR whose head is `development`
- **WHEN** the merge-workflow post-merge steps run
- **THEN** the skill SHALL NOT pass `--delete-branch` / `--remove-source-branch` /
  `--close-source-branch`
- **AND** SHALL NOT run worktree remove or `git branch -D` for `development`

#### Scenario: Feature head still cleans up
- **GIVEN** a merged PR whose head is `feat/example`
- **WHEN** the merge-workflow post-merge steps run
- **THEN** the skill SHALL delete the remote feature head (or use the provider
  delete-source flag)
- **AND** SHALL run worktree/local branch cleanup as before

### Requirement: GitHub delete_branch_on_merge preflight

The `git-merge-workflow` skill MUST include a Runtime Preflight that reads the
GitHub repo setting `delete_branch_on_merge`. When the value is `true`, the skill
MUST warn that long-lived heads can be wiped if used as PR head, and MUST document
the remediation:

```bash
gh api -X PATCH repos/{owner}/{repo} -f delete_branch_on_merge=false
```

The skill MUST NOT auto-apply that PATCH without explicit user approval.
Feature-head cleanup MUST NOT depend on GitHub auto-delete.

GitLab and Bitbucket skills MUST document that MR/PR UI "delete source branch"
options must stay off for protected heads, but NEED NOT call a repo-wide API
equivalent.

#### Scenario: git-merge-workflow documents delete_branch_on_merge check
- **GIVEN** the catalog `git-merge-workflow` skill
- **WHEN** the skill is read
- **THEN** it SHALL mention `delete_branch_on_merge`
- **AND** SHALL include the `gh api` PATCH remediation command

### Requirement: Release heads preferred over long-lived heads into main

Merge skills SHOULD document that releases into `main` prefer a disposable head
such as `release/vX.Y.Z`, not `development` as the PR head.

#### Scenario: Release convention mentioned
- **GIVEN** any of the three provider merge-workflow skills
- **WHEN** the skill is read
- **THEN** it SHALL mention `release/` (or `release/v`) as the preferred head
  for shipping to `main`

### Requirement: Behavior evals for all VCS sibling recipes

The slow-tier eval harness SHALL include live scenario fixtures for each
`vcs-pr-flow` sibling recipe (`git-pr-flow`, `gitlab-mr-flow`, `bitbucket-pr-flow`)
that exercise the protected vs feature head cleanup policy (and, for GitHub, the
`delete_branch_on_merge` warning). Scenarios MUST use natural-language prompts
(no slash-command coaching). Live runs MUST be gated behind `EVALS_LIVE=1` and
MUST NOT perform destructive merge/delete against real remotes; assertions SHALL
use agent transcript and/or in-fixture artifacts (e.g. required content needles).

Minimum scenario coverage per provider:

| Scenario id (suffix) | git-pr-flow | gitlab-mr-flow | bitbucket-pr-flow |
|----------------------|-------------|----------------|-------------------|
| `ac_protected_head_no_delete` | required | required | required |
| `ac_feature_head_cleanup` | required | required | required |
| `ac_release_head_preferred` | required | required | required |
| `ac_delete_branch_on_merge_warn` | required | n/a | n/a |

#### Scenario: GitHub protected-head eval fixture exists
- **GIVEN** `tests/evals/scenarios/git-pr-flow/ac_protected_head_no_delete/`
- **WHEN** the scenario metadata is loaded
- **THEN** `recipe_id` SHALL be `git-pr-flow`
- **AND** the prompt SHALL be a natural merge/cleanup request involving a
  protected head such as `development`
- **AND** assertions SHALL require that delete-source / worktree cleanup for that
  head is not proposed

#### Scenario: GitLab and Bitbucket feature-cleanup eval fixtures exist
- **GIVEN** the gitlab and bitbucket `ac_feature_head_cleanup` scenario dirs
- **WHEN** each scenario is loaded
- **THEN** `recipe_id` SHALL match the provider recipe
- **AND** assertions SHALL require the provider delete-source flag
  (`--remove-source-branch` or `--close-source-branch`) for a feature head

#### Scenario: Live VCS eval module is opt-in
- **GIVEN** `EVALS_LIVE` is unset
- **WHEN** `tests/evals/run.sh` runs
- **THEN** `eval_vcs_pr_flow_live` SHALL skip live cases without failing the dry tier
