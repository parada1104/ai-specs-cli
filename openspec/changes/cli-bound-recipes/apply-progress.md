# Apply progress: cli-bound-recipes

**Mode**: Strict TDD  
**Delivery**: single PR on `feat/cli-bound-recipes` (authorized singular PR; phase commits)  
**Status**: 23/23 tasks complete

## Commits
1. Phase 1 — unpin + WARN + add/init/list (`237a3d6`)
2. Phases 2–3 — project-cache + origin move + commands/flatten/sync-agent/init
3. Phase 4 — doctor/hub/docs/#104 + on-disk specs + fixture sweep

## TDD Cycle Evidence

| Phase | Tests | RED | GREEN | Notes |
|-------|-------|-----|-------|-------|
| 1 | materialize/add/list/toml/config/init render | ✅ | ✅ | legacy WARN; no pin fail-close |
| 2 | `test_project_cache`, external_dirs, materialize paths | ✅ | ✅ | cache key/meta/leftover migrate |
| 3 | `test_command_merge`, init/gitignore, sync-agent absolute skills links | ✅ | ✅ | local command wins; macOS /var fix |
| 4 | doctor legacy WARN + cache symlink OK; docs/#104; specs rewrite | ✅ | ✅ | fixture sweep via `_cache_paths` |

### Full suite
`./tests/validate.sh` — re-run after final commit prep.

## Deviations
- Absolute symlinks for cache-backed `resolved-skills` (relative links break under `/var/folders` → `/private/var`).
- `command-merge.py` thin wrapper; sync-agent uses `project-cache.py merge-commands`.
- Fixed stale `config_wizard` test to assert `write_envrc` (pre-existing API drift).

## Risks
- PTY Ctrl-C init_tui e2e tests may flake (timeout 120) in this environment.
