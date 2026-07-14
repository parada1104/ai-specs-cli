# Apply progress: cli-bound-recipes

**Mode**: Strict TDD  
**Delivery**: single PR on `feat/cli-bound-recipes` (authorized singular PR; phase commits)  
**Status**: 23/23 tasks complete

## Commits
1. Phase 1 — unpin + WARN + add/init/list (`237a3d6`)
2. Phase 2 — `project-cache` + materialize/vendor/skill-resolution origin move
3. Phase 3 — commands merge + flatten + sync-agent + init/gitignore
4. Phase 4 — doctor/hub/docs/#104 + on-disk specs + fixture sweep + PTY flake fix

## TDD Cycle Evidence

| Phase | Tests | RED | GREEN | Notes |
|-------|-------|-----|-------|-------|
| 1 | materialize/add/list/toml/config/init render | ✅ | ✅ | legacy WARN; no pin fail-close |
| 2 | `test_project_cache`, external_dirs, materialize paths | ✅ | ✅ | cache key/meta/leftover migrate |
| 3 | `test_command_merge`, init/gitignore, sync-agent absolute skills links | ✅ | ✅ | local command wins; macOS /var fix |
| 4 | doctor legacy WARN + cache symlink OK; docs/#104; specs rewrite | ✅ | ✅ | fixture sweep via `_cache_paths` |

### Full suite
`./tests/validate.sh` — green after PTY Ctrl-C tests switched from `send_signal(SIGINT)` to PTY `\x03` (avoids hung prompt_toolkit → rc 120).

## Deviations
- Absolute symlinks for cache-backed `resolved-skills` (relative links break under `/var/folders` → `/private/var`).
- `command-merge.py` thin wrapper; sync-agent uses `project-cache.py merge-commands`.
- Fixed stale `config_wizard` test to assert `write_envrc` (pre-existing API drift).
- PTY Ctrl-C e2e: deliver `\x03` on master FD instead of process SIGINT.

## Risks
- Worktree / rename creates a new cache key (sidecar records old root); expected by design.
