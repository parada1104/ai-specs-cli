# Design: VCS Pre-merge Artifacts

## Technical Approach

Promote the delta spec into the canonical `vcs-pr-flow` contract, then mirror the same archive-before-merge instruction into agent-facing VCS workflow skills. The rule remains invisible to end users: no new slash commands, no command changes, and no runtime brief/user README wording unless needed for sync consistency. Recipe versions and lock hashes move with the touched bundled skills so `ai-specs sync` stays deterministic.

## Architecture Decisions

| Decision | Choice | Alternatives considered | Rationale |
|---|---|---|---|
| Contract source | Add the requirement to `openspec/specs/vcs-pr-flow/spec.md` during archive | Keep only in provider skills | The spec is the provider-neutral source; skills must mirror it, not invent it. |
| Visibility | Put the rule in merge-workflow/worktree skills, not user commands or brief rules | Add visible docs/commands | Card #38 requires hidden ceremony; agent workflow guidance is sufficient. |
| Versioning | Bump touched recipe patch versions and refresh matching lock entries | Leave versions unchanged | Catalog consumers need a coherent recipe+skill update boundary. |
| Tests | Add golden text and lock/materialization checks | Manual review only | This is prompt/content behavior, so drift tests are the practical safety net. |

## Data Flow

```text
OpenSpec delta
  -> archived canonical spec (`openspec/specs/vcs-pr-flow/spec.md`)
  -> provider merge skills (`catalog/recipes/*-flow/skills/*merge-workflow/SKILL.md`)
  -> recipe version/hash refresh (`recipe.toml`, `ai-specs/.ai-specs.lock`)
  -> sync materializes updated hidden agent guidance
```

At merge time:

```text
Agent finishes verify -> archives/records SDD artifacts on review branch
  -> user approves provider PR/MR merge
  -> provider merge command runs
  -> post-merge cleanup/sync proceeds
```

## File Changes

| File | Action | Description |
|---|---|---|
| `openspec/specs/vcs-pr-flow/spec.md` | Modify | Add the pre-merge archive requirement from the delta spec. |
| `catalog/recipes/git-pr-flow/skills/git-merge-workflow/SKILL.md` | Modify | Add a pre-merge checklist/guardrail before `gh pr merge`. |
| `catalog/recipes/gitlab-mr-flow/skills/gitlab-merge-workflow/SKILL.md` | Modify | Add the same rule before `glab mr merge`. |
| `catalog/recipes/bitbucket-pr-flow/skills/bitbucket-merge-workflow/SKILL.md` | Modify | Add the same rule before `bb pr merge`. |
| `catalog/recipes/worktree-flow/skills/worktree-flow/SKILL.md` | Modify | Clarify that file-writing change work includes planning/SDD artifacts. |
| `catalog/recipes/{git-pr-flow,gitlab-mr-flow,bitbucket-pr-flow,worktree-flow}/recipe.toml` | Modify | Patch-bump recipes whose bundled skill content changes. |
| `ai-specs/ai-specs.toml` | Modify | Update pinned versions for dogfood-enabled bumped recipes (`git-pr-flow`, `worktree-flow`). |
| `ai-specs/.ai-specs.lock` | Modify | Refresh bundled skill hashes for changed dogfood-installed recipes; GitLab/Bitbucket stay absent unless enabled by the manifest. |
| `tests/test_{git_pr_flow,gitlab_mr_flow,bitbucket_pr_flow,worktree_flow}_recipe.py` | Modify | Add golden assertions for hidden pre-merge archive guidance. |
| `tests/test_lock.py` or sync/materialization tests | Modify | Pin lock round-trip/refresh behavior if new recipe skill lock entries are added. |

## Interfaces / Contracts

No new public CLI or manifest contract. Internal wording contract: each provider merge skill MUST require archived/recorded SDD/OpenSpec artifacts before the merge step and MUST identify the review branch as the archive boundary.

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Unit/golden | Each provider skill contains the pre-merge archive rule before merge wording | Extend existing recipe golden tests. |
| Materialization | Updated skills still materialize under `ai-specs/.recipe/<recipe>/skills/...` | Existing recipe materialization tests plus assertions if needed. |
| Lock | Changed bundled skills have coherent lock hashes | Use lock/helper tests or run sync/refresh path in fixture. |
| Validation | Repository remains valid | Run `./tests/run.sh`, then `./tests/validate.sh`. |

## Migration / Rollout

No data migration required. Existing consumers receive the rule when they sync the bumped recipes; no user workflow changes.

## Open Questions

None.
