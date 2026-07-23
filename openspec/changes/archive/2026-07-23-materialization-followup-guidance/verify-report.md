# Verify report: materialization-followup-guidance

**Change**: `2026-07-23-materialization-followup-guidance`  
**Branch**: `change/materialization-followup-guidance` @ `578c597`  
**Trello**: #49 https://trello.com/c/AfRD6P6O  
**Depth**: standard  
**Verified**: 2026-07-23

## Commands

| Layer | Command | Result |
|-------|---------|--------|
| Focused | harness literacy + doctor tracked leftover + project_cache | PASS |
| Full | `./tests/validate.sh` | PASS — 1023 tests, EXIT 0 |

## AC map

| AC | Evidence | Result |
|----|----------|--------|
| Pointer omits `ai-specs/skills/` | `test_agents_render_emits_harness_literacy_pointer` | ✅ |
| harness-lifecycle cache flatten, no `.new` | `test_harness_lifecycle_documents_cache_flatten` | ✅ |
| doctor WARN + `git rm --cached`; no index mutate | `test_tracked_bundled_leftover_warns_without_git_rm` | ✅ |
| refresh/sync prints remediation | `test_refresh_prints_tracked_leftover_remediation` | ✅ |

## Verdict

**PASS**

**Next**: archive on branch → PR → development. Dogfood manual: `git rm -r --cached` for deleted skill-creator/skill-sync + optional `[brief].context_sources` prose tweak.
