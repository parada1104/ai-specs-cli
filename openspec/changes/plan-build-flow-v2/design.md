# Design: Plan-Build-Flow v2 (Ambient)

## Technical Approach

Version bump to 2.0.0 signals a breaking recipe surface change: commands are removed, the bundled skill becomes the sole entry point via `auto_invoke` on substantial change work. Internal phase mapping (explore→proposal→spec→design→tasks before implementation; apply→verify→archive-tail after authorization) is preserved but never exposed as slash verbs.

## File Changes

| File | Action |
|------|--------|
| `catalog/recipes/plan-build-flow/recipe.toml` | Remove commands; ambient brief |
| `catalog/recipes/plan-build-flow/skills/plan-build-flow/SKILL.md` | Rewrite for ambient flow |
| `catalog/recipes/plan-build-flow/README.md` | Skill-only docs |
| `openspec/specs/plan-build-flow/spec.md` | Promote v2 requirements |
| `tests/test_plan_build_flow_recipe.py` | Replace command tests with ambient tests |

## Testing Strategy

Update AC1/AC9 tests: assert zero commands, skill auto_invoke present, brief references planning before implementation without `/plan`/`/build`.
