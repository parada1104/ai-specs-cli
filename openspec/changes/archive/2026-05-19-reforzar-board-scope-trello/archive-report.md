# Archive Report: Reforzar Board Scope en Trello MCP Workflow

**Archived**: 2026-05-19
**Change folder**: `openspec/changes/archive/2026-05-19-reforzar-board-scope-trello/`
**Branch**: `reforzar-board-scope-trello` (base: `development`)

## Change Summary

Reinforced board isolation in the Trello MCP Workflow recipe. Added forbidden-tool enforcement, board guard post-set verification, explicit boardId on all MCP calls, card idBoard validation, and template cleanup. All changes are at the recipe/skill level — no system contract specs were modified.

## What Was Implemented

| Priority | Item | Status |
|----------|------|--------|
| P1 | Forbidden Tools (trello_get_my_cards, trello_list_boards) | ✅ Done |
| P2 | Board Guard post-set verification with retry | ✅ Done |
| P3 | Explicit boardId on 5 MCP calls | ✅ Done |
| P4 | Card idBoard validation before get_card/add_comment | ✅ Done |
| P5 | Board guard precondition (step 0) in all capabilities | ✅ Done |
| P6 | Template cleanup (remove SDD Checklist section) | ✅ Done |
| P7 | board_isolation config schema in recipe.toml | ✅ Done |

## Files Changed (7 files)

| File | Change |
|------|--------|
| `catalog/recipes/trello-mcp-workflow/skills/trello-mcp-workflow/SKILL.md` | Added forbidden/restricted tools, board guard, explicit boardId, card validation |
| `catalog/recipes/trello-mcp-workflow/recipe.toml` | Added `[config.board_isolation]` schema |
| `catalog/recipes/trello-mcp-workflow/templates/card-feature.md` | Removed SDD Checklist section, added pm-skill reference |
| `catalog/recipes/trello-mcp-workflow/commands/trello-workflow.md` | Added forbidden/restricted tools reference table |
| `ai-specs/skills/trello-pm-workflow/SKILL.md` | Added board isolation cross-reference |
| `lib/_internal/recipe_schema.py` | Added `ConfigSchema.extra` for non-standard config sections |
| `tests/test_recipe_schema.py` | Added `test_nonstandard_config_section_parses` |

## Verification Result

**Verdict**: PASS WITH WARNINGS
**Mode**: Strict TDD

- 15/15 spec scenarios compliant
- 11/11 tasks complete
- 5/5 recipe_schema tests pass
- 6 pre-existing failures in full validation suite (unrelated)

**Warnings** (2 minor doc issues, no functional impact):
1. Restricted Tools numbering: header says "step 2", body says "step 1" — actual impl uses step 1 (board guard retry) and step 3
2. Bootstrap step ordering swapped between spec description and implementation (functionally correct)

## Commits (4)

```
58cf5c5 feat(trello-recipe): add board_isolation config schema and clean up card template
36f839b feat(trello-recipe): add board guard, forbidden tools, explicit boardId, and card validation
66bf038 docs(trello-recipe): update commands reference and pm-skill cross-reference
dc3f7b1 feat(schema): support non-standard config sections for recipe.toml
```

## Engram Artifact IDs

| Artifact | Topic Key | Observation ID |
|----------|-----------|---------------|
| Proposal | `sdd/reforzar-board-scope-trello/proposal` | #603 |
| Spec (Delta) | `sdd/reforzar-board-scope-trello/spec` | #605 |
| Design | `sdd/reforzar-board-scope-trello/design` | #604 |
| Tasks | `sdd/reforzar-board-scope-trello/tasks` | #606 |
| Apply Progress | `sdd/reforzar-board-scope-trello/apply-progress` | #607 |
| Verify Report | `sdd/reforzar-board-scope-trello/verify-report` | #609 |
| Archive Report | `sdd/reforzar-board-scope-trello/archive-report` | (this) |

## Specs Synced

**None** — all changes are recipe/skill-level. No system contract specs required updates. The only system change (`recipe_schema.py` extra dict support) is an implementation detail for non-standard config sections, not a contract change.

## SDD Cycle Complete

The change has been fully planned, implemented, verified, and archived.
