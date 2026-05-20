# Archive Report: docs-remove-sdd-refocus

**Archived**: 2026-05-18
**Previous location**: `openspec/changes/docs-remove-sdd-refocus/`
**Archive location**: `openspec/changes/archive/2026-05-18-docs-remove-sdd-refocus/`

## Verification Status

**PASS WITH WARNINGS** — No CRITICAL issues found.

All 11 tasks complete. All spec requirements (R1-R6) satisfied. All target files clean of SDD/OpenSpec references. Deleted files confirmed absent. No broken cross-links.

## Archival Summary

| Aspect | Detail |
|--------|--------|
| Spec merge to main | Not required — no `specs/` subdirectory (flat `spec.md` format). Change is docs-only cleanup, not a feature change. |
| Active changes cleaned | ✅ `docs-remove-sdd-refocus` removed from `openspec/changes/` |
| Artifact count | 5 artifacts preserved: proposal.md, spec.md, design.md, tasks.md, verify-report.md |

## Artifact Observation IDs (Engram)

| Artifact | Engram ID |
|----------|-----------|
| verify-report | #596 |

## Warnings Carried Forward

1. **Stale test assertions** — 3 tests in `tests/test_manifest_contract_docs.py` expect SDD references that were correctly removed:
   - `test_manifest_reference_lists_canonical_surface_and_compatibility_rules` — remove `Omission of [sdd] remains valid...` needle
   - `test_manifest_reference_marks_out_of_scope_items_as_deferred` — remove `[memory] (distinct from [sdd].artifact_store = memory)` needle
   - `test_readme_links_to_dedicated_manifest_and_recipe_references` — remove `[`docs/ai/sdd.md`](docs/ai/sdd.md)` needle

2. **Remaining SDD/OpenSpec references outside scope** — Several files still contain SDD references that were not in the task scope:
   - `docs/recipe-schema.md` (N3 protected)
   - `catalog/recipes/trello-mcp-workflow/templates/card-feature.md`
   - `catalog/recipes/trello-mcp-workflow/commands/trello-workflow.md`
   - `catalog/recipes/trello-mcp-workflow/recipe.toml`
   - `catalog/skills/testing-foundation/SKILL.md`
   - `catalog/skills/context-precedence/SKILL.md`
   - Consider a follow-up change if R1 is interpreted broadly.

3. **N1 minor violation** — `ai-specs/ai-specs.toml` had commented-out `[sdd]` block removed (4 comment lines). Zero functional impact.

4. **N4 violation by broader commit** — `lib/_internal/*.py`, `lib/sdd.sh`, `bin/ai-specs` changed as part of SDD product code removal commit, not specific to this doc change.

5. **Pre-existing test failures** — 2 assertions (links to `skills-by-agent.md` and `bundled-merge-rules.md`) were failing before this change.

## SDD Cycle Complete

The change has been fully planned, proposed, spec'd, designed, implemented, verified, and archived. Ready for the next change.
