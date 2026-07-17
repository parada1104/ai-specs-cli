# Tasks: sync-legacy-cleanup

Depth: standard

## Goal

Minimize in-project surface after sync: clean legacy skill-cache leftovers, keep root agent gitignore current, and stop staging shared helpers into `ai-specs/bin/` (invoke from `$AI_SPECS_HOME` instead).

## Tasks

- [x] 1. RED/GREEN: `remove_legacy_origin` also deletes leftover `ai-specs/.resolved-skills/` and `ai-specs/.internal/`, plus stale `ai-specs/bin/premerge_guardian.py` (and empty `ai-specs/bin/`)
- [x] 2. RED/GREEN: sync refreshes the root `.gitignore` managed agent block from `templates/gitignore-root.tmpl` (so `.pi/` / `.omp/` appear on existing projects)
- [x] 3. RED/GREEN: drop `[[provides.templates]]` → `ai-specs/bin/premerge_guardian.py` from plan-build-flow + VCS recipes; skills/docs invoke `${AI_SPECS_HOME:-$HOME/.ai-specs}/lib/_internal/premerge_guardian.py`
- [x] 4. Update unit tests; run `./tests/run.sh` (or focused suite) green
- [x] 5. Dogfood: remove committed `ai-specs/bin/` from this repo if present

## Out of scope

- New CLI subcommand wrapper for premerge check (optional follow-up)
- Per-project cache staging of bin (rejected — shared helper belongs in CLI home)
