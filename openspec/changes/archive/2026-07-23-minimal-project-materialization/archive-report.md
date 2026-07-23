# Archive report — minimal-project-materialization

**Archived:** 2026-07-23  
**Branch:** `change/minimal-project-materialization`  
**Status:** ready-to-merge (pre-merge archive gate)

## Outcome

Minimize the committed project surface after 0.15.0:

- CLI-bundled skills resolve from `{cache}/.bundled/` (tier-4); never materialize
  into `ai-specs/skills/`. Leftovers cleaned with content-match + legacy lock-hash
  migration guard (ordering: migrate inside `refresh-bundled` before lock normalize).
- toml-deps (`[[deps]]`) → gitignored `ai-specs/.deps/`; recipe-deps stay in cache.
- `ai-specs/recipes/**` ignored except `*/overrides/`.
- `.ai-specs.lock` collapsed to a provenance stamp (`[meta]`; optional
  `[commands]`/`[opted-out]` retained as follow-up).
- `init` no longer copies `skill-creator`/`skill-sync` into the project;
  `doctor` checks bundled skills in cache.

## Decisions (D1–D3)

| ID | Decision |
|----|----------|
| D1 | toml-deps in-project `ai-specs/.deps/` (gitignored) |
| D2 | recipes override boundary = `overrides/` only |
| D3 | `refresh-bundled` flatten-only (no in-project write / `.new`) |

## Specs synced

| Domain | Action |
|--------|--------|
| `skill-source-precedence` | Updated — four-tier scanning + bundled scenarios |
| `external-dirs-layout` | Rewritten — bundled leftovers, toml-deps split, recipes overrides, `ai-specs/.gitignore` |
| `sync-lock` | Created — provenance stamp capability |

## Verification

- Verify **PASS** (no CRITICAL/WARNING): `verify-report.md`
- `./tests/validate.sh` — 1020 tests, EXIT 0
- Migration AC subset — 32 tests OK

## Follow-ups (not this change)

1. Migration guidance for projects that already committed `ai-specs/recipes/` under 0.15.
2. Relocate bundled COMMANDS to cache; drop `[commands]`/`[opted-out]` from lock.

## Archive move

- Source: `openspec/changes/2026-07-23-minimal-project-materialization/`
- Destination: `openspec/changes/archive/2026-07-23-minimal-project-materialization/`
