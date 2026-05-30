# Design: Option C — Generate the Rich Runtime Brief

## Technical Approach

Adopt **C-2**: a project-level `[brief]` table in `ai-specs.toml` supplies prose; structured runtime values (board_id, integration_branch, test_command, vault_scope) and capability→provider names arrive as a pre-computed `--resolved-config <json>` blob written by `sync.sh`, mirroring `--recipe-mcp`. `agents-render.py` composes a fixed-order brief purely in Python (no template files, no interpolation engine). Catalog resolution stays out of the renderer.

## Architecture Decisions

| # | Decision | Choice | Rejected | Rationale |
|---|----------|--------|----------|-----------|
| 1 | Prose location | Project `[brief]` table; arrays for bullets, multi-line string for intro, sub-table for mcp_descriptions | per-recipe fragments (C-1), template file (C-3) | Smallest schema surface; no recipe.toml/interpolation changes |
| 2 | Config delivery | `sync.sh` writes a **temp JSON file**, passes `--resolved-config <path>` | inline arg (quoting hell), renderer reads catalog | Mirrors proven `--recipe-mcp` temp-file pattern; renderer stays catalog-free |
| 3 | Composition | Compute-in-Python, fixed section order, one helper fn per section | template + interpolation | MCP table already built this way; zero new deps |
| 4 | Binding lookup | Read `bindings` map from JSON: `tracker`/`canonical-store`/`vcs` → recipe_id → that recipe's merged config | renderer resolves bindings | Reuses `resolve_bindings`/`merge_config` already run in sync |
| 5 | Migration | Populate `[brief]` + remove marker in **one commit** after scratch-diff confirms ≈ | separate commits | Avoids sync clobbering AGENTS.md mid-migration |
| 6 | Degradation | JSON absent/empty → render prose + identity + MCP only; skip structured sections | hard-fail | Standalone `agents-render.py` (no sync) still works |

## Data Flow

    ai-specs.toml ─┐
                   ├─ recipe-materialize: resolve_bindings + merge_config
    catalog ───────┘            │
                                ▼
              sync.sh writes resolved-config.json (temp)
                                │  --resolved-config <path>
                                ▼
    [brief] + [project] + [mcp] ─→ agents-render._render_lines() ─→ AGENTS.md
                                     (per-section helpers)

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `lib/_internal/agents-render.py` | Modify | Add `--resolved-config`; restructure `_render_lines` into section helpers |
| `lib/_internal/recipe-materialize.py` | Modify | Emit resolved-config JSON (bindings + per-recipe merged config + enabled) via new `--resolved-config-out` |
| `lib/sync.sh` | Modify | mktemp resolved-config; pass to materialize-out and agents-render; cleanup |
| `ai-specs/ai-specs.toml` | Modify | Add `[brief]` table |
| `AGENTS.md` / `CLAUDE.md` | Modify | Remove marker (same commit as `[brief]`) |
| `docs/ai-specs-toml.md` | Modify | Document `[brief]` table |
| `tests/test_sync_pipeline.py` | Modify | Golden/needle + idempotency tests |

## Interfaces / Contracts

### `[brief]` schema (example, populated from this repo)

```toml
[brief]
intro = """
Canonical runtime context for agents: identity, MCPs, context sources,
safety rules, and workflow conventions. Work state lives in Trello and Engram.
"""
purpose = "per-project AI harness for agent configuration, MCPs, recipes, memory, and tracker integration."
runtime_flow = [
  "A session works on one explicit user request or Trello card.",
  "Artifact and implementation phases run in a dedicated worktree when they write files.",
]
context_sources = ["Trello is the source of truth for work state and dependencies."]
conflict_policy = ["Current explicit human instruction controls immediate scope unless it conflicts with safety/secrets."]
workflow_rules = ["Do not merge or push to the integration branch without explicit human instruction."]

[brief.mcp_descriptions]
trello = "project tracking through the ai-specs-cli Roadmap board."
engram = "operational/session memory (global MCP)."
vault-ai-specs = "canonical project notes in the Obsidian vault."
```

