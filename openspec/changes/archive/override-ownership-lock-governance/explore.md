# Explore: override-ownership-lock-governance

Phase: **explore** (planning only, 2026-08-05)
Branch: `change/override-ownership-lock-governance`
Worktree: `.worktrees/override-ownership-lock-governance`
Base: `development@12afc3f`
Tracker: Trello #63 (`wdwyRFTS`) — https://trello.com/c/wdwyRFTS

## Problem framing

Sync cannot tell a CLI-seeded override from a user-edited one. For
`condition = "not_exists"` templates, sync compares the on-disk file to the
**current catalog** source (`util.override_is_stale`). Any catalog evolution
looks identical to a user edit → non-blocking WARN + preserve → generated
overrides go permanently stale unless the user manually `rm` + re-sync.

This hurts evolving integration surfaces hardest (Trello card templates under
`ai-specs/recipes/trello-mcp-workflow/overrides/templates/`). Safety-sensitive
artifacts (runtime hooks) already use a different path (always rewrite) and
need an explicit, documented policy rather than accidental uniformity with
templates.

## Current lock contract

| Fact | Evidence |
|---|---|
| Lock path | `ai-specs/.ai-specs.lock` via `lib/_internal/lock.py` |
| Canonical role | Provenance stamp: `[meta].cli_version` + `synced_at` |
| Still written | Optional `[agents.*]` hashes (doctor stale-file seam) |
| Intentionally dropped | Per-file skill/recipe/dep/command hashes (minimal-project + relocate-bundled-commands) |
| Spec | `openspec/specs/sync-lock/spec.md` — "not an integrity manifest for committed content" |
| Dogfood lock today | `[meta]` only (`cli_version = "0.18.0"` in this checkout) |

Implication: reintroducing **selective** integrity for managed overrides is a
deliberate, scoped exception to the "lock is not an integrity manifest"
posture — not a full return to hashing every skill.

## Current override / template behavior

| Surface | Behavior today | Ownership signal |
|---|---|---|
| `[[provides.templates]]` + `condition=not_exists` | Skip if dest exists; WARN if dest ≠ catalog bytes | Catalog comparison only |
| Doctor `_check_stale_template_overrides` | Same catalog comparison → WARN + `rm … && sync` guidance | Same |
| Trello card templates (6 files) | Seeded once under `overrides/templates/` | Committed; never refreshed |
| worktree-flow cleanup script | Same under `overrides/bin/` | Spec'd as "stale cleanup override detection" |
| `[[provides.hooks]]` scripts | `materialize_hook_script` **always** overwrites (+ placeholder stamp) | CLI-owned; no user-edit path |
| Recipe commands (cache) | Always copy; WARN if overwriting differing managed cmd | Cache-managed |
| `[[provides.docs]]` | Unconditional copy (trello README uses `condition` in TOML but DocRef has no condition field — docs path may ignore it) | Check during design |
| Runtime brief / AGENTS.md | Marker / user-managed opt-out | Separate from lock |

Root helper: `util.override_is_stale(catalog_src, dest)` — sha256 of current
catalog vs dest. **No memory of last CLI-written bytes.**

## Failure modes (acceptance-aligned)

| Case | Today | Desired |
|---|---|---|
| Managed, unchanged vs last CLI write; catalog evolved | WARN + preserve (false "user edit") | Force-update per policy (`auto` default for templates) |
| Managed, matches catalog | Silent skip | Silent skip; refresh lock stamp if needed |
| User-modified (differs from last CLI write) | WARN + preserve (correct outcome, wrong reason) | Preserve + clear **user-modified** WARN |
| Missing dest | Fresh copy | Fresh copy + record managed hash |
| No lock metadata (legacy project) | Catalog compare only | Safe migration: never force; classify / seed conservatively |
| Explicit refresh | Manual `rm` + sync | Keep `rm` path; optionally document/confirm CLI refresh flag |

## Category / risk notes

| Category | Examples | Risk if auto-forced | Likely default policy |
|---|---|---|---|
| Integration templates | Trello `card-*.md` | Low–medium (user may customize copy) | `auto` when still managed |
| Operational scripts under overrides | `worktree-cleanup.sh` | Medium | `auto` when managed; strong user-modified preserve |
| Runtime hooks | `worktree-gate.sh`, `tracker-card-gate.sh` | High if user patches were expected — but product always rewrites today | `always-cli` (never treat as user override surface) |
| Docs / README seeds | recipe README `not_exists` | Low | Align with templates or leave out of v1 if docs ignore condition |
| Commands in cache | recipe commands | N/A (not committed overrides) | Out of governance v1 |

## Spec / recipe touchpoints

- `openspec/specs/sync-lock/spec.md` — must gain a scoped managed-override integrity section without resurrecting skill hashes.
- `openspec/specs/worktree-flow/spec.md` — "Stale Cleanup Override Detection" currently mandates catalog compare + never overwrite; must be revised once lock-backed ownership exists.
- `openspec/specs/recipe-schema/spec.md` — templates only know `condition`; no `update_policy`.
- `openspec/specs/recipe-overrides-runtime/spec.md` — runtime preference of override files; orthogonal but should stay consistent.
- Catalog: `trello-mcp-workflow`, `worktree-flow` (and any other `not_exists` templates).

## Code touchpoints (for design, not apply)

- `lib/_internal/lock.py` — load/write new section(s)
- `lib/_internal/util.py` — `override_is_stale` or successor classifier
- `lib/_internal/recipe-materialize.py` — `materialize_template` decision tree
- `lib/_internal/doctor.py` — `_check_stale_template_overrides`
- `lib/_internal/recipe_schema.py` — optional policy field on `TemplateRef`
- Tests: `test_lock.py`, `test_recipe_materialize.py`, `test_repo_topology.py` (stale helper), `test_external_dirs.py`, doctor tests

## Alternatives considered (input to proposal)

1. **Lock-backed last-managed hash (recommended)** — distinguish managed-stale vs user-modified; migrate safely.
2. **Sidecar `.ai-specs.managed` next to each override** — more files; worse git noise; rejected as primary.
3. **Always overwrite templates** — breaks legitimate customizations; rejected.
4. **mtime / "untouched since sync"** — fragile across clones/CI; rejected.
5. **Git blame / clean working tree heuristics** — not portable for fresh clones of customized overrides; rejected.

## Open for design

1. Lock TOML shape (`[managed."path"]` vs `[overrides."path"]` vs nested by recipe).
2. Exact migration rules when metadata is missing and dest ≠ catalog.
3. Whether `update_policy` is per-template in `recipe.toml`, category defaults in code, or both.
4. Confirm vs auto UX (interactive flag? env? doctor-only?).
5. Scope of v1: templates only vs templates+docs; hooks documented as always-cli.
6. Whether explicit refresh is a new flag (`--refresh-managed`) or stays `rm`+sync.

## Ready for Proposal

Yes — problem and root cause are concrete; capability is cross-cutting (lock +
sync + doctor + schema + specs) → **full** depth.
