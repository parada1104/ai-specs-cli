# Archive Report: cli-bound-recipes

**Archived:** 2026-07-14  
**Branch:** `feat/cli-bound-recipes`  
**Status:** ready-to-merge (pre-merge archive gate)

## Outcome

- **Change:** `cli-bound-recipes` — CLI-bound recipe catalog + off-project `$AI_SPECS_HOME` origin cache; drop per-recipe version pins.
- **Judgment Day:** APPROVED (#1264) after Round 3 final fix (sync-agent TOML guard before materialize) + re-judge; both judges CLEAN.
- **Verify:** PASS WITH WARNINGS (943 tests at verify) → JD fix rounds → **951 tests** green.
- **Tasks:** 23/23 complete on disk (`tasks.md`).

## Commits on branch (`development..HEAD`)

| SHA | Message |
|-----|---------|
| `f193652` | docs(openspec): add cli-bound-recipes planning artifacts |
| `237a3d6` | feat(recipes): drop per-recipe version pins (cli-bound phase 1) |
| `454acff` | feat(recipes): stage origin under AI_SPECS_HOME project cache |
| `fdec967` | docs(recipes): cache layout specs, doctor WARN, and #104 note |
| `c65fc26` | test(init-tui): deliver Ctrl-C via PTY \x03 to avoid hung SIGINT |
| `81387e2` | docs(openspec): record validate green for cli-bound-recipes apply |
| `559b6e3` | fix: gitignore user surface, sync-agent materialize ordering, doctor cache commands |
| `09f099d` | fix: guard remove_legacy_origin data loss and doctor false WARN for empty commands |
| `de6383f` | fix(sync-agent): run init guard before materialize to prevent misleading errors |

(+ this archive commit)

## Specs synced

| Domain | Action | Details |
|--------|--------|---------|
| `project-recipe-cache` | Created (apply) | Identical to delta; NEW capability already on disk |
| `recipe-manifest-contract` | Updated (apply) | Pin dropped; legacy WARN; post-upgrade catalog sync |
| `external-dirs-layout` | Rewritten (apply) | Origin under cache; in-project surface only |
| `skill-source-precedence` | Updated (apply) | Cache tiers + command merge |
| `recipe-overrides-runtime` | Updated (apply) | Overrides under `ai-specs/recipes/`; legacy migrate |
| `recipe-cli` | Completed at archive | ADDED `#104 documentation` + `No pin-bump UX` (MODIFIED list/add/catalog already applied) |

## Archive move

- Source: `openspec/changes/cli-bound-recipes/`
- Destination: `openspec/changes/archive/2026-07-14-cli-bound-recipes/`

## Engram observation IDs (traceability)

| Artifact | ID |
|----------|-----|
| proposal | #1255 |
| spec | #1256 |
| design | #1257 |
| tasks | #1259 |
| verify-report | #1262 |
| JD APPROVED | #1264 |

## Warnings carried forward (non-blocking)

Verify WARNs addressed or accepted via JD: #104 docs placement gaps, Strict TDD evidence coarseness, INFO residuals (dead `command-merge.py`, doctor message). No CRITICAL issues.