All keys optional; renderer skips missing sections. `runtime_flow`/`context_sources`/`conflict_policy`/`workflow_rules` = arrays of strings → bullet lists. `intro` = multi-line string → blockquote. `mcp_descriptions` = table keyed by server name → appended to each MCP entry.

### `--resolved-config` JSON shape

```json
{
  "bindings": { "tracker": "trello-mcp-workflow", "canonical-store": "vault-canonical-store", "vcs": "git-pr-flow" },
  "recipes": {
    "worktree-flow":       { "integration_branch": "development" },
    "git-pr-flow":         { "provider": "github", "base_branch": "development" },
    "tdd-flow":            { "test_command": "./tests/validate.sh" },
    "trello-mcp-workflow": { "board_id": "69ec097f13e2d38ecd89a557" },
    "vault-canonical-store": { "vault_scope": "nnodes/proyectos/ai-specs" }
  },
  "enabled": ["worktree-flow","git-pr-flow","session-context","tdd-flow","trello-mcp-workflow","vault-canonical-store"]
}
```

Renderer maps: `recipes[bindings.tracker].board_id` → Trello Tracking; `recipes[bindings.vcs].provider/base_branch` → Runtime Flow VCS bullet; `recipes["tdd-flow"].test_command` → Useful Commands; `recipes[bindings.canonical-store].vault_scope` → vault scope. `vcs` binds the capability name git-pr-flow declares (confirm at apply time).

### Section order (`_render_lines`)
1. `# {name} Runtime Brief` 2. intro blockquote 3. `## Project` (name, manifest, purpose, enabled, integration_branch) 4. `## Runtime MCPs` (existing table + descriptions + secrets rule) 5. `## Runtime Flow` 6. `## Trello Tracking` 7. `## Context Sources` 8. `## Conflict Policy` 9. `## Workflow Rules` 10. `## Useful Commands` (test_command + derived validate).

## Testing Strategy (TDD — tests first; runner `./tests/validate.sh`)

| Layer | What | Approach |
|-------|------|----------|
| Integration | Needles: board_id, integration_branch, test_command, vault_scope, MCP descriptions present in generated AGENTS.md | New `test_sync_renders_rich_brief_from_manifest`, mirroring `test_sync_redacts_literal_mcp_secrets_in_agents_md` (build manifest with `[brief]` + recipes, run sync, assertIn needles) |
| Integration | Idempotency | `test_sync_rich_brief_identical_on_second_run` (read_bytes twice, assertEqual) |
| Unit/Integration | Graceful degradation | Run `agents-render.py` standalone with no `--resolved-config`: asserts identity+MCP+prose render, no crash |
| Smoke | `./tests/validate.sh` green | Final gate |

Backward compat: `test_sync_preserves_runtime_brief_marker_in_agents_md` must still pass (`--preserve-if-runtime-brief` kept). Subrepo fixtures (`fixture-sync` needle) unaffected — heading format additive. Existing thin-format tests with no `[brief]` keep emitting identity+MCP only.

## Migration / Rollout

1. Add `[brief]` to manifest (marker still present). 2. Implement renderer + sync wiring. 3. `agents-render.py <toml> /tmp/AGENTS.scratch.md --resolved-config <generated>`; diff vs current AGENTS.md; iterate `[brief]` until ≈. 4. In ONE commit: remove `<!-- ai-specs:runtime-brief -->` from AGENTS.md/CLAUDE.md. Non-atomicity risk mitigated: marker protects the file until the same commit removes it, so no intermediate sync can clobber. Rollback: restore marker — `--preserve-if-runtime-brief` re-protects.

## Open Questions

- [ ] Exact capability id git-pr-flow declares for VCS (`vcs` vs `version-control`) — confirm by reading its recipe.toml at apply time.
- [ ] Whether `recipe-materialize` should expose the JSON or `sync.sh` calls a tiny helper — lean: extend materialize with `--resolved-config-out`.
