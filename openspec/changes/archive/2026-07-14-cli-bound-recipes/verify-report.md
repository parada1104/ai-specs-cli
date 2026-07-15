## Verification Report

**Change**: cli-bound-recipes  
**Version**: N/A (delta change; on-disk specs rewritten for cache/manifest/overrides/skill-source; recipe-cli partial)  
**Mode**: Strict TDD (`openspec/config.yaml` → `strict_tdd: true`)  
**Worktree**: `.worktrees/cli-bound-recipes` @ `feat/cli-bound-recipes`  
**Verified**: 2026-07-14

### Completeness

| Metric | Value |
|--------|-------|
| Tasks total | 23 |
| Tasks complete | 23 |
| Tasks incomplete | 0 |

All Phase 1–4 checkboxes in `tasks.md` are `[x]`. Apply progress records 23/23 with commit evidence (`237a3d6`, `454acff`, `fdec967`, `c65fc26`).

### Build & Tests Execution

**Build**: ✅ Passed (via `./tests/validate.sh`: `py_compile` + `bash -n`)

```text
./tests/validate.sh
python3 -m py_compile lib/_internal/*.py tests/*.py
bash -n lib/*.sh bin/ai-specs tests/*.sh
./tests/run.sh
```

**Tests**: ✅ 943 passed / ❌ 0 failed / ⚠️ 0 skipped

```text
Ran 943 tests in 222.049s
OK
EXIT:0
```

**Coverage**: ➖ Not available (no coverage tool in project capabilities)

### Strict TDD Evidence

| Check | Result | Notes |
|-------|--------|-------|
| TDD Cycle Evidence table in apply-progress | ✅ Present | Phase-level RED/GREEN for Phases 1–4 |
| Covering test files exist | ✅ | `test_project_cache`, `test_command_merge`, `test_recipe_materialize`, `test_external_dirs`, `test_recipe_list/add`, sync pipeline |
| GREEN still green at verify | ✅ | Full suite 943 OK |
| Per-task TRIANGULATE / SAFETY NET columns | ⚠️ Partial | apply-progress uses phase rows, not full strict columns |
| Coverage on changed files | ➖ Skipped | no coverage tool |

**TDD summary**: 4/4 phases report RED→GREEN; live suite confirms green. Format is coarser than full Strict TDD per-task matrix → WARNING only.

### Spec Compliance Matrix

