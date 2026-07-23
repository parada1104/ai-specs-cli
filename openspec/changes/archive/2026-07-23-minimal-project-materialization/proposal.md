# Minimize the committed project surface: bundled skills, toml-deps, recipe overrides, and the lock

## Problem

0.15.0 introduced "always-on harness CLI literacy" as **CLI-bundled skills**
(`bundled-skills/harness-lifecycle`, `harness-recipes`, `harness-skills-deps`,
plus the pre-existing `skill-creator`, `skill-sync`). These are shipped by the
CLI and are recipe-independent. But the skill model has no tier for that
category, so `refresh-bundled` materializes them **into the project** at
`ai-specs/skills/` and commits them (the `.gitignore` migration even removed
`recipes/`, `.recipe/`, `.deps/`, `.resolved-skills/` from ignore).

This contradicts the model the specs already encode:

- `external-dirs-layout` → **Local skills exclusivity**: *"`ai-specs/skills/`
  SHALL contain only local user skills after sync."*
- `project-recipe-cache` → recipe/dep/command/resolved-skills origin lives under
  `$AI_SPECS_HOME/cache/projects/<key>/`; the project stays minimal.

So after a 0.15.0 sync, `ai-specs/skills/` holds five bundled skills that are not
local user skills. Verified on this repo:

| `ai-specs/skills/` entry | Source |
|---|---|
| harness-lifecycle, harness-recipes, harness-skills-deps | `bundled-skills/` (CLI) |
| skill-creator, skill-sync | `bundled-skills/` (CLI) |
| release-flow, testing-foundation | local-only (genuinely this project's) |

Two root causes:

1. **No CLI-bundled skill tier.** `skill-source-precedence` defines three
   sources (local / recipe-cache / dep-cache). Recipe-independent CLI-bundled
   skills have nowhere to go but the local tier, so they pollute it and get
   committed.
2. **`refresh-bundled` is an in-project, edit-preserving mechanism.** Its whole
   design (auto-update untouched files, drop `<name>.new` for customized ones,
   respect deletions) exists to let users edit bundled skills in place. Product
   decision: **we are removing user modification of bundled skills for now.**
   Without that, in-project materialization has no justification.

Related surface to align in the same change:

- **`ai-specs/recipes/` is committed wholesale.** It carries bundled recipe docs
  (`README.md`), hooks, and templates alongside genuine project overrides. Only
  the declared override surface (`recipe-overrides-runtime`:
  `ai-specs/recipes/{id}/overrides/`) is user-owned; the rest is regenerable.
- **The `.ai-specs.lock` per-file SHA-256 hashes exist only to power
  `refresh-bundled`'s edit detection.** Remove in-project bundled materialization
  and the hashes lose their purpose.

## Solution

Unify everything CLI-owned onto the cache model (0.14.0 direction) and keep only
project-governed content in-project. Governance boundary = **"is it declared in
the project's `ai-specs.toml`?"**, not "is it committed?".

1. **CLI-bundled skill tier (new).** Bundled skills resolve from the CLI
   (`$AI_SPECS_HOME/bundled-skills/`, flattened through the cache like recipe/dep
   skills). They are NEVER materialized into `ai-specs/skills/` and NEVER
   committed. Add this tier to `skill-source-precedence` (lowest precedence: a
   local skill of the same id still wins).
2. **Enforce local-skills exclusivity.** Sync stops writing bundled skills into
   `ai-specs/skills/` and cleans up leftovers (harness-*, skill-creator,
   skill-sync) the same way it cleans other legacy in-project origin trees.
3. **toml-deps governance split.** Deps split by origin:
   - **recipe-deps** (a recipe vendors a skill) → cache, unchanged.
   - **toml-deps** (`add-dep` → `[[deps]]` in `ai-specs.toml`) → project
     governance, materialized in-project at `ai-specs/.deps/`, **gitignored**
     (regenerable from the declared git source).
4. **recipes/ gitignore + declared override allow-list.** `ai-specs/recipes/` is
   gitignored by default; only the declared override surface
   (`ai-specs/recipes/{id}/overrides/`) is negated back and committed. Bundled
   recipe docs/hooks/templates resolve from cache.
5. **Lock → provenance stamp.** `.ai-specs.lock` drops per-file content hashes of
   bundled/recipe skills and collapses to `[meta]` (`cli_version`, `synced_at`) —
   the only CLI-provenance signal that survives a fresh clone (the cache
   `meta.toml` is machine-local). `doctor`/`upgrade` keep version-drift
   detection; git already provides integrity/diff for the committed surface.

## Open decisions (resolve at authorization)

- **D1 — toml-deps location.** User model: in-project `ai-specs/.deps/`,
  gitignored (governance visibility). Tension: the stated goal is *reducing
  device boilerplate*, and cache (`{cache}/.deps/`) keeps the project tree
  cleaner and is shareable/regenerable. In-project-gitignored reduces *committed*
  boilerplate but not on-disk footprint. **Recommendation to confirm:** honor the
  governance instinct (in-project gitignored) unless minimizing on-disk footprint
  outweighs it.
- **D2 — recipes/ override boundary.** Trello's `templates/` currently sit at
  `ai-specs/recipes/trello-mcp-workflow/templates/`, not under `overrides/`. Do
  we require overrides under the spec'd `overrides/` path (and treat top-level
  `templates/` as bundled → ignored), or widen the allow-list to include declared
  non-`overrides/` paths?
- **D3 — `refresh-bundled` fate.** Remove the command/flow entirely, or repurpose
  it as a cache-flatten step with no in-project write and no `.new` sidecars?

## Affected modules

- `lib/refresh-bundled.sh`, `lib/_internal/refresh-bundled.py` (bundled flow)
- `lib/sync.sh`, `lib/sync-agent.sh` (skill resolution, leftover cleanup, gitignore)
- `lib/_internal/vendor-skills.py`, `lib/_internal/platform.sh` (dep/lock paths)
- `lib/skills-add.sh`, `lib/skills-remove.sh`, `lib/recipe-remove.sh`, `lib/init.sh` (lock writes)
- `lib/_internal/cli_version.py` (meta stamp — becomes the lock's core)
- `templates/gitignore-*.tmpl` (recipes/ ignore + toml-deps ignore + negations)
- `openspec/specs/{skill-source-precedence,external-dirs-layout,project-recipe-cache,recipe-overrides-runtime}/spec.md` (deltas)
- `tests/` (skill resolution, leftover cleanup, gitignore generation, lock schema)

## Out of scope

- User-editable bundled skills (explicitly deferred — this change removes it).
- Changing recipe-dep materialization (recipe-deps stay in cache, unchanged).
- Reworking the cache key or `$AI_SPECS_HOME/cache` layout.
