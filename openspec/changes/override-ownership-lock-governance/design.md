# Design: override-ownership-lock-governance

## Context

Sync seeds `condition=not_exists` templates under
`ai-specs/recipes/*/overrides/` once, then never refreshes them. Staleness is
detected by comparing on-disk bytes to the **current catalog**
(`util.override_is_stale`), so catalog evolution looks like a user edit. The
lock was intentionally reduced to a provenance stamp (`[meta]` + optional
`[agents.*]`) and no longer carries override integrity.

This design implements proposal locks L1–L7: scoped lock records, a sync
classifier, category policies, safe migration, and revised specs — without
resurrecting skill/recipe hash tables.

Grounding reads:
- `lib/_internal/lock.py`, `util.override_is_stale`, `recipe-materialize.materialize_template`
- `doctor._check_stale_template_overrides`
- `openspec/specs/sync-lock/spec.md`, `worktree-flow` stale-override requirement
- Catalog: `trello-mcp-workflow` (6 card templates), `worktree-flow` cleanup script

## Goals / Non-Goals

**Goals:**

1. Classify override targets as missing / managed-current / managed-stale /
   user-modified / untracked (missing metadata).
2. Force-update managed-stale under policy `auto`.
3. Preserve + warn user-modified and untracked (never force).
4. Seed lock records on every CLI write of a governed target.
5. Document category policies (auto / confirm / never-force).
6. Keep hooks as always-CLI rewrite; document them outside the override surface.

**Non-goals:** full lock hash revival; AGENTS.md ownership; interactive confirm
UX in v1; changing runtime override resolution order.

## Architecture

```text
catalog template ──► materialize_template
                         │
                         ├─ write dest bytes (post-placeholder)
                         └─ lock.managed[relpath] = { sha256, recipe, source, policy }

sync decision:
  missing dest                    → copy + record
  lock.sha == disk.sha
       catalog == disk            → no-op (managed-current)
       catalog != disk            → managed-stale → policy(auto|confirm|never-force)
  lock.sha != disk.sha            → user-modified → preserve + warn
  no lock entry                   → untracked → migrate (never force)
```

## Lock format

New optional section (project-relative paths as keys):

```toml
[meta]
cli_version = "0.19.0"
synced_at = "…"

[managed."ai-specs/recipes/trello-mcp-workflow/overrides/templates/card-feature.md"]
sha256 = "<hex of last CLI-written bytes>"
recipe = "trello-mcp-workflow"
source = "templates/card-feature.md"
kind = "template"
policy = "auto"
```

Rules:

- Keys are POSIX-style paths relative to project root (same as template
  `target`).
- `sha256` is mandatory; other fields are written when known and ignored if
  absent on read (forward compatible).
- `write_lock` **emits** `[managed.*]` (unlike legacy skill hashes).
- `load_lock` returns `managed: dict[str, dict]`.
- Unknown legacy sections continue to be dropped; `[agents.*]` unchanged.
- Header comment updated: provenance + **managed-override integrity** (not
  general content integrity).

## Classifier

Replace catalog-only `override_is_stale` usage for governed templates with a
shared helper (name locked for impl: e.g. `classify_managed_override`):

| Inputs | Result |
|---|---|
| dest missing | `missing` |
| dest present, no lock entry | `untracked` |
| dest sha == lock sha, catalog sha == dest sha | `managed_current` |
| dest sha == lock sha, catalog sha ≠ dest sha | `managed_stale` |
| dest sha ≠ lock sha | `user_modified` |

Hashing: normalize CRLF→LF consistently with `sha256_of` / existing write
path. For templates with placeholders (`__WORKTREE_REPO_TOPOLOGY__` etc.), the
lock hash MUST be of the **written** content (after substitution), and catalog
comparison for staleness MUST use the same rendered bytes the CLI would write
now (re-render catalog through the same placeholder path, or compare
pre-render only when no placeholders — design prefers always compare
would-write vs on-disk for the stale branch).

## Sync actions

| Result | Action |
|---|---|
| `missing` | Copy/render; record lock |
| `managed_current` | No file write; ensure lock fields present |
| `managed_stale` + `auto` | Overwrite with catalog render; update lock; info log (not user-modified warn) |
| `managed_stale` + `confirm` | **v1:** preserve + WARN naming file + refresh instructions (same as never-force messaging but labeled confirm-required). Optional later: `--refresh-managed` / interactive |
| `managed_stale` + `never-force` | Preserve + WARN |
| `user_modified` | Preserve + WARN (`user-modified`, include path + `rm … && sync` or refresh flag) |
| `untracked` | Migration (below) |