| Capability | Scenario | Test / evidence | Result |
|------------|----------|-----------------|--------|
| project-recipe-cache | Cache path | `test_project_cache` > `test_ensure_cache_writes_meta_toml`, `test_cache_key_*` | ✅ COMPLIANT |
| project-recipe-cache | Surface split | `test_external_dirs` > materialize/vendor/local exclusivity; cache path helpers | ✅ COMPLIANT |
| project-recipe-cache | Leftover rm | `test_project_cache` > `test_remove_legacy_origin_*`; materialize calls `remove_legacy_origin` | ✅ COMPLIANT |
| project-recipe-cache | Orphan via resolver | `test_external_dirs` > `test_orphan_recipe_directory_removed`, `test_orphan_dep_directory_removed` | ✅ COMPLIANT |
| recipe-manifest-contract | No version | `test_recipe_materialize` > `test_sync_without_version_succeeds` | ✅ COMPLIANT |
| recipe-manifest-contract | Legacy WARN | `test_recipe_materialize` > `test_legacy_version_warns_and_succeeds` | ✅ COMPLIANT |
| recipe-manifest-contract | Disabled / unknown | `test_disabled_recipe_skips_materialization`, `test_unknown_recipe_fails` | ✅ COMPLIANT |
| recipe-manifest-contract | In-place update / no version write | `test_recipe_add` > `test_add_appends_recipe_without_version`; `test_recipe_config_write` asserts no `version` | ✅ COMPLIANT |
| recipe-manifest-contract | Post-upgrade | Covered by no pin fail-close + catalog materialize (`test_sync_without_version_succeeds`); no separate upgrade simulator | ✅ COMPLIANT |
| external-dirs-layout | Recipe skills in cache | `test_external_dirs` > `test_materializes_bundled_skill_to_recipe_dir` | ✅ COMPLIANT |
| external-dirs-layout | Dep skills in cache | `test_external_dirs` > `test_vendor_writes_to_deps_dir` | ✅ COMPLIANT |
| external-dirs-layout | No skills pollution | `test_local_skills_untouched_by_materialization`, `test_vendor_does_not_write_to_ai_specs_skills` | ✅ COMPLIANT |
| external-dirs-layout | Not origin | docs/hooks under `ai-specs/recipes/`; origin under cache (materialize + surface tests) | ✅ COMPLIANT |
| external-dirs-layout | Init / gitignore migration | `test_init_does_not_create_in_project_origin_dirs`, `test_gitignore_omits_in_project_origin_dirs` | ✅ COMPLIANT |
| recipe-cli | Info not pin | `test_recipe_list` > `test_list_catalog_version_info_only_not_outdated` | ✅ COMPLIANT |
| recipe-cli | Empty / uninitialized | `test_empty_catalog`, `test_cli_uninitialized_project` | ✅ COMPLIANT |
| recipe-cli | Add (no version, no sync) | `test_recipe_add` > `test_add_appends_recipe_without_version` (+ no-materialize contract) | ✅ COMPLIANT |
| recipe-cli | Authoritative CLI catalog | `test_list_uses_cli_catalog_*`, `test_add_uses_cli_catalog_*` | ✅ COMPLIANT |
| recipe-cli | #104 docs | `docs/recipe-schema.md` § Managed templates (#104); no automated doc needle test | ⚠️ PARTIAL |
| recipe-cli | No update path | No `recipe-update.py`; list forbids `outdated`; pin-bump abandoned | ✅ COMPLIANT |
| skill-source-precedence | Precedence | `test_external_dirs` > local/recipe/dep precedence tests | ✅ COMPLIANT |
| skill-source-precedence | Merge and fan-out | `test_command_merge` > `test_local_wins_over_managed`; `sync-agent.sh` wires `merge-commands`; fan-out targets via sync pipeline | ✅ COMPLIANT |
| recipe-overrides-runtime | Override config exists/missing/isolation | `test_external_dirs` > `test_override_config_*` | ✅ COMPLIANT |
| recipe-overrides-runtime | Override template | `test_override_template_preferred` (+ fallback) | ✅ COMPLIANT |
| recipe-overrides-runtime | Migrate before leftover rm | `test_project_cache` > `test_remove_legacy_origin_migrates_overrides_then_deletes` | ✅ COMPLIANT |

