# Design: motor-restructurar-dirs-externos

## Context

Today `ai-specs/skills/` is a monolithic directory that mixes three distinct provenances:

1. **Local project skills** — created and maintained by the project team (e.g., `context-precedence`, `vault-context`).
2. **Recipe-bundled skills** — shipped as part of a recipe from the ai-specs-cli catalog (e.g., `skill-creator`, `skill-sync`).
3. **Vendored dependency skills** — cloned from external git repos via `[[deps]]` in `ai-specs.toml`.

This mixing creates several problems:
- Projects must commit external code or treat vendored skills as generated artifacts that are re-cloned on every sync.
- There is no way to distinguish project-owned skills from externally-provided ones.
- Recipe-bundled skills cannot be customized without modifying files that may be overwritten on the next sync.
- Conflict resolution between recipes and local skills is implicit and poorly defined.

The current `ai-specs sync` pipeline vendors all skills into `ai-specs/skills/`, and `sync-agent` reads skills from that single directory. This design document explains how to restructure the layout into three isolated directories while preserving backward compatibility for the local-skills workflow.

## Goals / Non-Goals

**Goals:**
- Isolate external skill sources (recipe-bundled and vendored deps) from local project skills.
- Define deterministic precedence rules for skill resolution across the three sources.
- Allow recipe-bundled skills to be customized at runtime via overrides without modifying vendored files.
- Ensure `ai-specs/skills/` remains exclusively for local, committable project skills.
- Update `ai-specs init`, `sync`, and `sync-agent` to create, populate, and read from the new layout.
- Maintain idempotency: re-running `sync` with no changes must be a no-op.

**Non-Goals:**
- Changing the TOML manifest schema (ai-specs.toml) or the recipe catalog format.
- Supporting file-level merging of skills across sources (entire-directory precedence only).
- Changing how commands, templates, docs, or MCP presets are materialized (they remain in `ai-specs/`).
- Removing the existing bundled-skills behavior of `ai-specs init` (`skill-creator` and `skill-sync` still seed the local tree).
- Multi-target subrepo behavior beyond mirroring the resolved skill tree.

## Decisions

### Decision 1: Three-directory layout

**Choice:** Introduce `.recipe/` and `.deps/` at project root, keep `ai-specs/skills/` for locals.

**Rationale:**
- Symmetric to package-manager patterns (`node_modules/`, `.venv/`, `vendor/`) where external dependencies are isolated from source code.
- Dot-prefixed directories are conventionally hidden and easy to gitignore.
- Placing them at project root rather than inside `ai-specs/` makes the boundary between "source of truth" (ai-specs/) and "restored artifacts" (external dirs) visually obvious.

**Alternatives considered:**
- `ai-specs/external/skills/` and `ai-specs/external/deps/` — rejected because it puts generated content inside the manifest directory, blurring the source-of-truth boundary.
- `ai-specs/.recipe/` and `ai-specs/.deps/` — rejected for the same reason; also harder to explain to users why some things in `ai-specs/` are committed and others are not.

### Decision 2: Source-level (not file-level) precedence

**Choice:** When a skill ID is resolved, the entire skill directory comes from the highest-precedence source that contains it. No file merging occurs across sources.

**Rationale:**
- Simpler to implement, test, and reason about.
- Avoids subtle bugs where a missing asset file in a local override silently falls back to a recipe-provided version, creating unexpected behavior.
- Aligns with how most package managers resolve dependencies.

**Precedence order:**
1. `ai-specs/skills/{id}/` — local (highest)
2. `.recipe/{recipe-id}/skills/{id}/` — recipe-bundled (middle)
3. `.deps/{dep-id}/skills/{id}/` — vendored dependency (lowest)

**Alternatives considered:**
- File-level merging (e.g., local `SKILL.md` + recipe `assets/`) — rejected because it creates fragile composite skills that are hard to debug.

### Decision 3: Recipe-scoped overrides

**Choice:** Each recipe may provide `.recipe/{recipe-id}/overrides/config.toml` and `overrides/templates/`. These override bundled skill defaults at runtime but only for skills within that recipe.

