# Proposal: complete D2 recipe overrides-boundary migration (trello-mcp-workflow, worktree-flow)

## Context

`2026-07-23-minimal-project-materialization` (PR #145, archived
`openspec/changes/archive/2026-07-23-minimal-project-materialization/`) made
decision **D2**: "recipes override boundary = `overrides/` only" and shipped
the `.gitignore` pattern in `ai-specs/.gitignore`:

```
.deps/
recipes/**
!recipes/*/
!recipes/*/overrides/
!recipes/*/overrides/**
```

(rendered by `lib/_internal/gitignore-render.py`, tested by
`tests/test_external_dirs.py::test_gitignore_ignores_recipes_except_overrides`).

Its `tasks.md` explicitly listed a companion task that was **never executed**:
"migrate any non-`overrides/` declared overrides (e.g. trello templates)."

This change completes that deferred migration for the two recipes whose
`[[provides.templates]]` declarations still target bare
`ai-specs/recipes/{id}/templates|bin/` paths (currently gitignored and
un-committable after first materialization):

- `trello-mcp-workflow` — 6 card templates (feature, bug, spike, epic, handoff, decision)
- `worktree-flow` — 1 cleanup script (`worktree-cleanup.sh`)

The prior archived change
`2026-07-24-relocate-bundled-commands` (PR #147) is the canonical structural
template for this same-tier "finish a deferred migration in the same domain"
change. Classification there was `domain_change (proposal → design → spec → tasks)`.

## Objective

Relocate the affected `[[provides.templates]]` targets from bare
`ai-specs/recipes/{id}/templates|bin/` paths to
`ai-specs/recipes/{id}/overrides/templates|bin/` paths, so they are correctly
un-ignored by the existing `.gitignore` rule and become committable project
surface (matching the D2 governance model: project-owned override content lives
under `overrides/`, bundled recipe content resolves from the cache).

## Scope — In

- `catalog/recipes/trello-mcp-workflow/recipe.toml` — rewrite the 6
  `[[provides.templates]]` entries' `target` values from
  `ai-specs/recipes/trello-mcp-workflow/templates/card-*.md` to
  `ai-specs/recipes/trello-mcp-workflow/overrides/templates/card-*.md`.
- `catalog/recipes/worktree-flow/recipe.toml` — rewrite the 1
  `[[provides.templates]]` entry's `target` from
  `ai-specs/recipes/worktree-flow/bin/worktree-cleanup.sh` to
  `ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh`.
- Update all hardcoded path references in recipe content (commands, skills,
  README):
  - `catalog/recipes/worktree-flow/README.md` (line 30)
  - `catalog/recipes/worktree-flow/commands/worktree-clean.md` (lines 18, 30)
  - `catalog/recipes/worktree-flow/skills/worktree-flow/SKILL.md` (line 71)
  - `catalog/recipes/gitlab-mr-flow/skills/gitlab-merge-workflow/SKILL.md`
    (line 187)
  - `catalog/recipes/bitbucket-pr-flow/skills/bitbucket-merge-workflow/SKILL.md`
    (line 182)
  - `catalog/recipes/git-pr-flow/skills/git-merge-workflow/SKILL.md` (line 193)
  - `catalog/recipes/trello-mcp-workflow/skills/trello-mcp-workflow/SKILL.md`
    (lines 133, 250)
- Regression test extending `tests/test_external_dirs.py` (or a new test file)
  asserting the new targets are NOT gitignored via `git check-ignore`.
- Sync `openspec/specs/recipe-overrides-runtime/spec.md` and
  `openspec/specs/external-dirs-layout/spec.md` deltas (clarify that
  `[[provides.templates]]` with `condition = "not_exists"` MUST target
  `overrides/` paths to be committable).

## Scope — Out

- `lib/_internal/recipe-materialize.py` — `materialize_template()` writes
  literally to `tpl.target`; no code change needed, only the recipe.toml
  `target` values change. (If backward-compat migration is decided in D2, a
  one-time detector/migrator may be added — see Open Decisions.)
- `lib/_internal/skill-resolution.py` — `_overrides_dir()`,
  `resolve_skill_template()`, `load_skill_config()` are for **skill-provided**
  templates/config (SKILL.md referencing `templates/foo.md`), NOT
  recipe-provided card templates. Already targets `overrides/` correctly.
  Unrelated to this migration; no changes needed.
- `[[provides.docs]]` READMEs with `condition = "not_exists"` (trello-mcp-workflow,
  worktree-flow) — `materialize_doc()` ignores `condition` and always
  overwrites; these are correctly regenerable/bundled content, NOT part of
  this bug. Leave alone. (Note: trello-mcp-workflow's README declares
  `condition = "not_exists"` in the TOML but `materialize_doc` ignores it — a
  separate, minor, pre-existing inconsistency; mention but do not fix unless
  trivial.)
- `catalog/recipes/test-fixture/recipe.toml` — its `[[provides.templates]]`
  entry has `condition = "not_exists"` but targets `docs/test-template-output.md`
  (not under `ai-specs/recipes/`); this is a test fixture, not part of the
  overrides-boundary governance model. Out of scope.
- Migration guidance docs for consumer projects (separate follow-up if needed).

## Open Decisions

- **D1 — New target shape**: confirm `overrides/templates/` and `overrides/bin/`
  (matching the existing pattern for skill overrides under
  `overrides/templates/`). Alternative: keep `templates/` and `bin/` as-is but
  under `overrides/` (proposed). No other shape makes sense — this is the
  exact pattern the D2 governance model was designed for.

- **D2 — Backward compatibility for existing consumer projects**:
  Consumer projects that already ran `ai-specs sync` before this change may
  have materialized files at the OLD paths:
  - `ai-specs/recipes/trello-mcp-workflow/templates/card-*.md` (6 files)
  - `ai-specs/recipes/worktree-flow/bin/worktree-cleanup.sh` (1 file)

  After this change, sync will write to the NEW `overrides/` paths. The old
  files become orphaned leftovers (on-disk, gitignored, never updated again).

  **Option A — No migration, just document**: sync starts writing to new
  paths; old files become orphaned leftovers. Projects can manually delete
  them. Doctor does NOT warn (they're gitignored, invisible to doctor's
  tracked-file checks). Simple, zero-risk, but leaves dead files on disk.

  **Option B — One-time migration in sync**: `recipe-materialize.py` detects
  old-path files and moves them to the new `overrides/` paths before writing
  the new content (preserving any project customizations). Mirrors the
  `remove_bundled_skill_leftovers()` / `remove_bundled_command_leftovers()`
  pattern from `lib/_internal/project-cache.py`. More complex, but cleaner
  for consumers.

  **Option C — Doctor WARN**: add a doctor check that detects old-path files
  and warns the user to manually migrate. Middle ground: no automatic
  migration, but surfaces the issue.

  Prior art: `archive/2026-06-12-trello-card-24/explore.md:104` already
  flagged "existing consumer projects have materialized copies... not_exists
  means a template fix may not overwrite" as a known-deferred rollout concern.
  This change is that rollout.

  **Recommendation**: Option A (no migration, just document) for v1. The old
  files are gitignored and invisible to git; they don't break anything. If
  a consumer project wants to customize the templates, they can copy the
  bundled content from `catalog/recipes/{id}/templates/` to the new
  `overrides/` path manually. Doctor WARN (Option C) can be a follow-up if
  it becomes a support burden.

- **D3 — Doctor WARN for old-path leftovers**: if Option C from D2 is chosen,
  design the detection logic. Likely mirrors `_tracked_bundled_leftovers()`
  in `project-cache.py`: check for specific file paths
  (`ai-specs/recipes/{id}/templates/` and `ai-specs/recipes/{id}/bin/`),
  warn if present. Out of scope for v1 unless D2 chooses Option C.

## Affected Modules

- `catalog/recipes/trello-mcp-workflow/recipe.toml` — 6 target rewrites
- `catalog/recipes/worktree-flow/recipe.toml` — 1 target rewrite
- `catalog/recipes/worktree-flow/README.md` — 1 path reference update
- `catalog/recipes/worktree-flow/commands/worktree-clean.md` — 2 path reference updates
- `catalog/recipes/worktree-flow/skills/worktree-flow/SKILL.md` — 1 path reference update
- `catalog/recipes/gitlab-mr-flow/skills/gitlab-merge-workflow/SKILL.md` — 1 path reference update
- `catalog/recipes/bitbucket-pr-flow/skills/bitbucket-merge-workflow/SKILL.md` — 1 path reference update
- `catalog/recipes/git-pr-flow/skills/git-merge-workflow/SKILL.md` — 1 path reference update
- `catalog/recipes/trello-mcp-workflow/skills/trello-mcp-workflow/SKILL.md` — 2 path reference updates
- `tests/test_external_dirs.py` — add regression test for new targets not gitignored
- `openspec/specs/recipe-overrides-runtime/spec.md` — clarify `[[provides.templates]]` with `condition = "not_exists"` MUST target `overrides/` paths
- `openspec/specs/external-dirs-layout/spec.md` — clarify the override boundary for recipe-provided templates

## Rollback

Additive/reversible: revert the recipe.toml `target` rewrites and the
hardcoded path reference updates. No data migration or git history rewrite
needed. Consumer projects that already synced with the new targets will have
files at `overrides/` paths; reverting to old targets means those files
become orphaned (same situation as before, but in reverse). Zero risk of
data loss.

## Success Criteria

- `catalog/recipes/trello-mcp-workflow/recipe.toml` declares all 6 card
  templates with `target = "ai-specs/recipes/trello-mcp-workflow/overrides/templates/card-*.md"`.
- `catalog/recipes/worktree-flow/recipe.toml` declares the cleanup script with
  `target = "ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh"`.
- All hardcoded path references in recipe content (commands, skills, README)
  point to the new `overrides/` paths.
- `ai-specs sync` on a fresh project materializes the templates/scripts at the
  new `overrides/` paths.
- `git check-ignore` confirms the new `overrides/` paths are NOT ignored
  (committable), while the old bare paths ARE ignored (un-committable).
- Regression test asserts the new targets are not gitignored.
- `openspec/specs/recipe-overrides-runtime/spec.md` and
  `openspec/specs/external-dirs-layout/spec.md` clarify the override boundary
  for recipe-provided templates with `condition = "not_exists"`.
- `./tests/validate.sh` passes.

## Linked Trello Card

https://trello.com/c/tYUPnI4J (#51) — fix(recipes): complete D2 overrides-boundary migration (trello-mcp-workflow, worktree-flow)
