# Proposal: override-ownership-lock-governance

## Intent

Give sync a reliable ownership signal for CLI-seeded overrides so it can
**force-update stale managed files**, **preserve and warn on user modifications**,
and **migrate existing projects without destructive guesses**. Governance is
category-aware: evolving integration templates (e.g. Trello cards) default to
automatic refresh when still CLI-owned; safety-sensitive hook scripts remain
CLI-always-rewrite and are documented as outside the user-override surface.

## Tracker

- **card_id**: `wdwyRFTS`
- **url**: https://trello.com/c/wdwyRFTS
- **title**: Override ownership and lock governance
- **board**: ai-specs-cli Roadmap

## Why now

After lock minimization, `.ai-specs.lock` no longer tracks content integrity for
materialized files. Stale detection falls back to comparing overrides to the
**current** catalog. Catalog evolution is indistinguishable from user edits, so
sync conservatively preserves and warns — leaving Trello templates and similar
surfaces permanently stale. Card #63 accepts lock provenance + policy + tests
as the fix.

## Depth

**Full** — new lock capability, sync decision tree, migration, schema/policy
surface, and revision of existing "never overwrite when catalog differs"
requirements (worktree-flow stale override). Cross-cutting across lock, sync,
doctor, recipe schema, and catalog recipes.

## Scope

### In scope

1. **Lock managed-override records** — extend `ai-specs/.ai-specs.lock` with a
   scoped integrity section for CLI-managed override targets (path → sha256 of
   last CLI-written content + minimal provenance). Do **not** resurrect
   skill/recipe/dep/command hash tables.
2. **Sync classifier** — for `condition=not_exists` (and aligned) templates:
   missing / managed-current / managed-stale / user-modified / missing-metadata.
3. **Force-update when managed-stale** — when on-disk bytes still match the
   last CLI-managed hash and catalog differs, update per category policy
   (default `auto` for templates).
4. **Preserve + warn user-modified** — when on-disk ≠ last managed hash,
   never overwrite; emit a clear user-modified warning (not a generic "differs
   from catalog" message).
5. **Safe migration** — projects without new metadata: never force-update on
   first encounter; seed managed records only when dest matches catalog (or
   other non-destructive rule locked in design).
6. **Policy matrix** — document and implement defaults for
   `auto` | `confirm` | `never-force` by artifact category (templates vs hooks
   vs docs); hooks remain always-CLI rewrite.
7. **Explicit refresh** — keep `rm <target> && ai-specs sync`; design may add
   an opt-in refresh path without changing the default safety posture.
8. **Tests** — managed unchanged, managed stale (force), user-modified,
   missing metadata / migration, explicit refresh.
9. **Spec updates** — sync-lock, override-ownership (new), recipe-schema
   (optional policy field), worktree-flow (revise stale-override requirement).

### Out of scope

- Reintroducing full skill/recipe/dep content-hash lock sections.
- Changing runtime override *resolution* (config.toml / templates/ preference)
  in `recipe-overrides-runtime`.
- Interactive TUI for every stale file in v1 (confirm policy may defer to
  flag/env or treat as preserve+warn until a later UX).
- Reworking AGENTS.md / runtime-brief ownership markers.
- Force-updating user-customized Trello copy without their consent.

## Approach (summary)

1. Record last-managed sha256 in the lock when the CLI writes an override.
2. On sync, classify using lock hash + catalog hash + on-disk hash.
3. Apply category policy for the managed-stale branch only.
4. Migrate missing metadata conservatively.
5. Align doctor messaging with the same classifier.
6. Document auto / confirm / never-force; ship catalog defaults for Trello +
   worktree-flow templates.

## Locked decisions (proposal-level)

| # | Decision | Rationale |
|---|---|---|
| L1 | Depth is **full** | Cross-cutting lock + sync + migration + policy |
| L2 | Integrity is **override-scoped** only | Preserve minimal lock; git still covers general integrity |
| L3 | Default template policy is **`auto`** for managed-stale | Fixes stale Trello/worktree templates without ceremony |
| L4 | User-modified always **preserve + warn** | Acceptance + safety |
| L5 | Missing metadata → **never force** | Safe migration |
| L6 | Hooks stay **always-CLI rewrite** (`never-force` N/A as user-override) | Matches current materialize; high risk if treated as overrides |
| L7 | Manual `rm` + sync remains a supported explicit refresh | Already documented; keep |

## Success criteria (acceptance)

- [ ] Lock records provenance/integrity sufficient to distinguish managed vs user-modified overrides.
- [ ] Sync force-updates when content still matches last CLI-managed and policy allows.
- [ ] Sync preserves and clearly warns on user-modified overrides.
- [ ] Existing projects without new metadata migrate non-destructively.
- [ ] Policy documents auto / confirm / never-force by category/risk.
- [ ] Tests cover managed, stale, user-modified, missing-metadata, migration, explicit refresh.

## Risks

| Risk | Mitigation |
|---|---|
| Spec tension with "lock is not an integrity manifest" | Narrow exception language in sync-lock; only managed overrides |
| False "user-modified" after line-ending / placeholder transforms | Hash the **bytes the CLI actually wrote** (post-placeholder), same as write path |
| Confirm policy without UX | Design: `confirm` may degrade to preserve+warn + documented refresh until interactive path exists |
| Docs `condition` ignored by DocRef | Design verifies; either wire condition or exclude docs from v1 |

## Non-goals reminder

No production code in this planning commit. Stop for authorization before apply.
