# Proposal: Remove SDD/OpenSpec from Documentation

## Intent

SDD/OpenSpec code was removed from the product (now handled by [gentle-ai](https://github.com/Gentleman-Programming/gentle-ai)), but documentation still references it heavily: README has an SDD key concept, AGENTS.md prescribes SDD workflow, `docs/ai/sdd.md` is a full SDD guide, and the catalog ships an `openspec-sdd-conventions` skill. This creates confusion — users see SDD commands and concepts that don't exist in the product. Clean all docs to reflect what ai-specs-cli actually provides: fan-out, harness engineering, skills, MCP servers, and recipes.

## Scope

### In Scope
- Delete `docs/ai/sdd.md` — full SDD guide (no longer relevant)
- Delete `docs/ai/examples/config.yaml` — OpenSpec `spec-driven` config (SDD-dependent)
- Delete `catalog/skills/openspec-sdd-conventions/SKILL.md` — SDD conventions skill
- Remove "SDD (Spec-Driven Development)" key concept and `ai-specs sdd` CLI row from `README.md`
- Remove OpenSpec/SDD references from `AGENTS.md` Purpose, Context Sources, Conflict Policy, Workflow Rules
- Remove `[sdd]` manifest section and SDD links from `docs/ai-specs-toml.md`
- Remove "SDD failures" section and SDD links from `docs/ai/troubleshooting.md`
- Remove `openspec-sdd-conventions` entry from `catalog/README.md`
- Rewrite `catalog/recipes/trello-mcp-workflow/README.md` to remove "SDD workflows" wording

### Out of Scope
- Modifying `ai-specs/ai-specs.toml` (dogfooding recipes/deps are internal to this repo)
- Modifying `openspec/specs/` (internal to this repo's own SDD, not user-facing docs)
- Modifying `docs/recipe-schema.md`, `docs/mcp-distribution.md`, `docs/skills-by-agent.md`, `docs/bundled-merge-rules.md` (no SDD content)
- Any code changes to `lib/` or `src/`

## Capabilities

### New Capabilities
None — documentation-only change.

### Modified Capabilities
None — no spec-level behavior changes.

## Approach

File-by-file cleanup following the established concise doc style:

1. **README.md**: Remove `ai-specs sdd` row from CLI table. Replace "SDD (Spec-Driven Development)" key concept with a brief note that agent orchestration is handled by gentle-ai. Remove "and SDD configuration" from manifest description.
2. **AGENTS.md**: Strip "and OpenSpec/SDD workflows" from Purpose. Replace "OpenSpec controls SDD artifacts; Vault" with "Vault" in Context Sources and Conflict Policy. Change "Use the project SDD workflow" to a neutral harness-engineering note.
3. **docs/ai-specs-toml.md**: Cut entire `[sdd]` manifest section. Remove `[sdd]` from Canonical V1 surface. Remove SDD links from See also.
4. **docs/ai/troubleshooting.md**: Delete "SDD failures" section (3 subsections). Remove SDD link from See also.
5. **catalog/README.md**: Remove `openspec-sdd-conventions` skill entry and `[[deps]]` example.
6. **catalog/recipes/trello-mcp-workflow/README.md**: Change "ai-specs SDD workflows" → "ai-specs projects".

**Files deleted**: 3 (`sdd.md`, `config.yaml`, `openspec-sdd-conventions/SKILL.md`)
**Files modified**: 6

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `README.md` | Modified | Remove SDD section, CLI row |
| `AGENTS.md` | Modified | Remove OpenSpec/SDD from runtime brief |
| `docs/ai-specs-toml.md` | Modified | Remove `[sdd]` manifest section |
| `docs/ai/troubleshooting.md` | Modified | Remove SDD failures section |
| `docs/ai/sdd.md` | Removed | Full SDD guide — DELETE |
| `docs/ai/examples/config.yaml` | Removed | OpenSpec config example — DELETE |
| `catalog/README.md` | Modified | Remove sdd-conventions entry |
| `catalog/skills/openspec-sdd-conventions/` | Removed | SDD conventions skill — DELETE |
| `catalog/recipes/trello-mcp-workflow/README.md` | Modified | Remove "SDD workflows" wording |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Users bookmarked `docs/ai/sdd.md` and get 404 | Low | SDD docs were for a removed feature; gentle-ai docs cover SDD now |
| Trello recipe users see changed README | Low | Change is cosmetic ("SDD workflows" → "projects"), no behavior change |
| Cross-links from other repos to deleted files | Low | Only referenced from within this repo's own docs |

## Rollback Plan

`git revert` the commit that applies these changes. All deletions and modifications are in a single isolated branch (`docs-remove-sdd-refocus`).

## Dependencies

None.

## Success Criteria

- [ ] `grep -ri 'sdd\|openspec\|spec-driven' docs/ README.md AGENTS.md catalog/README.md catalog/recipes/trello-mcp-workflow/README.md` returns zero SDD-related hits (excluding gentle-ai references)
- [ ] `docs/ai/sdd.md`, `docs/ai/examples/config.yaml`, and `catalog/skills/openspec-sdd-conventions/SKILL.md` no longer exist
- [ ] README still accurately describes core primitives: fan-out, harness, skills, MCP, recipes
- [ ] All other docs remain internally consistent (no broken cross-references)
