# Exploration: cli-bound-recipes

> Supersedes `sdd/recipe-update-flow/explore` (#1247) and the pin-bump `recipe update` direction. Locked product north star confirmed 2026-07-14. Prior Q answers (#1252) that still apply: #104 WARN/note only; no auto-sync flag required.

### Current State

**Version pins (ack ceremony)**
Consumer manifests declare `[recipes.<id>]` with required `enabled` + exact `version` (`openspec/specs/recipe-manifest-contract/spec.md`). Sync calls `validate_version_pin` in `lib/_internal/recipe-materialize.py` — strict equality vs catalog `recipe.toml`; fail-closed on mismatch. Catalog ships **one** version per recipe per CLI install, so after `ai-specs upgrade` every enabled pin is stale until hand-edited. Pins do not select among historical recipe versions.

**In-project origin staging (noise)**
Today materialize writes recipe/dep **origin** into the consumer tree:
- Bundled skills → `ai-specs/.recipe/<recipe-id>/skills/<skill-id>/`
- Dep skills → `ai-specs/.deps/<dep-id>/skills/...` (`vendor-skills.py`)
- Recipe commands → `ai-specs/commands/<id>.md` (mixed with hand-authored commands; dir mostly gitignored via `commands/.gitignore`)
- Flatten staging → `ai-specs/.internal/resolved-skills/` then fan-out to agent targets
- Docs/templates/hooks/bin → often under `ai-specs/recipes/<id>/...` (project-relative paths for hooks)

`ai-specs/.gitignore` (via `gitignore-render.py`) ignores `.recipe/`, `.deps/`, `recipes/`, `.internal/`, `.resolved-skills/`. Init also `mkdir`s `.recipe`/`.deps` under `ai-specs/` (`lib/init.sh`). Spec `external-dirs-layout` still documents root `.recipe/`/`.deps/` — **already stale vs code** (code uses `ai-specs/.…`).

**Resolution + fan-out**
`skill-resolution.py` scans local `ai-specs/skills/` > `ai-specs/.recipe/` > `ai-specs/.deps/`. `sync-agent.sh` flattens resolved skills and fans out skills/commands/MCP to agent dirs (`.cursor/skills`, etc.). Fan-out contract is **unchanged** by this change.

**User surface that stays**
- `ai-specs.toml` — agents, MCP, brief, bindings, recipe `enabled` + `[recipes.*.config]`
- `ai-specs/skills/` — local user skills
- `ai-specs/recipes/` — docs + user overrides (+ today also managed hooks/bin/templates with `not_exists`)

**Prior direction (superseded)**
`recipe-update-flow` recommended guided pin-bump (`recipe update`) + list/doctor outdated visibility. User later confirmed pins are ceremony and preferred CLI-bound catalog + off-project origin cache (#1252, #1253).

### Affected Areas

| Area | Why |
|------|-----|
| `lib/_internal/recipe-materialize.py` | Drop `validate_version_pin`; retarget skill/command staging to cache; orphan cleanup; keep docs/hooks/templates policy |
| `lib/_internal/skill-resolution.py` | Scan cache `.recipe`/`.deps` instead of (or in addition to migrating from) project paths |
| `lib/_internal/vendor-skills.py` | Dep clone target → cache |
| `lib/_internal/flatten-resolved-skills.py` + `lib/sync-agent.sh` | Resolve from cache; command source merge (cache recipe cmds + in-project hand-authored) |
| `lib/_internal/gitignore-render.py` + `lib/init.sh` | Stop creating/ignoring in-project `.recipe`/`.deps`; optional cleanup notes |
| `lib/_internal/recipe-add.py`, `recipe-init.py`, `recipe-config-write.py` | Stop writing/requiring `version=` |
| `lib/_internal/recipe-list.py`, `doctor.py`, `hub.py` | No outdated-pin UX; show catalog/CLI-bound version as info only |
| `lib/_internal/lock.py` | Hash paths may point at cache; lock can stay in-project as sync metadata |
| New helper (likely) `lib/_internal/project-cache.py` | Cache root + per-project key |
| Specs: `recipe-manifest-contract`, `external-dirs-layout`, skill-source-precedence, recipe-cli | Rewrite pin + layout contracts |
| Docs: `README.md`, `docs/recipe-schema.md`, `docs/recipes-catalog.md`, `docs/ai/troubleshooting.md`, `templates/ai-specs.toml.tmpl` | Remove pin ceremony; document cache model |
| Tests: `test_recipe_add.py`, `test_recipe_materialize.py`, `test_external_dirs.py`, `test_sync_pipeline.py`, many fixtures with `version =` | Broad migration |
| Migration | Ignore/strip stale `version` keys; delete leftover `ai-specs/.recipe`/`.deps` on sync |

**Out of scope (confirm):** fan-out redesign; keeping old recipe content after CLI upgrade; full #104 managed-template refresh (WARN/note only).

### Approaches

1. **Keep pins + guided `recipe update`** (prior explore #1247)
   - Pros: Smaller delta; preserves exact-pin narrative; mirrors `ai-specs upgrade` UX.
   - Cons: Pins remain ack ceremony (no multi-version catalog); sync still fail-closes after CLI upgrade until bump; does **not** remove in-project origin noise/worktree clutter; false sense origin lives in project.
   - Effort: Medium
   - **Reject** — superseded by locked north star.

2. **CLI-bound recipes + off-project origin cache (LOCKED)**
   - Drop per-recipe `version` from user toml. Sync always materializes **current catalog** shipped with installed CLI.
   - Stage `.recipe`, `.deps`, and managed recipe-command origin under a CLI-hidden per-project cache.
   - Keep in-project: toml (enabled+config), `ai-specs/skills/`, `ai-specs/recipes/` (docs+overrides; hooks/bin/templates as today where project-relative paths matter).
   - Fan-out targets unchanged.
   - Pros: Matches runtime reality; removes pin bump after upgrade; cleans project/worktree noise; clearer mental model (catalog↔CLI, config↔project).
   - Cons: Larger blast radius (layout + manifest contract + many tests); cache keying/portability design needed; migration of existing manifests; AGENTS.md links to recipe skills already point at resolved paths — must remain correct after sync.
   - Effort: High (likely chained PRs)
   - **Recommend.**

3. **CLI-bound pins dropped but staging stays in-project**
   - Pros: Fixes upgrade fail-close only; smaller than full cache move.
   - Cons: Leaves gitignored origin trees and clutter; incomplete vs agreed model.
   - Effort: Medium
   - **Reject** as final; only acceptable as an intermediate PR slice if delivery strategy requires it.

#### Cache placement (sub-options for Approach 2)

| Sub-option | Pros | Cons |
|------------|------|------|
| **A. `$AI_SPECS_HOME/cache/projects/<key>/`** | Tied to CLI install; easy to wipe on uninstall; no XDG dependency | Dev checkouts sharing AI_SPECS_HOME share cache root (ok if keyed per project) |
| **B. XDG `~/.cache/ai-specs/projects/<key>/`** | Standard OS cache semantics | Must define Windows/fallback; separate from AI_SPECS_HOME lifecycle |

**Tentative cache key:** `sha256(realpath(project_root))` short hex + optional project name suffix for humans. Record `project_root` in a small cache sidecar for doctor/debug.

**Tentative stay-in-project under `ai-specs/recipes/`:** docs, `not_exists` templates, hook scripts, bin helpers (need stable project-relative paths for agent/hook wiring). Only skill origin + managed command **staging** + optionally `.internal/resolved-skills` move off-project.

### Recommendation

**Approach 2 — CLI-bound catalog + off-project origin cache**, with:

1. Manifest: `[recipes.<id>]` requires only `enabled`; optional ignore/warn on legacy `version` then strip in migration note; `recipe add` no longer writes version.
2. Sync: always use catalog version from installed CLI; remove fail-closed pin check.
3. Cache: prefer **sub-option A** (`$AI_SPECS_HOME/cache/projects/<key>/{.recipe,.deps,commands,…}`) unless propose finds strong XDG preference; single resolver module used by materialize, vendor, skill-resolution, flatten, orphan cleanup.
4. Commands: recipe-managed command files stage in cache; hand-authored remain in `ai-specs/commands/`; fan-out merges both (precedence: local hand-authored over recipe, matching conflict spec spirit).
5. Fan-out: no contract change — still writes agent targets as today.
6. Doctor/list/hub: drop outdated-pin; optionally show catalog recipe versions as informational; sync/doctor may note CLI upgrade implies recipe refresh on next sync.
7. #104: document/WARN that `not_exists` templates do not refresh — separate follow-up.
8. Delivery: expect chained PRs (manifest pin removal → cache layout → cleanup/docs); abandon `recipe-update-flow` proposal path.

### Risks

- **Wide test/spec churn** around `version =` fixtures and layout paths; 400-line PR budget → chain.
- **Cache identity**: path renames / multiple worktrees = multiple cache dirs (acceptable; sync rebuilds). Shared worktrees need realpath consistency.
- **Stale in-project trees**: leftover `ai-specs/.recipe`/`.deps` confuse users if not cleaned or warned.
- **Hook/template boundary**: moving too much of `ai-specs/recipes/` breaks project-relative hook paths — keep those in-project.
- **Command merge**: must not clobber hand-authored commands when staging moves.
- **Template false confidence (#104)** remains after CLI upgrade+sync.
- **Spec debt**: `external-dirs-layout` already wrong; this change must rewrite it, not patch root vs ai-specs/.

### Open Questions (for propose)

1. Cache root: `AI_SPECS_HOME/cache` vs XDG?
2. Move `.internal/resolved-skills` off-project in same change, or leave as thin in-project staging?
3. Migration: silent ignore of `version` keys vs one-time strip on sync/add with message?
4. On sync, aggressively `rm -rf` leftover project `ai-specs/.recipe` and `.deps`?
5. Should `recipe list` still display catalog `version` (informational) after pins are gone?

### Ready for Proposal

Yes. Orchestrator should run **sdd-propose** for `cli-bound-recipes` against this locked model; treat `recipe-update-flow` artifacts as historical/superseded.

### Slice forecast (for tasks later)

- Slice 1: Manifest contract — drop pin validation + recipe add/init/docs/tests
- Slice 2: Cache layout + materialize/vendor/skill-resolution/orphan cleanup
- Slice 3: Commands staging merge + sync-agent + gitignore/init cleanup + migration cleanup
- Slice 4: Doctor/list/hub/docs polish + #104 WARN notes

Decision needed before apply: Yes (cache root + migration aggressiveness)
Chained PRs recommended: Yes
400-line budget risk: High if monolithic; Medium per slice