Doctor uses the same classifier: WARN for `user_modified`, `untracked` (when
catalog differs), and `managed_stale` under confirm/never-force; OK/silent for
managed_current; after sync, managed_stale+auto should clear.

## Migration (missing metadata)

On first sync encounter of an existing `not_exists` target with no lock entry:

1. If on-disk bytes == would-write catalog bytes → treat as managed: **do not
   rewrite**; **seed** lock with current sha + provenance. No warn.
2. If on-disk ≠ catalog → treat as `user_modified` / untracked-custom: **preserve**;
   WARN once-style message that metadata was missing and file was not
   overwritten; **do not** seed a managed hash (or seed an explicit
   `ownership = "user"` record — prefer **no managed entry** so future syncs
   keep preserving until user refreshes). Locked choice: **no managed entry**
   until the CLI successfully writes the file.

Never force-update in the untracked branch.

## Policy matrix

| Category / kind | Default policy | Notes |
|---|---|---|
| `template` (integration cards, cleanup scripts under overrides/) | `auto` | Fixes #63 primary pain |
| `doc` with not_exists (if wired) | `auto` | Only if DocRef gains condition; else out of v1 |
| Runtime hook scripts (`provides.hooks`) | n/a — **always-cli** | Keep unconditional overwrite; not recorded as user overrides; document as `never` user-force surface |
| Cache commands | out of scope | Already overwrite |

Optional recipe.toml field:

```toml
[[provides.templates]]
source = "templates/card-feature.md"
target = "ai-specs/recipes/trello-mcp-workflow/overrides/templates/card-feature.md"
condition = "not_exists"
update_policy = "auto"   # optional; default from kind/category
```

Allowed values: `auto` | `confirm` | `never-force`. Unknown → validation error.
Absence → category default (`auto` for templates).

v1 catalog: no required TOML churn if code defaults templates to `auto`;
document that recipes MAY set `never-force` for sensitive templates.

## Explicit refresh

Supported paths:

1. `rm <target> && ai-specs sync` (existing; seeds fresh managed record).
2. Optional CLI flag (design recommendation, implement if low-cost):
   `ai-specs sync --refresh-managed[=<path>|all]` — only refreshes targets
   classified `managed_stale` or explicitly listed; **refuses**
   `user_modified` unless `--force` (separate, dangerous; **out of v1** —
   user must `rm` to discard edits).

If flag cost is high, ship (1) only and document; tasks mark flag as optional.

## Spec impact

| Spec | Change |
|---|---|
| `sync-lock` | Allow/require `[managed.*]` for override integrity; keep ban on skill/recipe/command hashes |
| `override-ownership` (NEW) | Classifier, policies, migration, refresh |
| `recipe-schema` | Optional `update_policy` on templates |
| `worktree-flow` | Replace catalog-only stale requirement with lock-backed managed-stale auto-refresh + user-modified preserve |

## File-level plan

| Area | Files |
|---|---|
| Lock | `lib/_internal/lock.py`, `tests/test_lock.py` |
| Classifier | `lib/_internal/util.py` (or small `managed_override.py`) |
| Sync | `lib/_internal/recipe-materialize.py` (`materialize_template`) |
| Doctor | `lib/_internal/doctor.py` |
| Schema | `lib/_internal/recipe_schema.py` (+ parse tests) |
| Docs | sync-lock header, recipe README / docs mentioning refresh, policy table |
| Specs | deltas under this change folder → promote on apply |
| Catalog | optional `update_policy` docs; no mandatory recipe.toml edits if defaults suffice |

## Testing plan (acceptance)

| Case | Expectation |
|---|---|
| Managed current | No warn; no rewrite |
| Managed stale + auto | Rewrite; lock updated; no user-modified warn |
| User-modified | Preserve; clear warn |
| Missing metadata + matches catalog | Seed lock; no rewrite |
| Missing metadata + differs | Preserve; warn; no lock seed |
| Explicit refresh (`rm`+sync) | Fresh copy + lock |
| Hook materialize | Still always overwrite (regression) |

## Open questions (resolved in this design unless reopened)

| Q | Resolution |
|---|---|
| Lock section name | `[managed."path"]` |
| Confirm UX in v1 | Degrade to preserve+warn + refresh docs |
| `--refresh-managed` | Optional nicety; `rm`+sync is mandatory support |
| Docs condition | Verify in apply task; wire or exclude |
| Hash placeholders | Hash post-render would-write bytes |

## Authorization

Stop for human authorization before any production code. Planning artifacts
only until approved.
