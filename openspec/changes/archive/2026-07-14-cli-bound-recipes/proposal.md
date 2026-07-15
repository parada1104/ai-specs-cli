# Proposal: cli-bound-recipes

> Supersedes `sdd/recipe-update-flow` (pin-bump / guided `recipe update`). Pins were ack ceremony; catalog ships one version per CLI install.

## Intent

After `ai-specs upgrade`, exact recipe pins fail sync until hand-edited, and recipe/deps origin stages into the project (`ai-specs/.recipe`, `.deps`, managed commands, flatten staging)—noise that users mistake for project surface. Bind recipes to the installed CLI catalog and stage origin off-project so projects keep only config + local skills + docs/overrides.

## Scope

### In Scope
- Drop per-recipe `version` from user toml; `[recipes.<id>]` requires `enabled` (+ optional config)
- Sync always materializes **latest catalog** shipped with installed CLI (no pin check)
- Off-project cache for origin: `.recipe`, `.deps`, managed command staging, `.internal/resolved-skills` flatten
- Keep in-project: `ai-specs.toml`, `ai-specs/skills/`, `ai-specs/recipes/` (docs+overrides; hooks/bin/templates needing project-relative paths)
- Fan-out contract unchanged
- Legacy `version` keys: **ignore + migration WARN** (optional doctor note); sync works without toml edit
- On sync: **remove leftover** in-project `ai-specs/.recipe` and `ai-specs/.deps`
- `recipe list` may show catalog version as **info only** (not a pin)
- #104: WARN/note only (no template refresh)
- Spec/docs/test migration for manifest + layout contracts

### Out of Scope
- Guided pin-bump `recipe update` / outdated-pin UX (`recipe-update-flow`)
- Floating/`min_version` pins; multi-version recipe hosting
- Fan-out redesign; auto-sync flag
- Full #104 managed-template refresh (follow-up)
- Moving hooks/bin/`not_exists` templates that need stable project-relative paths off-project

## Locked assumptions

1. **Cache root**: `$AI_SPECS_HOME/cache/projects/<key>/` (prefer AI_SPECS_HOME over XDG — one knob). Key: short `sha256(realpath(project_root))` (+ optional name suffix); sidecar records `project_root` for doctor/debug.
2. **Flatten staging**: move `ai-specs/.internal/resolved-skills` into the same cache tree (origin flatten, not user surface).
3. **Legacy `version`**: ignore + WARN; do not force toml rewrite for sync to succeed; optional strip on next safe toml write path; optional doctor note.
4. **Cleanup migration**: sync deletes leftover in-project `ai-specs/.recipe` and `ai-specs/.deps` when present.
5. **List UX**: catalog version display is informational only.

## Capabilities

### New Capabilities
- `project-recipe-cache`: per-project CLI cache root, layout (`.recipe`/`.deps`/commands/resolved-skills), keying, orphan cleanup, migration cleanup of in-project origin trees

### Modified Capabilities
- `recipe-manifest-contract`: remove required `version` pin; legacy ignore+WARN; sync uses CLI catalog
- `external-dirs-layout`: rewrite — origin under AI_SPECS_HOME cache; in-project user surface only
- `recipe-cli`: no outdated-pin/`recipe update`; list shows catalog version as info; add/init stop writing version
- Skill-source / sync pipeline requirements: resolve from cache; command merge (cache managed + in-project hand-authored); fan-out unchanged

## Approach

1. **Manifest** — stop writing/requiring `version`; remove `validate_version_pin`; WARN on legacy keys.
2. **Cache module** — single resolver (`project-cache.py`) used by materialize, vendor, skill-resolution, flatten, orphan cleanup.
3. **Materialize** — stage skills/deps/managed commands + resolved-skills under cache; keep docs/hooks/templates policy in-project where paths matter.
4. **Commands** — managed origin in cache; hand-authored stay in `ai-specs/commands/`; fan-out merges (local hand-authored wins on conflict).
5. **Migration** — sync rm leftover in-project `.recipe`/`.deps`; stop init mkdir of those under project; update gitignore-render.
6. **UX** — drop pin-outdated doctor/list paths; optional CLI-upgrade→resync note; #104 WARN/docs only.
7. **Delivery** — chained PRs (manifest → cache layout → commands/migration → polish).

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `recipe-materialize.py` | Modified | Drop pin check; cache staging; leftover cleanup |
| `skill-resolution.py`, `vendor-skills.py`, flatten/`sync-agent.sh` | Modified | Resolve/stage from cache |
| `project-cache.py` | New | Cache root + key |
| `recipe-add/init/config-write`, list, doctor, hub | Modified | No version pin UX |
| `gitignore-render.py`, `init.sh`, `lock.py` | Modified | Layout + hash paths |
| Specs/docs/tests | Modified | Contract + fixtures |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Wide test/spec churn; 400-line PR budget | High | Chained slices |
| Worktree/path rename → new cache dir | Med | realpath key; sync rebuilds; sidecar |
| Incomplete leftover cleanup confuses users | Med | Aggressive sync rm of in-project `.recipe`/`.deps` |
| Moving hooks/templates breaks relative paths | Med | Keep project-relative artifacts in-project |
| Command merge clobbers hand-authored | Med | Precedence: local over recipe |
| #104 false confidence after upgrade+sync | Med | WARN/note only; follow-up |

## Rollback Plan

Revert feature branch/PRs. No required consumer data migration beyond re-adding pins if rolling back mid-upgrade (document). Cache dirs under AI_SPECS_HOME are disposable; re-sync rebuilds. Do not revive `recipe-update-flow` without explicit re-approval.

## Dependencies

- Explore #1254; storage-model #1253; user-decisions #1252
- Supersedes recipe-update-flow proposal/design/spec (#1248–#1250)
- #104 template refresh remains a separate follow-up

## Success Criteria

- [ ] Enabled recipes sync to CLI catalog without `version` in toml; legacy `version` WARN-only and does not block
- [ ] Origin skills/deps/managed commands/resolved-skills live under `$AI_SPECS_HOME/cache/projects/<key>/`; project keeps toml + `skills/` + `recipes/`
- [ ] Sync removes leftover in-project `ai-specs/.recipe` and `.deps`
- [ ] Fan-out targets and precedence unchanged; hand-authored commands preserved
- [ ] `recipe list` may show catalog version as info only; no pin-bump/`recipe update` path
- [ ] #104 documented as WARN/note only
- [ ] Specs `recipe-manifest-contract` + `external-dirs-layout` match the new model

## Proposal question round

Product/explore decisions already locked by user (2026-07-14); residual explore Qs 1–5 closed above. **No blocking open product questions.** Non-blocking delivery notes for tasks (not product): cache key suffix format; exact WARN copy; PR slice boundaries.

User may still correct framing or request a second round before design/spec if anything above is wrong.
