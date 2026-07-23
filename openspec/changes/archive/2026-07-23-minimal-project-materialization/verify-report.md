# Verify report: minimal-project-materialization

**Change**: `2026-07-23-minimal-project-materialization`  
**Branch**: `change/minimal-project-materialization` @ `185476c`  
**Worktree**: `.worktrees/minimal-project-materialization`  
**Depth**: Full  
**Verified**: 2026-07-23

## Commands

| Layer | Command | Result |
|-------|---------|--------|
| Focused AC | `python3 -m unittest tests.test_external_dirs tests.test_harness_cli_literacy tests.test_lock tests.test_project_cache tests.test_doctor -v` | PASS — 126 tests, 24.438s |
| Migration AC subset | `BundledLeftoverCleanupTests` + `SkillResolutionTests` + `InitExternalDirsTests` + `VendorSkillsPathTests` + `HarnessCliLiteracyTests` + `LockRoundTripTests` | PASS — 32 tests, 2.719s |
| Full | `./tests/validate.sh` | PASS — 1020 tests, 226.342s, EXIT 0 |

Scratchpad `migrate_smoke.sh` from apply is gone (disposable). Migration coverage is re-verified via the unit AC subset above (leftover cleanup, lock-hash migration, refresh-bundled ordering, lock stamp collapse).

## Spec compliance matrix

### skill-source-precedence

| Scenario | Evidence | Result |
|----------|----------|--------|
| Bundled skill resolves from `{cache}/.bundled/`, not project | `test_bundled_fallback_when_no_other_source`, `test_refresh_bundled_flattens_harness_skills_to_cache` | ✅ |
| Local skill shadows bundled; not deleted | `test_local_precedence_over_bundled`, `test_removes_bundled_leftover_keeps_local_and_customized` | ✅ |
| Precedence local > recipe > dep > bundled | `SkillResolutionTests` (local/recipe/dep/bundled matrix) | ✅ |

### external-dirs-layout

| Scenario | Evidence | Result |
|----------|----------|--------|
| Leftover bundled skills removed on sync | `test_removes_bundled_leftover_keeps_local_and_customized` | ✅ |
| Untouched old-version copy removed via legacy lock hash | `test_removes_untouched_old_version_copy_via_lock_hash`, `test_refresh_bundled_migrates_inproject_copy_via_lock_hash` | ✅ |
| Edited copy preserved | `test_keeps_edited_copy_not_matching_source_or_lock` | ✅ |
| toml-dep → `ai-specs/.deps/` (not cache) | `test_vendor_writes_to_deps_dir`, `test_inproject_toml_dep_resolves_as_dep` | ✅ |
| recipe-dep stays in cache | `test_materializes_recipe_dep_skill_to_deps_dir` | ✅ |
| `.deps` + `recipes/**` ignored; `*/overrides/` kept | `test_gitignore_ignores_recipes_except_overrides` (`git check-ignore`) | ✅ |
| refresh-bundled flatten-only (no in-project write) | `test_refresh_bundled_flattens_harness_skills_to_cache` | ✅ |

### sync-lock

| Scenario | Evidence | Result |
|----------|----------|--------|
| Lock is provenance stamp (`[meta]`); no skill/recipe hashes | `test_skill_recipe_dep_hashes_not_emitted` | ✅ |
| Legacy hash sections dropped on rewrite | `test_legacy_hash_sections_dropped_on_rewrite` | ✅ |
| Doctor version drift from `[meta].cli_version` | `test_no_pin_stale_last_sync_reports_warn`, `test_doctor_cli_version_is_read_only` | ✅ |
| Doctor checks bundled skills in cache | `test_bundled_skills_present_reports_ok` (+ related doctor suite) | ✅ |

**Compliance summary**: all listed scenarios ✅ COMPLIANT for unit/integration gates.

## Issues found

**CRITICAL**: None.

**WARNING**: None.

**SUGGESTION**: Spec text in `external-dirs-layout` says “Root `.gitignore` SHALL ignore `ai-specs/.deps/`…”. Implementation correctly writes relative rules into `ai-specs/.gitignore` (`.deps/`, `recipes/**`, `!recipes/*/overrides/`), and root `.gitignore` stays agent-surface only. Behavior matches intent; at archive, tighten the delta wording to “`ai-specs/.gitignore`” so it matches the renderer and tests.

**FOLLOW-UPS** (explicitly out of this change; from tasks.md):

1. Migration guidance for projects that already committed `ai-specs/recipes/` under 0.15 — users need `git rm -r --cached` (keep `*/overrides/`).
2. Relocate bundled COMMANDS to cache and drop `[commands]`/`[opted-out]` from the lock.

## Verdict

**PASS**

Materialization model (four-tier skills, toml-deps split, recipes override surface, lock stamp, leftover migration) is verified green on the change branch. Full suite 1020/1020.

**Next recommended**: open PR → `development`, then archive-tail on the review branch before merge.
