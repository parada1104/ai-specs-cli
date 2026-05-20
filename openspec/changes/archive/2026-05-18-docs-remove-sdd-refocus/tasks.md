# Tasks: Remove SDD/OpenSpec from Documentation

## Phase 1: Deletions (no dependencies)

- [x] 1.1 Delete `docs/ai/sdd.md` — full SDD provider guide, product no longer ships SDD
- [x] 1.2 Delete `docs/ai/examples/config.yaml` — OpenSpec `spec-driven` config example
- [x] 1.3 Delete `catalog/skills/openspec-sdd-conventions/` directory (includes SKILL.md) — SDD conventions skill, no OpenSpec workflow to reference

## Phase 2: Reference-document modifications (links to deleted files)

- [x] 2.1 Modify `docs/ai-specs-toml.md` — remove `[sdd]` from surface list, field table, manifest sections, example, and See also; remove SDD link from Compatibility rules
- [x] 2.2 Modify `docs/ai/troubleshooting.md` — delete "SDD failures" section (3 subsections); remove SDD link from See also; update subtitle
- [x] 2.3 Modify `docs/ai/examples/minimal-manifest.toml` — remove `[sdd]` block

## Phase 3: Top-level document modifications

- [x] 3.1 Modify `README.md` — remove `ai-specs sdd` CLI row; replace SDD key concept with "Harness engineering" paragraph; update manifest description; no fluff added
- [x] 3.2 Modify `AGENTS.md` — strip OpenSpec/SDD from Purpose, Runtime Flow, Context Sources, Conflict Policy, and Workflow Rules; replace with neutral harness/project-tracking language

## Phase 4: Catalog modifications

- [x] 4.1 Modify `catalog/README.md` — remove `openspec-sdd-conventions` entry and its `[[deps]]` example; update "Repeat with" sentence
- [x] 4.2 Modify `catalog/recipes/trello-mcp-workflow/README.md` — change "ai-specs SDD workflows" → "ai-specs projects"
- [x] 4.3 Modify `catalog/recipes/trello-mcp-workflow/skills/trello-mcp-workflow/SKILL.md` — remove all SDD/OpenSpec language; generalize trello-card-linking, trello-state-sync, trello-progress-comment, and graceful-degradation triggers to project-agnostic wording
