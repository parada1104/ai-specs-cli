## Exploration: gitlab-mr-flow recipe

### Current State
`git-pr-flow` is a catalog recipe under `catalog/recipes/git-pr-flow/` with this anatomy: `[recipe]` metadata, `[[capabilities]] id = "vcs-pr-flow"`, an `on-sync` `validate-config` hook, optional `provider` and `base_branch` config defaults, a bundled `git-merge-workflow` skill, `/pr-create` command, runtime-brief workflow fragments, and README documentation copied via `provides.docs`.

Recipes are discovered dynamically from `catalog/recipes/<id>/recipe.toml` by `recipe-list.py`, `recipe-add.py`, and `recipe-materialize.py`; there is no central registration table for normal catalog discovery. Some secondary surfaces are hardcoded/documented and should be updated for discoverability: `docs/recipes-catalog.md`, `docs/capabilities.md`, and `lib/_internal/rules-inventory.py`.

The `vcs-pr-flow` capability is already canonical. `resolve_bindings()` auto-binds it when exactly one enabled recipe provides it. If both `git-pr-flow` and a future `gitlab-mr-flow` are enabled with no explicit `[[bindings]]`, sync warns about capability ambiguity and leaves `vcs-pr-flow` unbound; an explicit binding resolves the provider. Primitive conflicts are separate and only cover skill, command, and MCP IDs, so using distinct IDs (`gitlab-merge-workflow`, `mr-create`) avoids collisions.

`validate-config` is generic: it checks required config, optional regex validation, and a Trello-specific `board_id` guard. It does not check CLI installation or authentication. The GitLab recipe should keep `glab` installation/auth checks as runtime preconditions in the skill/command.

`glab` equivalents for the GitHub flow are straightforward: `git push -u origin <branch>`, then `glab mr create --target-branch <base_branch> --source-branch <branch> --title "..." --description "..." --yes`. CI can be inspected with `glab ci status --branch <branch>` if needed. Avoid `glab mr create --fill` in the command path unless explicitly desired, because `--fill` sets `push=true` and may push implicitly.

### Affected Areas
- `catalog/recipes/gitlab-mr-flow/recipe.toml` — new recipe manifest providing `vcs-pr-flow`, defaults `provider = "gitlab"` and `base_branch = "development"`, hook, bundled skill, command, brief fragments, and README doc target.
- `catalog/recipes/gitlab-mr-flow/skills/gitlab-merge-workflow/SKILL.md` — bundled GitLab workflow using `glab`; should target configured base branch and never enable auto-merge.
- `catalog/recipes/gitlab-mr-flow/commands/mr-create.md` — slash command mirroring `/pr-create` but creating GitLab merge requests and stopping after MR creation.
- `catalog/recipes/gitlab-mr-flow/README.md` — human docs mirroring git-pr-flow structure and documenting explicit `[[bindings]]` when both providers are enabled.
- `docs/recipes-catalog.md` — catalog table and recipe section should include the new recipe.
- `docs/capabilities.md` — update `vcs-pr-flow` typical provider text from future GitLab to actual `gitlab-mr-flow`.
- `tests/test_gitlab_mr_flow_recipe.py` — mirror `test_git_pr_flow_recipe.py` to validate manifest, capability, bundled skill, command, and materialization.
- `tests/test_recipe_materialize.py` / `tests/test_recipe_conflicts.py` — existing generic capability ambiguity coverage is sufficient, but adding a targeted `git-pr-flow` + `gitlab-mr-flow` binding case would protect the provider-swap contract.
- `tests/test_sync_pipeline.py` or `tests/test_agents_render_brief_fragments.py` — optional targeted coverage that binding `vcs-pr-flow` to `gitlab-mr-flow` renders `provider = gitlab` and the configured base branch.
- `lib/_internal/rules-inventory.py` — optional discoverability update if migration inventory should suggest GitLab flow for MR/GitLab wording.

### Approaches
1. **Sibling provider recipe** — Add `gitlab-mr-flow` as a new recipe that mirrors `git-pr-flow` but uses GitLab-specific skill and command IDs.
   - Pros: Matches the documented capability/binding model; low code risk; no provider abstraction refactor; no primitive conflicts with `git-pr-flow`; easy to test by mirroring existing recipe tests.
   - Cons: Duplicates some workflow prose with `git-pr-flow`; projects that enable both must add explicit `[[bindings]]`.
   - Effort: Low

2. **Provider abstraction refactor** — Turn `git-pr-flow` into a generic GitHub/GitLab provider switch and branch inside one skill/command.
   - Pros: Less duplicated recipe structure; one recipe owns the `vcs-pr-flow` capability.
   - Cons: Contradicts the current docs that name future GitLab as a sibling recipe; larger tests/docs impact; harder to keep provider-specific commands (`/pr-create` vs `/mr-create`) clear; higher risk for existing GitHub projects.
   - Effort: High

3. **Thin GitLab recipe with shared renderer changes** — Add the sibling recipe and also change `agents-render.py` to emit a `(`glab` CLI)` suffix for `provider = "gitlab"`.
   - Pros: Keeps the sibling recipe model and improves runtime-brief clarity.
   - Cons: Slightly broadens scope beyond recipe assets; requires renderer tests; not necessary if recipe brief, README, skill, and command are explicit about `glab`.
   - Effort: Medium

### Recommendation
Use Approach 1. Implement `gitlab-mr-flow` as a sibling provider recipe with distinct primitive IDs and the same `vcs-pr-flow` capability. Keep the implementation asset-only unless tests expose a renderer gap; runtime brief already reads `provider` and `base_branch` from the recipe bound to `vcs-pr-flow`, and recipe docs/skill/command can provide the GitLab-specific `glab` details.

For the command path, prefer explicit push followed by MR creation:

```bash
git push -u origin <branch-name>
glab mr create --target-branch <base_branch> --source-branch <branch-name> --title "<title>" --description "<summary and verification>" --yes
```

Do not use `--auto-merge`; do not merge during `/mr-create`; leave CI and merge automation out of scope.

### Risks
- Enabling both `git-pr-flow` and `gitlab-mr-flow` without explicit `[[bindings]]` leaves `vcs-pr-flow` unbound after a sync warning, so the generated runtime brief may not name an active VCS provider even though both commands/skills materialize.
- `glab mr create --fill` can push implicitly; using it would weaken the existing explicit-push guardrail.
- `validate-config` will not prove `glab` is installed or authenticated; the skill/command must check `command -v glab` and `glab auth status` at runtime.
- `base_branch = "development"` intentionally differs from `git-pr-flow`'s `main` default; tests and docs must make that default explicit.
- Renderer currently special-cases only GitHub with a `gh CLI` suffix; GitLab clarity must come from recipe prose unless renderer scope is expanded.

### Ready for Proposal
Yes. The orchestrator should proceed to proposal/spec/design for a low-risk sibling recipe, explicitly calling out the provider binding behavior when both GitHub and GitLab recipes are enabled.
