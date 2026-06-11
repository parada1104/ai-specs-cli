## Why

VCS provider recipes (`git-pr-flow`, `gitlab-mr-flow`, and the in-flight `bitbucket-pr-flow`)
each encode a single host tool, yet they still expose a `provider` config key that duplicates
what the recipe id and `[[bindings]]` already express. That redundancy confuses authors,
invites misconfiguration (e.g. `gitlab-mr-flow` with `provider = "github"`), and blurs the
line between foundational patterns and concrete integrations.

## What Changes

- **BREAKING (soft):** Remove `[config.provider]` from all VCS sibling recipes; manifests
  that still set `provider` receive a sync warning and the key is ignored.
- Replace `{config.provider}` in `[provides.brief].workflow_rules` with fixed, recipe-specific
  prose (GitHub/gh, GitLab/glab, Bitbucket/bb).
- Update `agents-render.py` to derive the Runtime Flow VCS bullet from the bound recipe id,
  not from `recipes[<id>].config.provider`.
- Skills, commands, and READMEs reference only `base_branch` as project config.
- Reclassify VCS recipes in docs as **specific** `vcs-pr-flow` providers (not generic
  multi-provider recipes).
- Land `bitbucket-pr-flow` in the same apply cycle **without** a `provider` field (cherry-pick
  or merge from `feat/bitbucket-pr-flow` worktree).

## Capabilities

### New Capabilities

_(none)_

### Modified Capabilities

- `vcs-pr-flow`: recipe manifests no longer declare `provider`; binding selects the host;
  only `base_branch` remains configurable per project.
- `runtime-brief-rendering`: VCS Runtime Flow bullet sourced from bound recipe id mapping.

## Impact

| Area | Change |
|------|--------|
| `catalog/recipes/git-pr-flow/` | Drop `provider` config; fix brief fragments, skill, command, README |
| `catalog/recipes/gitlab-mr-flow/` | Same |
| `catalog/recipes/bitbucket-pr-flow/` | Add from parallel branch without `provider` |
| `lib/_internal/agents-render.py` | Recipe-id → VCS label map |
| `docs/recipes-catalog.md`, `docs/capabilities.md`, `docs/recipe-schema.md` | Doc alignment |
| `openspec/specs/vcs-pr-flow/spec.md` | Promoted after archive |
| Tests | Recipe manifest, render, catalog contract, sync pipeline |

**Rollback:** Revert recipe.toml `provider` fields and restore renderer config lookup; no
data migration required.
