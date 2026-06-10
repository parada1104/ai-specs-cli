# vcs-pr-flow Specification: GitLab MR Flow

## Purpose

Provide a GitLab-backed `vcs-pr-flow` recipe that mirrors `git-pr-flow` semantics while using GitLab merge requests, explicit branch pushes, and approval-gated merging.

## Requirements

### Requirement: Recipe Manifest

The `gitlab-mr-flow` recipe MUST declare `vcs-pr-flow`, default `provider = "gitlab"`, default `base_branch = "development"`, an `on-sync` `validate-config` hook, a bundled `gitlab-merge-workflow` skill, an `mr-create` command, and README doc provision.

#### Scenario: Manifest validates
- GIVEN the catalog recipe is loaded
- WHEN recipe schema validation runs
- THEN the recipe is valid and declares `vcs-pr-flow`
- AND skill, command, docs, config defaults, and hook are discoverable

### Requirement: Materialized Assets

Sync MUST materialize `gitlab-merge-workflow/SKILL.md`, `commands/mr-create.md`, and `ai-specs/recipes/gitlab-mr-flow/README.md` without changing GitHub recipe assets.

#### Scenario: Sync provisions assets
- GIVEN `gitlab-mr-flow` is enabled
- WHEN `ai-specs sync` runs
- THEN the GitLab skill, command, and README exist in generated locations

### Requirement: GitLab MR Workflow Skill

The skill MUST trigger for GitLab MR creation, merge, and cleanup requests, inspect branch/status, require verification evidence, use `git push -u origin <branch>`, then create an MR with `glab mr create --target-branch <base_branch> --title ... --description ...`, and MUST NOT merge without explicit user approval.

#### Scenario: Skill opens MR safely
- GIVEN a clean feature branch and completed verification
- WHEN the user requests an MR
- THEN the skill pushes explicitly before creating the MR against the configured base branch
- AND it stops after reporting the MR URL

### Requirement: Slash Command

`/mr-create` MUST be a thin command for MR creation only, reading recipe config, running the same explicit push-before-create flow, and avoiding `glab mr create --fill` because it can push implicitly.

#### Scenario: Command avoids implicit push
- GIVEN `/mr-create` is invoked
- WHEN it builds the GitLab command sequence
- THEN `git push -u origin <branch>` appears before `glab mr create`
- AND `--fill` is not used

### Requirement: Config Validation Hook

`validate-config` MUST validate manifest shape and config types/defaults only; it MUST NOT check whether `glab` is installed or authenticated.

#### Scenario: Manifest-only validation
- GIVEN valid TOML config but no `glab` binary
- WHEN `validate-config` runs during sync
- THEN sync validation does not fail because runtime tooling is absent

### Requirement: Provider Binding Semantics

When `git-pr-flow` and `gitlab-mr-flow` both provide `vcs-pr-flow`, the system MUST require an explicit `[[bindings]]` selection; without it, sync MUST warn and leave `vcs-pr-flow` unbound rather than picking a provider.

#### Scenario: Ambiguous providers stay unbound
- GIVEN both provider recipes are enabled without `[[bindings]]`
- WHEN sync resolves capabilities
- THEN a warning names the ambiguity
- AND no implicit `vcs-pr-flow` binding is selected

#### Scenario: Explicit binding selects GitLab
- GIVEN both provider recipes are enabled with a binding to `gitlab-mr-flow`
- WHEN sync resolves capabilities
- THEN `vcs-pr-flow` is bound to GitLab assets and brief rules

### Requirement: Runtime Checks and Docs

The skill and command MUST check `command -v glab` and `glab auth status` before MR creation, stop with actionable blockers on failure, and README MUST document enablement, config, explicit bindings, runtime prerequisites, explicit push behavior, and no auto-merge policy.

#### Scenario: Runtime blocker
- GIVEN `glab` is missing or unauthenticated
- WHEN MR creation is requested
- THEN the skill/command stops before pushing or creating an MR
- AND reports the exact install/auth blocker