**Rationale:**
- Gives projects a supported customization path without forking recipe code.
- Keeps overrides isolated per recipe, preventing cross-contamination.
- Overrides are gitignored, so teams can share the base recipe while allowing individual developers or environments to customize.

**Alternatives considered:**
- Global override directory (e.g., `ai-specs/overrides/`) — rejected because it breaks recipe encapsulation and makes it unclear which override applies to which recipe.

### Decision 4: `sync-agent` performs multi-source scanning

**Choice:** `sync-agent` (and the Python helpers it calls) scans all three sources and resolves skill paths using the precedence rules before rendering AGENTS.md or fanning out to agent configs.

**Rationale:**
- `sync-agent` is the consumer of skills; it is the right place to resolve which skill content to use.
- `recipe-materialize.py` and `vendor-skills.py` remain responsible for *writing* skills into `.recipe/` and `.deps/` respectively, not for deciding which one wins.
- This separation of concerns keeps materialization simple and deterministic.

**Sequence diagram — sync-agent skill resolution:**

```
sync-agent.sh
  │
  ├─► agents-md-render.py ──► collect_skills()
  │                              │
  │                              ├─► scan ai-specs/skills/*          (local)
  │                              ├─► scan .recipe/*/skills/*         (recipe)
  │                              ├─► scan .deps/*/skills/*           (dep)
  │                              │
  │                              ▼
  │                         resolve_by_precedence(skill_id)
  │                              │
  │                              ├─► if exists in local  → use local
  │                              ├─► elif exists in recipe → use first recipe
  │                              ├─► elif exists in dep    → use first dep
  │                              └─► else → error (missing skill)
  │
  ├─► platform_get(agent) → render per-agent configs
  │      │
  │      ├─► symlink/copy skills from resolved paths
  │      └─► symlink AGENTS.md, render MCP, copy commands
  │
  ▼
✓ sync-agent complete
```

### Decision 5: Local skills silently override all external sources

**Choice:** If a skill exists in `ai-specs/skills/`, it wins over recipe and dep versions without warning or error.

**Rationale:**
- Projects should be able to fork/vendor a skill locally by simply copying it into `ai-specs/skills/`.
- Warnings would be noisy for intentional overrides.
- Recipe-recipe conflicts (same primitive ID declared by two recipes) still fail hard, preserving the existing conflict contract.

**Alternatives considered:**
- Emit a warning for local overrides — rejected because it penalizes the supported customization pattern.

### Decision 6: First-seen ordering within a precedence tier

**Choice:** When the same skill ID appears in multiple recipes (or multiple deps), the first one in declaration order wins, with a warning.

**Rationale:**
- Declaration order in `ai-specs.toml` is already the user's explicit priority.
- A warning is appropriate because this is usually unintentional (two recipes bundling the same common utility skill).
- Errors are reserved for recipe-recipe primitive conflicts at the catalog level, not for runtime resolution ambiguities.

### Decision 7: Gitignore rules for external directories

**Choice:** `.recipe/`, `.deps/`, and `.recipe/*/overrides/` are all added to the root `.gitignore` during `ai-specs init`.

**Rationale:**
- These directories contain restored/generated artifacts that should never be committed.
- The existing `ai-specs/.gitignore` (rendered by `gitignore-render.py`) continues to ignore vendored dep skills, but now those skills live in `.deps/` rather than `ai-specs/skills/`.
- Root `.gitignore` is the right place because `.recipe/` and `.deps/` are at project root.

## Risks / Trade-offs

| Risk | Mitigation |
|------|------------|
| **Breaking existing projects** that rely on vendored skills being in `ai-specs/skills/` | `vendor-skills.py` and `recipe-materialize.py` will write to the new paths. Existing `ai-specs/skills/` contents are left untouched; only newly synced skills go to the new locations. A one-time `sync` after upgrading the CLI will move skills implicitly. |
| **AGENTS.md links break** if they still point to `ai-specs/skills/` for recipe/dep skills | `agents-md-render.py` already renders relative links based on the skills directory it scans. With multi-source resolution, links will point to the *resolved* path. However, since `.recipe/` and `.deps/` are gitignored, links to them in committed AGENTS.md may be dead on a fresh clone. **Decision:** AGENTS.md links for external skills will still reference the resolved source path; this is acceptable because AGENTS.md is regenerated on every `sync`. |
| **Performance** of scanning three directory trees instead of one | Measured risk is low: typical projects have <50 skills total. If needed, `agents-md-render.py` can cache the scan result. |
| **Override files may conflict** if two recipes bundle the same skill and both have overrides | Impossible by design: overrides are scoped to their parent recipe, and a skill can only be resolved from one source. |
| **Users manually editing files in `.recipe/`** | Same risk as editing `node_modules/`. Documentation and `.gitignore` conventions make it clear these are generated. No additional enforcement needed. |

