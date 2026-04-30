## Verification Report: motor-restructurar-dirs-externos

### Summary

| Dimension    | Status                          |
|--------------|---------------------------------|
| Completeness | 35/35 tasks, 5 spec domains     |
| Correctness  | PASS                            |
| Coherence    | PASS                            |

### Test Results

- `python3 -m unittest tests.test_external_dirs`: **30/30 tests passing**, exit 0
- `python3 -m unittest tests.test_openspec_sdd_workflow_skill`: **2/2 tests passing**, exit 0
- `./tests/run.sh`: **183/183 tests passing**, exit 0
- `./tests/validate.sh`: **183/183 tests passing**, exit 0

### Completeness

All 35 tasks from `tasks.md` are marked complete and remain backed by implementation and regression coverage.

| Spec Domain | Requirements | Status |
|-------------|--------------|--------|
| `external-dirs-layout` | 5 requirements, 8 scenarios | Implemented and tested |
| `recipe-conflict-resolution` | 1 modified + 1 added requirement, 8 scenarios | Implemented and tested |
| `recipe-overrides-runtime` | 4 requirements, 5 scenarios | Implemented and tested |
| `recipe-sync-materialization` | 1 modified + 2 added requirements, 5 scenarios | Implemented and tested |
| `skill-source-precedence` | 4 requirements, 5 scenarios | Implemented and tested |

### Correctness

- `lib/init.sh` creates `.recipe/` and `.deps/` at project root.
- `templates/gitignore-root.tmpl` adds `.recipe/`, `.deps/`, and `.recipe/*/overrides/` to root `.gitignore`.
- `lib/_internal/vendor-skills.py` clones `[[deps]]` skills into `.deps/{dep-id}/skills/{skill-id}/`.
- `lib/_internal/recipe-materialize.py` materializes bundled skills to `.recipe/{recipe-id}/skills/{skill-id}/` and dep skills to `.deps/{dep-id}/skills/{skill-id}/`.
- `lib/_internal/recipe-materialize.py` warns before overwriting an existing non-identical command in `ai-specs/commands/`.
- `lib/_internal/skill-resolution.py` implements local > recipe > dep precedence without file-level fallback across sources.
- `lib/_internal/skill-resolution.py` emits first-seen warnings within recipe and dep tiers.
- `lib/_internal/skill-resolution.py` raises `RuntimeError` for missing skills.
- `lib/_internal/skill-resolution.py` implements recipe-scoped override config and template resolution.
- `lib/sync-agent.sh` uses `flatten-resolved-skills.py` to flatten multi-source resolution into `.resolved-skills/` for agent consumption.
- `lib/_internal/agents-md-render.py` consumes resolved skill dictionaries for root workspace rendering.
- `lib/_internal/doctor.py` accepts `.resolved-skills/` as a valid symlink target.
- Orphan cleanup removes unreferenced `.recipe/{id}` and `.deps/{id}` directories.
- Re-sync idempotency remains covered by `ResyncIdempotencyTests.test_sync_is_idempotent`.

### Warning Resolution

The prior verification warnings are resolved:

1. `recipe-conflict-resolution` command overwrite divergence: resolved by warning in `materialize_command` plus `test_warns_when_recipe_command_overwrites_existing_command`.
2. `recipe-sync-materialization` dep skill gap: resolved by `test_materializes_recipe_dep_skill_to_deps_dir`.
3. `skill-source-precedence` no-backfill gap: resolved by `test_local_precedence_does_not_backfill_files_from_recipe`.

### Post-Rebase Note

After rebasing onto `origin/development`, the existing SDD workflow skill contract tests failed because the local skill docs no longer contained the required contract wording. The branch restores the tested wording in `ai-specs/skills/openspec-phase-orchestrator/SKILL.md` and `ai-specs/skills/openspec-sdd-workflow/SKILL.md` without running `ai-specs sync`.

### Issues by Priority

#### CRITICAL

- None

#### WARNING

- None

### Final Assessment

All tasks and spec scenarios are implemented with focused and full validation passing. The local worktree is ready for commit and review; archive should wait until the corrected implementation is actually integrated into `development`.

**Verdict: PASS.**
