# Tasks: complete D2 recipe overrides-boundary migration

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 120–220 |
| 400-line budget risk | Low |
| Session review budget | 900 |
| Chained PRs recommended | No |
| Suggested split | single PR |
| Delivery strategy | single-pr |
| Chain strategy | size-exception |

```text
Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: size-exception
400-line budget risk: Low
```

Rationale: pure declaration + catalog prose relocation (7 `target` strings, 9
path substitutions across 7 files), one regression test (~40–60 LOC), one
cheap catalog-lint (~30–50 LOC), and a CHANGELOG bullet. **Zero production
Python/shell edits.** Well under both the classic 400-line and this session's
900-line review budgets.

## Planning depth

- **Classification**: domain_change (proposal → design → spec → tasks). Same
  tier as `archive/2026-07-24-relocate-bundled-commands`: finishes a deferred
  migration in the D2 overrides-boundary domain already shipped by
  `2026-07-23-minimal-project-materialization`.
- **Accepted baselines (Option A)**:
  - **D1**: new targets nest under `overrides/templates/` and `overrides/bin/`.
  - **D2 Option A**: no leftover migration of old bare paths; no doctor WARN.
  - **Docs channel**: do **not** touch any `[[provides.docs]]` block
    (including trello's dead `condition = "not_exists"` on README).
  - **Materializer**: do **not** edit `lib/_internal/recipe-materialize.py`
    (`materialize_template` already writes literal `tpl.target`).
- **Authorization**: baselines above are the working plan; final maintainer
  gate still applies before apply. No open design forks remain for the apply
  agent.

## Non-goals (apply MUST NOT)

- Edit `lib/_internal/recipe-materialize.py`, `skill-resolution.py`,
  `doctor.py`, `gitignore-render.py`, or any other production code.
- Migrate or delete old bare-path leftovers on consumer disks.
- Add doctor WARN for old-path leftovers (Option C is a documented follow-up).
- Touch any `[[provides.docs]]` / `materialize_doc` overwrite channel
  (README targets stay bare `ai-specs/recipes/{id}/README.md`).
- Touch `catalog/recipes/test-fixture/recipe.toml` (targets outside
  `ai-specs/recipes/`).
- Edit `proposal.md`, `design.md`, or `specs/` during apply.

## Implementation (red-green-refactor)

### Phase 1 — RED: gitignore boundary regression (`git check-ignore`)

Covers spec scenarios:

- `external-dirs-layout` → "Gitignore allows overrides templates but not bare
  recipes paths"
- `external-dirs-layout` → "Override boundary covers all conditional template
  sub-paths"
- `recipe-overrides-runtime` → "Conditional template targets overrides path" /
  "… bare path (non-committable)" / "Bin script target under overrides"

- [ ] 1.1 RED: in `tests/test_external_dirs.py` (`InitExternalDirsTests`), add
      `test_gitignore_committable_relocated_recipe_templates` mirroring
      `test_gitignore_ignores_recipes_except_overrides` (temp project →
      `git init` → `ai-specs init --no-tui` → create files on disk →
      `ignored(rel)` via `git check-ignore -q`).
- [ ] 1.2 RED assertions (all required):
      - `ai-specs/recipes/trello-mcp-workflow/overrides/templates/card-feature.md`
        is **NOT** ignored (representative of the 6 card templates).
      - `ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh`
        is **NOT** ignored.
      - OLD bare paths
        `ai-specs/recipes/trello-mcp-workflow/templates/card-feature.md` and
        `ai-specs/recipes/worktree-flow/bin/worktree-cleanup.sh`
        **ARE** ignored (negative boundary guard — not a tautology).
- [ ] 1.3 Confirm the new test fails for the right reason before any catalog
      rewrite only if a later phase couples catalog parse into this test; the
      gitignore rule already ships, so this test is expected **green once
      written** against current `.gitignore`. Treat it as the permanent
      regression gate that fails if a future change drops the
      `!recipes/*/overrides/**` negation or moves targets back out of
      `overrides/`.

### Phase 2 — RED: catalog-lint for conditional template targets

Lightweight belt-and-suspenders guard sanctioned by design.md (not a new
materializer behavior test — generic `materialize_template` + `not_exists` is
already covered by `tests/test_recipe_materialize.py::test_materializes_template_not_exists`
/ `test_skips_template_when_target_exists`).

- [ ] 2.1 RED: add a focused test (prefer extending
      `tests/test_external_dirs.py` or a small catalog assertion in an existing
      recipe/catalog test module — do not invent a new production validator)
      that parses every non-fixture `catalog/recipes/*/recipe.toml` and asserts:
      - every `[[provides.templates]]` entry with `condition == "not_exists"`
        whose `target` starts with `ai-specs/recipes/` contains
        `/overrides/` in the target path;
      - specifically, the seven known targets resolve to:
        - `ai-specs/recipes/trello-mcp-workflow/overrides/templates/card-{feature,bug,spike,epic,handoff,decision}.md`
        - `ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh`
      - `catalog/recipes/test-fixture` remains excluded or allowed (its target
        is outside `ai-specs/recipes/`).
- [ ] 2.2 Run the new catalog-lint test alone and confirm **RED** against
      current catalog (bare `templates/` / `bin/` targets still present).

### Phase 3 — GREEN: recipe.toml target rewrites (7)

- [ ] 3.1 GREEN: rewrite the 6 `target` values only in
      `catalog/recipes/trello-mcp-workflow/recipe.toml` (lines ~103–131).
      Leave `source`, `condition`, and every other field untouched.

  | `source` (unchanged) | new `target` |
  |---|---|
  | `templates/card-feature.md` | `ai-specs/recipes/trello-mcp-workflow/overrides/templates/card-feature.md` |
  | `templates/card-bug.md` | `…/overrides/templates/card-bug.md` |
  | `templates/card-spike.md` | `…/overrides/templates/card-spike.md` |
  | `templates/card-epic.md` | `…/overrides/templates/card-epic.md` |
  | `templates/card-handoff.md` | `…/overrides/templates/card-handoff.md` |
  | `templates/card-decision.md` | `…/overrides/templates/card-decision.md` |

- [ ] 3.2 GREEN: rewrite the 1 `target` in
      `catalog/recipes/worktree-flow/recipe.toml` (lines ~84–87):
      - old: `ai-specs/recipes/worktree-flow/bin/worktree-cleanup.sh`
      - new: `ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh`
      - `source` stays `templates/worktree-cleanup.sh` (catalog layout unchanged).
- [ ] 3.3 Explicit non-touch: leave both recipes' `[[provides.docs]]` README
      blocks (including trello's `condition = "not_exists"` on the docs block)
      exactly as-is.
- [ ] 3.4 Re-run Phase 2 catalog-lint → **GREEN**.

### Phase 4 — GREEN: hardcoded path-reference substitutions (9 / 7 files)

Literal string replace: insert `overrides/` after `{id}/` for the paths below.
Do not rewrite unrelated prose.

| File | Old substring | New substring |
|---|---|---|
| `catalog/recipes/worktree-flow/README.md` | `ai-specs/recipes/worktree-flow/bin/worktree-cleanup.sh` | `…/overrides/bin/worktree-cleanup.sh` |
| `catalog/recipes/worktree-flow/commands/worktree-clean.md` (2×) | same | same |
| `catalog/recipes/worktree-flow/skills/worktree-flow/SKILL.md` | same | same |
| `catalog/recipes/gitlab-mr-flow/skills/gitlab-merge-workflow/SKILL.md` | same | same |
| `catalog/recipes/bitbucket-pr-flow/skills/bitbucket-merge-workflow/SKILL.md` | same | same |
| `catalog/recipes/git-pr-flow/skills/git-merge-workflow/SKILL.md` | same | same |
| `catalog/recipes/trello-mcp-workflow/skills/trello-mcp-workflow/SKILL.md` (2×) | `ai-specs/recipes/trello-mcp-workflow/templates/` | `…/overrides/templates/` |

- [ ] 4.1 Apply all 9 substitutions across the 7 files above.
- [ ] 4.2 Re-grep catalog-wide and assert **zero** residual matches for:
      - `recipes/worktree-flow/bin/` (non-`overrides`)
      - `recipes/trello-mcp-workflow/templates/` (non-`overrides`)
      Allow hits only inside this change's `openspec/` artifacts if any; catalog
      production content must be clean.
- [ ] 4.3 Confirm cross-recipe merge-workflow skills (gitlab/bitbucket/git-pr)
      now point at `worktree-flow/overrides/bin/worktree-cleanup.sh`.

### Phase 5 — TRIANGULATE: materialize + ignore smoke (no production code)

Design: zero materializer edits; keep smoke thin and data-driven.

- [ ] 5.1 Optional focused smoke (only if cheap with existing fixtures): enable
      `trello-mcp-workflow` and/or `worktree-flow` in a temp project, run
      materialize/sync path already used by `tests/test_recipe_materialize.py`,
      and assert files land under the new `overrides/` targets and that
      `not_exists` skips on second run. Prefer extending an existing materialize
      test helper over a large new fixture. Skip if catalog wiring cost exceeds
      ~30 LOC — Phase 1 + Phase 2 already pin the contract.
- [ ] 5.2 Re-run Phase 1 `git check-ignore` test → still **GREEN**.
- [ ] 5.3 Do **not** add doctor checks, leftover migrators, or README migration
      sections (Option A).

### Phase 6 — Docs: CHANGELOG only

- [ ] 6.1 Under `CHANGELOG.md` → `## [Unreleased]` → `### Fixed`, add the
      design-drafted bullet (wording may be tightened, meaning must match):

      > - **Recipe override boundary completed for trello/worktree templates**:
      >   `trello-mcp-workflow` card templates and the `worktree-flow` cleanup
      >   script now materialize under `ai-specs/recipes/{id}/overrides/` so they
      >   are committable (previously written to gitignored `templates/`/`bin/`
      >   paths). Projects synced before this release may delete the orphaned
      >   old-path copies (`ai-specs/recipes/trello-mcp-workflow/templates/`,
      >   `ai-specs/recipes/worktree-flow/bin/`); they are gitignored and no
      >   longer used.

- [ ] 6.2 No README product-doc section, no doctor-adjacent mention, no consumer
      migration guide (design: CHANGELOG is the single upgrade touchpoint).

## Validation

- [ ] 7.1 `./tests/run.sh tests.test_external_dirs` (or equivalent focused
      invocation) green after Phase 1–2 land with Phase 3 fixes.
- [ ] 7.2 Full `./tests/validate.sh` exit 0 before verify (apply phase; tasks
      author does not run the suite in this pass).
- [ ] 7.3 Catalog residual grep (Phase 4.2) clean.
- [ ] 7.4 Diff audit: **no** production code under `lib/`, `bin/`, or
      `templates/` changed; only `catalog/recipes/**`, `tests/test_external_dirs.py`
      (and/or one small catalog-lint addition), and `CHANGELOG.md`.
- [ ] 7.5 Spec scenario checklist for verify-report (post-apply):
      - [ ] recipe-overrides-runtime: Conditional template targets overrides path
      - [ ] recipe-overrides-runtime: Conditional template targets bare path
      - [ ] recipe-overrides-runtime: Bin script target under overrides
      - [ ] external-dirs-layout: Gitignore allows overrides templates but not bare paths
      - [ ] external-dirs-layout: Override boundary covers all conditional sub-paths
      - [ ] external-dirs-layout: Non-conditional template targets unaffected
      - [ ] Explicit: `[[provides.docs]]` / README materialize_doc channel untouched
- [ ] 7.6 Promote spec deltas into
      `openspec/specs/{recipe-overrides-runtime,external-dirs-layout}/spec.md`
      at archive (not during apply).

## Rollback

Revert the 7 `target` rewrites, 9 content substitutions, test additions, and
CHANGELOG bullet. No data migration, no git-history rewrite. Symmetric with
Option A leftover situation in either direction.
