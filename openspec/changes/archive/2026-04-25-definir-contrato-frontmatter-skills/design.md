## Context

`ai-specs` discovers local skills from `ai-specs/skills/**/SKILL.md`, vendors external skills from `[[deps]]`, renders generated `AGENTS.md`, and then runs `skill-sync` to populate Auto-invoke tables. Before this change, these paths interpreted frontmatter separately, which made it easy for local skills, vendored skills, and generated AGENTS rows to drift.

The existing worktree already contains a viable implementation. The design formalizes that implementation after updating it to current `development` so the change can be verified and archived instead of being discarded as stale work.

## Goals / Non-Goals

**Goals:**

- Define canonical required and optional frontmatter fields for local skills.
- Define generated frontmatter for vendored skills based on manifest `[[deps]]` inputs.
- Use one shared Python helper for parsing and normalizing frontmatter in renderer and vendoring code.
- Keep a temporary compatibility mode for existing first-party skills.
- Preserve byte-consistent fan-out for vendored skills across root and subrepos.
- Make incomplete auto-invoke metadata fail with actionable errors.

**Non-Goals:**

- No general YAML dependency; parsing remains a minimal in-repo parser for the supported frontmatter subset.
- No complete schema engine for every skill body field.
- No hard cutover that breaks untouched legacy first-party skills in this change.
- No unrelated OpenSpec config cleanup.

## Decisions

| Decision | Choice | Alternatives considered | Rationale |
|---|---|---|---|
| Contract location | `ai-specs/contracts/skill-frontmatter.md` | README-only docs or hidden test fixtures | Keeps the contract versioned and discoverable near skill assets. |
| Enforcement module | `lib/_internal/skill_contract.py` | Keep parsing duplicated in renderer/vendor/sync shell | Shared enforcement avoids drift between consumers. |
| Local skill compatibility | Allow compatibility defaults for first-party rollout | Immediate hard fail on every missing field | Existing bundled/local skills need a safe migration path. |
| Vendored ownership | Generate vendored frontmatter from `[[deps]]` plus upstream body | Preserve upstream frontmatter verbatim | Vendored output must be reproducible from the manifest and upstream source. |
| Sync metadata rule | Require `metadata.scope` and `metadata.auto_invoke` together only when a skill participates in Auto-invoke | Require both for every skill | Manual-only local skills should remain valid without Auto-invoke rows. |

## Risks / Trade-offs

| Risk | Mitigation |
|---|---|
| Minimal parser misses unsupported YAML syntax | Contract documents the supported subset and tests cover the accepted forms. |
| Compatibility defaults hide missing metadata too long | Contract includes a hard-fail cutover note for a later change. |
| Shell `skill-sync` and Python contract diverge | Tests cover sync output and invalid metadata behavior; future work can migrate more shell parsing into Python. |
| Generated bundled assets drift | Regenerate with `ai-specs sync` and commit lockfile updates together with source changes. |

## Migration Plan

1. Keep compatibility mode on for local first-party skill reading.
2. Generate canonical frontmatter for vendored skills during root sync.
3. Update bundled/local skill files to include required metadata where they are owned by this repo.
4. Validate with targeted unit and sync pipeline tests.
5. Archive the change and make the main spec the future source of truth.

Rollback: revert the contract doc, helper, consumer changes, generated assets, and tests. No persisted user data migration is required.

## Open Questions

- When should compatibility defaults become hard failures for all local skills? This is explicitly left for a later cutover change.
