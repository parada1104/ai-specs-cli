# Proposal: Option C — Generate the Rich Runtime Brief from the Manifest

## Intent

Today `agents-render.py` emits only the project name + MCP table, so this repo opts its rich `AGENTS.md` out of generation via the `<!-- ai-specs:runtime-brief -->` marker and maintains it by hand. This causes three problems:

- **Duplication**: `board_id`, `integration_branch`, `test_command`, `vault_scope` live in BOTH `ai-specs.toml` and the hand-written `AGENTS.md`.
- **Non-idempotent sync**: `ai-specs sync` cannot be safely re-run here without clobbering the manual brief (the marker is the only thing protecting it).
- **Doc contradiction**: repo docs say "never hand-edit AGENTS.md — it is auto-generated", yet this repo does exactly that.

Option C enriches the manifest schema + renderer so the GENERATED brief equals the rich one, letting the marker be removed and `AGENTS.md` become 100% generated and reproducible.

## Scope

### In Scope
- New `[brief]` table in `ai-specs.toml` for prose fields (intro, purpose, runtime_flow, context_sources, conflict_policy, workflow_rules, mcp_descriptions), using string arrays for bullet lists.
- Enrich `agents-render.py` to emit the rich brief: compose `[project]` + `[agents]` + `[brief]` prose + structured fields resolved from recipe configs (board_id, integration_branch, test_command, vault_scope) + capability-binding lookups naming the tracker / canonical-store / vcs provider.
- Wire resolved bindings/config into the renderer via a `--resolved-config <json>` arg emitted by `sync.sh` (mirror the existing `--recipe-mcp` pattern). The renderer gets NO catalog access.
- Remove the `<!-- ai-specs:runtime-brief -->` marker from this repo's `AGENTS.md`/`CLAUDE.md` once generated ≈ manual.
- Golden/needle + idempotency tests; update `docs/ai-specs-toml.md`.

### Out of Scope
- Per-recipe `[brief]` fragments (C-1) — deferred to a future "recipe brief contributions" change; no `recipe.toml` schema changes.
- The stale Auto-invoke table (handled via SKILL.md frontmatter, not AGENTS.md).
- `--preserve-if-runtime-brief` stays as a PERMANENT escape hatch — not removed.

## Capabilities

### New Capabilities
- `runtime-brief-rendering`: how `agents-render.py` composes the rich `AGENTS.md` runtime brief from `[project]`, `[agents]`, the new `[brief]` table, and structured fields/capability bindings supplied via `--resolved-config`.

### Modified Capabilities
- None. (`mcp-env-rendering` is unaffected; the MCP table behavior is preserved.)

## Approach

Adopt **Option C-2** (per exploration recommendation): a project-level `[brief]` table holds prose; structured data is pulled from already-resolved recipe configs; no recipe schema changes. The renderer stays simple Python with a fixed section order, reading `[brief]` + a `--resolved-config` JSON blob (bindings + merged configs) that `sync.sh` pre-computes — keeping catalog resolution out of the renderer. Migration lands the `[brief]` population AND marker removal in the SAME commit to avoid a sync overwriting `AGENTS.md` between steps.

### Alternatives considered & rejected
- **C-1 per-recipe fragments**: needs template files + interpolation + `recipe.toml` schema changes; fragile section ordering. High effort, not needed for this repo's problem.
- **C-3 single brief-template file**: another file/format + interpolation engine; not meaningfully simpler than C-2.
- **C-4 keep marker (no-op)**: leaves duplication and non-idempotent sync unresolved.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `lib/_internal/agents-render.py` | Modified | Emit rich brief; accept `--resolved-config <json>` |
| `lib/sync.sh` | Modified | Pre-compute bindings/config JSON; pass `--resolved-config` |
| `ai-specs/ai-specs.toml` | Modified | Add `[brief]` table |
| `docs/ai-specs-toml.md` | Modified | Document `[brief]` table |
| `tests/test_sync_pipeline.py` | Modified | Golden/needle + idempotency tests |
| `AGENTS.md` / `CLAUDE.md` | Modified | Remove runtime-brief marker (same commit as `[brief]`) |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Renderer needing catalog access | Med | Pass `--resolved-config` JSON from sync.sh (mirrors `--recipe-mcp`); no catalog dep |
| Prose in TOML awkward for long text | Med | String arrays for bullets; single multi-line string for intro; document pattern |
| Migration non-atomic → sync clobbers AGENTS.md | High | Land `[brief]` population + marker removal in the SAME commit |
| Scope creep into C-1 fragments | Med | Hard boundary: project-level `[brief]` only this change |

## Rollback Plan

Restore the `<!-- ai-specs:runtime-brief -->` marker in `AGENTS.md`/`CLAUDE.md`; the still-present `--preserve-if-runtime-brief` flag then re-protects the hand-maintained brief. Optionally revert the `[brief]` table and renderer commit. No data loss — the marker is the permanent escape hatch.

## Dependencies

- Exploration `openspec/changes/option-c-runtime-brief/explore.md` (complete).
- Existing recipe resolution helpers (`recipe-materialize.py`: `resolve_bindings`, `merge_config`) reused by `sync.sh` to build the JSON.

## Success Criteria

- [ ] `agents-render.py` run on this repo's `ai-specs.toml` produces an `AGENTS.md` containing all key needles (board_id, integration_branch, test_command, vault_scope) and the prose sections.
- [ ] Generated `AGENTS.md` ≈ the current hand-maintained brief; the marker is removed and sync is idempotent (second sync produces no diff).
- [ ] Golden/needle + idempotency tests pass; `./tests/validate.sh` green.
- [ ] `docs/ai-specs-toml.md` documents the `[brief]` table.