## Migration Plan

1. **Update `lib/init.sh`**
   - Add `mkdir -p "$TARGET_PATH/.recipe" "$TARGET_PATH/.deps"`.
   - Append `.recipe/`, `.deps/`, and `.recipe/*/overrides/` to the root `.gitignore` block.

2. **Update `lib/_internal/vendor-skills.py`**
   - Change `target_dir` from `project_root / "ai-specs" / "skills" / dep_id` to `project_root / ".deps" / dep_id / "skills" / skill_subpath`.
   - Preserve shallow-clone behavior and YAML frontmatter replacement.

3. **Update `lib/_internal/recipe-materialize.py`**
   - Change `materialize_bundled_skill` destination to `project_root / ".recipe" / recipe_id / "skills" / skill_id`.
   - Change `materialize_dep_skill` to use `.deps/{dep-id}/skills/{skill-id}/` instead of `ai-specs/skills/`.
   - Keep commands, templates, docs, and MCP presets in `ai-specs/`.

4. **Update `lib/_internal/gitignore-render.py`**
   - Add `.recipe/` and `.deps/` to the root `.gitignore` template.
   - Continue to ignore vendored skills, but now via the root `.gitignore` entry for `.deps/`.

5. **Update `lib/sync-agent.sh` and `lib/_internal/agents-md-render.py`**
   - Implement `collect_skills_multi_source()` that scans all three directories.
   - Apply precedence rules: local > recipe > dep.
   - Warn on same-ID within recipe tier or dep tier.
   - Error if a required skill is missing from all sources.

6. **Add regression tests**
   - Test `init` creates `.recipe/` and `.deps/`.
   - Test `vendor-skills.py` writes to `.deps/`.
   - Test `recipe-materialize.py` writes bundled skills to `.recipe/` and dep skills to `.deps/`.
   - Test `sync-agent` resolves precedence correctly across all three sources.
   - Test override loading from `.recipe/{recipe-id}/overrides/`.

7. **Rollback strategy**
   - Reverting the CLI version restores the old paths. Since `ai-specs/skills/` is untouched for local skills, only recipe/dep skills would need to be re-synced.

## Open Questions

1. **Should `refresh-bundled` preset behavior change?** The existing `refresh-bundled.py` compares bundled skills against a SHA baseline. With bundled skills now in `.recipe/`, should the baseline still track them, or should bundled skills be considered immutable recipe artifacts?
   - **Tentative answer:** Leave `refresh-bundled` as-is for now; bundled skills in `.recipe/` are still version-pinned by the recipe lock. Revisit in a follow-up change if needed.

2. **Should `ai-specs add-dep` validate that the dep does not conflict with a local skill?** Currently `add-dep` only mutates `ai-specs.toml`. With precedence rules, a local skill will always win. Is a warning useful?
   - **Tentative answer:** No warning in `add-dep`; the conflict is resolved deterministically at sync time.

3. **Should `sync-agent --source-root --target` multi-target mode mirror `.recipe/` and `.deps/` into subrepo targets?** Currently it mirrors `ai-specs/skills/` and `ai-specs/commands/`.
   - **Tentative answer:** Yes. Multi-target fan-out should mirror the *resolved* skill tree (after precedence) into the target's `ai-specs/skills/`, so subrepos see a flat, finalized view. The external directories remain a root-workspace concern only.

4. **AGENTS.md links to external skills** — since `.recipe/` and `.deps/` are gitignored, links to skills in those directories will 404 on GitHub. Is this acceptable?
   - **Tentative answer:** Yes. AGENTS.md is regenerated on every `sync` and is primarily consumed locally by the agent. The link is still useful for local navigation.