**Compliance summary**: 25/26 scenarios ✅ COMPLIANT; 1/26 ⚠️ PARTIAL (#104 docs placement/test needle)

### Correctness (Static Evidence)

| Requirement | Status | Notes |
|------------|--------|-------|
| Drop `validate_version_pin` | ✅ Implemented | Absent from `lib/`; materialize WARNs only |
| `project-cache.py` resolver | ✅ Implemented | Key, meta.toml, path helpers, leftover rm, merge-commands |
| Origin under `$AI_SPECS_HOME/cache/projects/<key>/` | ✅ Implemented | Used by materialize/vendor/skill-resolution/flatten |
| In-project surface only toml + skills + recipes | ✅ Implemented | Init/gitignore stop creating/ignoring origin dirs |
| Fan-out targets unchanged | ✅ Implemented | Sync pipeline symlink/fan-out tests still green |
| Legacy version ignore+WARN | ✅ Implemented | materialize + doctor `_check_legacy_recipe_versions` |
| Dogfood `ai-specs/ai-specs.toml` | ✅ Consistent | No recipe `version=` pins; comment documents legacy |
| Fixtures still writing `version=` | ✅ Intentional | Legacy WARN / catalog metadata paths; suite green |
| On-disk `openspec/specs/*` | ⚠️ Mostly rewritten | `project-recipe-cache`, `external-dirs-layout`, `recipe-manifest-contract`, `recipe-overrides-runtime`, `skill-source-precedence` updated; **`recipe-cli` missing #104 / No pin-bump ADDED reqs** |

### Coherence (Design)

| Decision | Followed? | Notes |
|----------|-----------|-------|
| Cache under AI_SPECS_HOME (not XDG) | ✅ Yes | |
| Key = short sha256 + basename | ✅ Yes | `test_cache_key_stable_and_includes_basename` |
| Flatten in cache `resolved-skills/` | ✅ Yes | Absolute symlinks deviation documented (macOS `/var` → `/private/var`) |
| Legacy version ignore+WARN | ✅ Yes | |
| Leftover `.recipe`/`.deps` rm on sync | ✅ Yes | |
| Overrides → `ai-specs/recipes/<id>/overrides/` | ✅ Yes | migrate-before-rm tested |
| Managed commands in cache; local wins | ✅ Yes | `command-merge.py` thin wrapper + `project-cache.merge_commands` (documented deviation) |
| Delete pin check / no recipe-update | ✅ Yes | |
| 4 chained PRs | ⚠️ Authorized deviation | Single PR on `feat/cli-bound-recipes` with phase commits (apply-progress) |
| Diff size | ⚠️ High | ~64 files, +2286/−337 vs `development` — review budget risk remains |

### Proposal success criteria

| Criterion | Met? |
|-----------|------|
| Sync without toml `version`; legacy WARN-only | ✅ |
| Origin under AI_SPECS_HOME cache; project keeps toml/skills/recipes | ✅ |
| Sync removes leftover in-project `.recipe`/`.deps` | ✅ |
| Fan-out + hand-authored commands preserved | ✅ |
| `recipe list` info-only; no pin-bump/`recipe update` | ✅ |
| #104 documented WARN/note only | ⚠️ Partial — present in `docs/recipe-schema.md`; missing from `docs/recipes-catalog.md` + `docs/ai/troubleshooting.md` (task 4.2 named those) |
| Specs match new model | ⚠️ Near-complete — recipe-cli on-disk delta merge incomplete for #104 / no-pin-bump |

Proposal.md success-criteria checkboxes remain unchecked (process hygiene only).

### Issues Found

**CRITICAL**: None

**WARNING**:
1. **#104 docs placement incomplete vs task 4.2** — `docs/recipe-schema.md` has the WARN/note; `docs/recipes-catalog.md` and `docs/ai/troubleshooting.md` do not mention #104 / non-refresh. Spec scenario "#104 docs" is only PARTIAL.
2. **On-disk `openspec/specs/recipe-cli/spec.md` missing ADDED delta requirements** — change-folder delta includes `#104 documentation` and `No pin-bump UX`; archived/on-disk recipe-cli was only partially updated (`outdated` forbidden) and still lacks those ADDED sections. Archive step should finish the merge.
3. **Doctor legacy-version check untested** — `doctor.py` `_check_legacy_recipe_versions` is implemented; no dedicated `tests/test_doctor.py` assertion found for `recipe-version` check.
4. **Strict TDD evidence granularity** — apply-progress records phase RED/GREEN, not full per-task TRIANGULATE/SAFETY NET columns.
5. **Review workload** — monolithic branch ~2.2k insertions; chained-PR forecast was High; delivery was authorized as single PR — judgment-day reviewers should expect large diff.

**SUGGESTION**:
1. Tick proposal success-criteria checkboxes after verify/archive acceptance.
2. Refresh stale comments referencing `.internal/resolved-skills` in `tests/test_sync_pipeline.py` (helper already uses cache path).
3. Optional end-to-end sync-agent test asserting hand-authored command wins through fan-out (unit merge already covers precedence).
4. Fixture hygiene: many tests still embed `version=` for legacy paths — fine while intentional; consider a shared helper that omits pins by default to reduce noise.

### Dogfood / fixture consistency

| Item | Status |
|------|--------|
| Dogfood `ai-specs/ai-specs.toml` | ✅ No recipe pins; documents CLI-bound model |
| `templates/ai-specs.toml.tmpl` | ✅ `version=` commented as legacy |
| Test fixtures with `version=` | ✅ Compatible (legacy WARN / catalog reads); suite green |
| Cache helper `_cache_paths.py` | ✅ Shared across materialize/external_dirs tests |

### Verdict

**PASS WITH WARNINGS**

23/23 tasks done, 943/943 tests green, runtime behavior matches the six capability deltas. Non-blocking gaps: #104 cross-doc placement, incomplete on-disk `recipe-cli` ADDED merge, missing doctor legacy unit test, and Strict TDD evidence coarseness. No CRITICAL blockers for judgment-day.

**Next recommended**: judgment-day (parent launches). Optionally fix WARN docs/#104 + recipe-cli on-disk merge + doctor test in a tiny follow-up before or after JD — not required to start JD.
