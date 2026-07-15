# Design: cli-bound-recipes

> Supersedes `sdd/recipe-update-flow/design` (#1249). Pins were ack ceremony; origin moves off-project.

## Technical Approach

Bind enabled recipes to the **installed CLI catalog** (no per-recipe pin). Stage origin (`.recipe`, `.deps`, managed commands, flatten) under `$AI_SPECS_HOME/cache/projects/<key>/`. Keep in-project: `ai-specs.toml`, `ai-specs/skills/`, `ai-specs/recipes/` (docs, hooks, templates, skill overrides). Fan-out targets unchanged.

## Architecture Decisions

| Decision | Options | Choice |
|----------|---------|--------|
| Cache root | AI_SPECS_HOME vs XDG | **`$AI_SPECS_HOME/cache/projects/<key>/`** (locked) |
| Cache key | full hash vs short+name | **`sha256(realpath(project_root))[:12]` + `-` + sanitized basename** |
| Sidecar | none vs metadata | **`meta.toml`**: `project_root`, `created_at` |
| Flatten staging | leave in-project vs cache | **Cache `resolved-skills/`** (locked) |
| Legacy `version` | strip vs ignore | **Ignore + WARN**; optional strip on next toml write |
| Leftover `.recipe`/`.deps` | warn vs rm | **Sync `rm -rf` both trees** (locked) |
| Skill overrides path | stay under cache `.recipe` vs in-project | **`ai-specs/recipes/<id>/overrides/`**; migrate from leftover `.recipe/.../overrides` before rm |
| Managed commands | in-project vs cache | **Cache `commands/`**; fan-out merges with `ai-specs/commands/` (**local wins**) |
| Pin check | keep fail-closed vs drop | **Delete `validate_version_pin`** |
| Delivery | monolithic vs chain | **4 chained PRs** (below) |

## Cache layout + helpers

```
$AI_SPECS_HOME/cache/projects/<key>/
  meta.toml
  .recipe/<recipe-id>/skills/...
  .deps/<dep-id>/skills/...
  commands/<cmd-id>.md
  resolved-skills/<skill-id>/
```

New `lib/_internal/project-cache.py` (stdlib; follow `util.ai_specs_home`):

```python
def cache_key(project_root: Path) -> str: ...
def cache_root(project_root: Path, cli_home: Path | None = None) -> Path: ...
def ensure_cache(project_root: Path) -> Path:  # mkdir + write meta.toml
def recipe_skills_root(project_root) -> Path
def deps_skills_root(project_root) -> Path
def commands_dir(project_root) -> Path
def resolved_skills_dir(project_root) -> Path
def remove_legacy_origin(project_root) -> None  # rm ai-specs/.recipe + .deps
```

All materialize / vendor / skill-resolution / flatten / orphan paths go through these helpers. No hardcoded `ai-specs/.recipe` for origin.

## Data Flow

```
ai-specs sync
  ├─ WARN legacy version= keys (non-blocking)
  ├─ ensure_cache(project)
  ├─ remove_legacy_origin (+ migrate overrides → ai-specs/recipes/<id>/overrides)
  ├─ materialize → cache .recipe/.deps/commands; in-project docs/hooks/templates
  ├─ clean_orphans(cache)
  ├─ flatten → cache/resolved-skills
  └─ sync-agent fan-out
        skills ← cache/resolved-skills
        commands ← merge(cache/commands, ai-specs/commands)  # local wins
        agent targets unchanged
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `lib/_internal/project-cache.py` | Create | Key, root, path helpers, legacy cleanup |
| `lib/_internal/recipe-materialize.py` | Modify | Drop pin; cache staging; legacy WARN; leftover rm; orphan on cache |
| `lib/_internal/skill-resolution.py` | Modify | Scan cache `.recipe`/`.deps`; overrides under `ai-specs/recipes/<id>/overrides/` |
| `lib/_internal/vendor-skills.py` | Modify | `sync_dep_target` → cache `.deps` |
| `lib/_internal/flatten-resolved-skills.py` | Modify | Default dest = cache resolved-skills |
| `lib/sync-agent.sh` | Modify | Flatten + command merge from cache |
| `lib/_internal/toml-read.py` | Modify | `version` optional; expose for WARN |
| `lib/_internal/recipe-add.py`, `recipe-init.py`, `recipe-config-write.py`, `init_tui.py` | Modify | Stop writing `version=` |
| `lib/_internal/recipe-list.py`, `doctor.py`, `hub.py` | Modify | Catalog version info-only; no outdated-pin UX |
| `lib/init.sh`, `gitignore-render.py` | Modify | Stop mkdir/ignore in-project `.recipe`/`.deps`; drop `.internal/resolved-skills` ignore once moved |
| `openspec/specs/recipe-manifest-contract/spec.md` | Modify | enabled-only; legacy WARN |
| `openspec/specs/external-dirs-layout/spec.md` | Rewrite | Cache origin layout |
| `openspec/specs/recipe-overrides-runtime/spec.md` | Modify | Overrides path → `ai-specs/recipes/<id>/overrides/` |
| `openspec/specs/recipe-cli/spec.md` | Modify | list info; no update/pin |
| Docs + `templates/ai-specs.toml.tmpl` | Modify | Cache model; #104 WARN/note |
| Tests (many fixtures with `version=`) | Modify | Broad path/contract migration |

**Not created:** `recipe-update.py` / pin-bump path (abandoned).

## Interfaces

- Manifest `[recipes.<id>]`: require `enabled` only; `config` optional; unknown `version` → WARN, ignore.
- Catalog `recipe.toml` `version` remains catalog metadata (list/info).
- Lock file stays in-project; hashes may reference cache file contents.
- Command merge helper (Python preferred, callable from sync-agent): copy cache cmds then overlay `ai-specs/commands/`.

## Failure Modes

| Case | Behavior |
|------|----------|
| Cache not writable | Fail sync with path + errno |
| `realpath` fails | Fail closed |
| Legacy `version` present | WARN; sync continues |
| Leftover rm permission error | WARN; continue |
| Command id collision | Local hand-authored wins; optional WARN |
| Worktree / rename | New cache key; sync rebuilds (sidecar records old root) |
| Unknown recipe id | Fail as today |

## Testing Strategy

| Layer | What | Approach |
|-------|------|----------|
| Unit | `cache_key` stability, sidecar, path helpers | temp dirs + `AI_SPECS_HOME` |
| Unit | no pin check; legacy WARN | materialize fixtures |
| Unit | skills resolve from cache; overrides new path | extend `test_external_dirs` |
| Unit | command merge precedence | new/extend sync tests |
| Unit | leftover `.recipe`/`.deps` deleted | materialize/sync |
| Unit | add/init omit version | recipe-add/config-write |
| Unit | list shows catalog version, not pin status | recipe-list |
| Integration | full sync → fan-out skills/commands | `test_sync_pipeline` |
| Specs | manifest + external-dirs + overrides | openspec scenarios |

RED→GREEN via `tdd-flow`; `./tests/validate.sh` before commit.

## Migration / Rollout

1. Sync WARNs on `version=`; does not rewrite toml unless a write path already runs.
2. Sync migrates `.recipe/<id>/overrides` → `ai-specs/recipes/<id>/overrides` if destination absent, then deletes in-project `.recipe` and `.deps`.
3. Init stops creating those dirs; gitignore-render stops listing them (keep `recipes/` ignore as today).
4. Cache disposable; uninstall/wipe OK.
5. Rollback = revert PRs; re-adding pins only if mid-upgrade consumers need old CLI.

## Chained PR slices

1. **Manifest unpin** — drop `validate_version_pin`; add/init/toml-read/docs/tests; legacy WARN; list catalog-info only.
2. **Cache module + skill origin** — `project-cache.py`; materialize/vendor/skill-resolution/orphan → cache; leftover rm + override migrate.
3. **Commands + flatten + sync-agent** — cache commands + merge; resolved-skills in cache; init/gitignore cleanup.
4. **Polish** — doctor/hub copy; #104 WARN/docs; external-dirs + overrides spec rewrite; fixture sweep.

`Decision needed before apply: Yes` (slice boundaries / delivery_strategy)
`Chained PRs recommended: Yes`
`400-line budget risk: High` monolithic; **Medium** per slice

## Open Questions

None blocking. Non-blocking for tasks: exact WARN string; key suffix sanitization charset; whether gitignore keeps temporary `.recipe/` patterns one release (prefer remove in slice 3).

## Risks

| Risk | Mitigation |
|------|------------|
| Wide test churn | Chain; fixture helper for cache paths |
| Override loss on leftover rm | Migrate before delete |
| Relative hook paths | Keep hooks under `ai-specs/recipes/` |
| Command clobber | Local-wins merge |
| #104 false confidence | WARN/note only |
