# Design: VCS Drop Deferred Cleanup

## Technical Approach

Update the existing renderer path, not the recipe materializer. `recipe-materialize.py` already writes `resolved["bindings"]`, `resolved["enabled"]`, defaults, and `brief_fragments`; `agents-render.py` currently iterates all enabled recipes for fragments while the Runtime Flow VCS bullet already reads `bindings["vcs-pr-flow"]`. The fix narrows VCS `workflow_rules` fragment collection to the bound recipe, adds a custom-id label fallback warning, and mirrors the existing GitLab/Bitbucket docs contract for GitHub.

## Architecture Decisions

| Decision | Choice | Alternatives considered | Rationale |
|---|---|---|---|
| Fragment filter | Keep `collect_recipe_brief_fragments()` iteration, add an optional allow-list used by `_section_workflow_rules()` for bound VCS ids. | Direct lookup of only the bound recipe. | Preserves ordering, substitution, key/text dedupe, and missing-recipe tolerance while filtering only VCS siblings. |
| Generic label | Render `VCS PR (custom)` in the existing bullet shape. | `Unknown VCS recipe`, omit bullet. | Safe, user-facing, and confirms a VCS flow exists without pretending to know the host/CLI. |
| Warning de-dupe | Warn once per unknown id per render using a local `set`, not module-global state. | Warn every occurrence; warn once per process. | Avoids noisy duplicate bindings while keeping repeated render/test invocations observable. |
| Warning target | `print(..., file=sys.stderr)` with `⚠ ai-specs:` prefix. | `warnings.warn`; custom logger. | Matches CLI stderr patterns and is easy for subprocess tests to assert. |
| Docs contract scope | Check both README and `docs/recipes-catalog.md`. | README only. | The spec requires both and existing GitLab/Bitbucket tests already mirror both surfaces. |

## Data Flow

```text
ai-specs.toml + catalog recipes
  └─ recipe-materialize.py → resolved-config.json
       ├─ bindings["vcs-pr-flow"] = bound recipe id
       ├─ enabled = all enabled recipes
       └─ recipes[*].brief_fragments/defaults
            ↓
agents-render.py
  ├─ _section_workflow_rules() → collect_recipe_brief_fragments(..., allowed_vcs={bound})
  └─ _section_runtime_flow() → _VCS_RECIPE_LABELS lookup
          └─ unknown id → stderr warning + `VCS PR (custom)` bullet
```

Edge cases: no VCS binding emits no VCS sibling fragments; one bound + two unbound emits only the bound fragments; known ids keep existing labels; custom bound ids emit no host-specific fragments unless their own recipe is enabled and warn for the generic bullet.

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `lib/_internal/agents-render.py` | Modify | Add optional fragment allow-list/filtering, VCS sibling known-set handling, generic custom label warning helper, and local warning de-dupe. |
| `tests/test_agents_render_brief_fragments.py` | Modify | Add renderer unit tests for 3 enabled VCS recipes + 1 binding, no binding, custom id fragments, and unknown-id warning/bullet. |
| `tests/test_sync_pipeline.py` | Modify | Add integration-style subprocess coverage for unknown bound VCS id warning to stderr and generic bullet in `AGENTS.md`. |
| `tests/test_recipes_catalog.py` | Modify | Add `GitPrFlowDocsContractTests` mirroring GitLab/Bitbucket README and catalog no-`provider` assertions. |

## Interfaces / Contracts

- `_VCS_RECIPE_LABELS` remains the known VCS recipe set.
- Warning format: `⚠ ai-specs: VCS recipe '<id>' is not in the known label set; using generic label 'VCS PR (custom)'` to stderr.
- New helper contract may be internal, e.g. `collect_recipe_brief_fragments(resolved, section, *, recipe_ids: set[str] | None = None)`.
- `git-pr-flow/recipe.toml` has only `config.base_branch`; no `provider`, so the docs contract is valid.

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | Bound-only `workflow_rules` fragments | Add RED tests in `tests/test_agents_render_brief_fragments.py`; assert GitLab present and GitHub/Bitbucket absent with all 3 enabled. |
| Unit | No binding / custom id edge cases | Assert no VCS sibling fragments without binding; custom bound recipe can contribute its own fragments if enabled. |
| Unit/CLI | Unknown VCS fallback | Capture stderr; assert exact `⚠ ai-specs:` warning once per id and `VCS/PR provider: VCS PR (custom)` with base branch if present. |
| Doc contract | `git-pr-flow` README/catalog omit `provider` | Mirror GitLab/Bitbucket tests in `tests/test_recipes_catalog.py`, including catalog section. |
| Final | Regression suite | Strict TDD: RED per item, GREEN implementation, then `./tests/run.sh` and `./tests/validate.sh`. |

## Migration / Rollout

No migration required — purely additive behavior change. Existing single-VCS adopters see no change; multi-VCS adopters benefit from fragment isolation; custom-VCS adopters now see a visible warning instead of silent fallback.

## Open Questions

None.
