# Design: Plan-Build-Flow v2 (Ambient)

## Technical Approach

Version 2.0.0 removed slash commands; 2.1.0 adds planning depth classification and hard gates without new schema surface.

### Change depth classifier

| Tier | Chain | Minimum before build/PR |
|------|-------|-------------------------|
| Full | explore → proposal → spec → design → tasks | tasks + (proposal or design) + spec delta |
| Standard | spec → tasks | tasks + spec delta |
| Light | tasks | tasks only |

Direct implementation verbs on requests without a change folder still run classify → plan → stop.

### Gates

1. **PR gate** — block `gh pr create` / equivalents until tier minimum files exist under `openspec/changes/<slug>/` and are committed on the review branch.
2. **Pre-merge archive gate** — archive-tail on review branch before merge; reject post-merge archive as boundary (aligns with `vcs-pr-flow`).

## File Changes

| File | Action |
|------|--------|
| `catalog/recipes/plan-build-flow/recipe.toml` | v2.1.0; classifier + gate brief rules |
| `catalog/recipes/plan-build-flow/skills/plan-build-flow/SKILL.md` | Classifier + Sections 7–10 gates |
| `catalog/recipes/plan-build-flow/README.md` | Document tiers and gates |
| `openspec/specs/plan-build-flow/spec.md` | Promote classifier + gate requirements |
| `tests/test_plan_build_flow_recipe.py` | AC11–AC13 |

## Testing Strategy

Assert classifier tiers in skill, PR/archive gate language, and brief fragments mention classify / PR / pre-merge archive without forbidden vocabulary.
