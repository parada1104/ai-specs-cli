# Design: GitLab MR Flow Recipe

## Technical Approach

Add `gitlab-mr-flow` as a sibling catalog recipe to `git-pr-flow`, reusing the existing recipe schema, sync-time `validate-config` action, capability binding resolver, and materialization pipeline. The change is asset-first: new recipe manifest, bundled skill, slash command, README, docs, and tests. No renderer or provider abstraction change is planned unless tests expose a gap.

## Architecture Decisions

| Decision | Choice | Alternatives considered | Rationale |
|---|---|---|---|
| Provider model | New sibling recipe providing `vcs-pr-flow` | Extend `git-pr-flow` with provider branches | Existing docs/specs define provider swapping through `[[bindings]]`; separate skill/command IDs avoid primitive conflicts. |
| MR command sequence | `command -v glab` → `glab auth status` → `git push -u origin <branch>` → `glab mr create --source-branch <branch> --target-branch <base_branch> --title ... --description ... --yes` | Use `glab mr create --fill`; rely on glab to push | Explicit push preserves the GitHub recipe safety model; `--fill` can push implicitly and is forbidden by the spec. |
| Error handling | Stop before push when `glab` is missing/unauthenticated; stop after push failure; if MR creation fails, report that the branch may already be pushed and give retry context | Best-effort fallbacks or web UI handoff | Avoids hidden side effects and keeps failures actionable. |
| Config validation | Use existing `[[hooks]] event = "on-sync", action = "validate-config"`; no new hook script | Add recipe-local shell hook | Current hook only validates manifest shape, required fields, defaults, and regex metadata. Runtime checks belong in skill/command. |
| Command arguments | `/mr-create [title] [description/verification]`; no provider/base flags | Allow `--base`, `--fill`, `--merge` | Provider/base come from recipe config; merge automation is out of scope. Missing title/body are drafted from branch/status and confirmed before push. |
| Skill triggers | Auto-invoke for GitLab MR creation, merge approval, worktree cleanup, finishing a feature branch, and syncing the integration branch | Broad Git/PR triggers | Keeps GitLab-specific workflow discoverable without stealing generic git work. |

## Data Flow

```text
ai-specs.toml ──sync──> recipe-materialize.py ──copy──> ai-specs/.recipe/gitlab-mr-flow/skills/gitlab-merge-workflow
      │                         │                         ai-specs/commands/mr-create.md
      │                         └── validate-config ──> merged config only
      └── [[bindings]] ──> resolved-config.json ──> runtime brief selects vcs-pr-flow provider

/mr-create ──> inspect git status/branch ──> glab preflight ──> git push ──> glab mr create ──> MR URL, STOP
```

## File Changes

| File | Action | Description |
|---|---|---|
| `catalog/recipes/gitlab-mr-flow/recipe.toml` | Create | Metadata, `vcs-pr-flow`, `provider=gitlab`, `base_branch=development`, validate hook, skill/command/docs/brief provisions. |
| `catalog/recipes/gitlab-mr-flow/skills/gitlab-merge-workflow/SKILL.md` | Create | GitLab workflow guardrails, glab preflight, MR creation, approval-gated merge/cleanup. |
| `catalog/recipes/gitlab-mr-flow/commands/mr-create.md` | Create | Thin command for explicit push + MR creation; no merge and no `--fill`. |
| `catalog/recipes/gitlab-mr-flow/README.md` | Create | Enablement, config, explicit binding example, glab prerequisites, safety policy. |
| `docs/recipes-catalog.md` | Modify | Add catalog entry and recipe section. |
| `docs/capabilities.md` | Modify | Name `gitlab-mr-flow` as an actual `vcs-pr-flow` provider. |
| `tests/test_gitlab_mr_flow_recipe.py` | Create | Manifest/materialization/golden content coverage. |
| `tests/test_recipe_materialize.py` | Modify | Targeted GitHub+GitLab ambiguity and explicit binding coverage if generic tests are insufficient. |

## Interfaces / Contracts

```toml
[recipe]
id = "gitlab-mr-flow"
version = "1.0.0"

[[capabilities]]
id = "vcs-pr-flow"

[[hooks]]
event = "on-sync"
action = "validate-config"

[config.provider]
required = false
type = "string"
default = "gitlab"

[config.base_branch]
required = false
type = "string"
default = "development"

[provides]
skills = [{ id = "gitlab-merge-workflow", source = "bundled" }]
commands = [{ id = "mr-create", path = "commands/mr-create.md" }]
```

Dual-provider projects MUST bind explicitly:

```toml
[[bindings]]
capability = "vcs-pr-flow"
recipe = "gitlab-mr-flow"
```

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Unit | Recipe schema, config defaults, hook declaration, provides | Mirror `test_git_pr_flow_recipe.py`; parse TOML and assert values. |
| Unit/golden | Skill and command text | Assert preflight commands, push-before-create order, `--target-branch`, `--description`, and absence of `--fill`/auto-merge. |
| Integration-ish unit | Materialization and bindings | Temp project with recipe enabled; assert skill/command/README paths, ambiguity warning/unbound behavior, explicit binding resolution. |
| Validation | Repo health | `./tests/run.sh`, then `./tests/validate.sh`. |

## Migration / Rollout

No data migration required. Projects opt in by enabling `gitlab-mr-flow`; projects enabling both providers add explicit `[[bindings]]`.

## Open Questions

None.
