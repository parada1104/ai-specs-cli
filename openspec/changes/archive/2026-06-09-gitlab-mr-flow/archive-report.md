# Archive Report: gitlab-mr-flow

**Archived**: 2026-06-09
**Source**: `openspec/changes/gitlab-mr-flow/` → `openspec/changes/archive/2026-06-09-gitlab-mr-flow/`
**MRs**: #81 (PR 1), #82 (PR 2), #83 (PR 3), #84 (final merge)
**Base branch**: `development`

---

## Executive Summary

Built `gitlab-mr-flow` — a sibling provider recipe to `git-pr-flow` that delivers the same `vcs-pr-flow` capability for GitLab repositories. The recipe provides a bundled `gitlab-merge-workflow` skill, an `/mr-create` slash command, and comprehensive documentation. Implementation followed strict TDD across 3 chained PRs, producing 14 defined tasks, 50+ new tests across 2 test files, and 4 rounds of Judgment Day review that fixed 11 issues.

## What Was Built

| Component | Description |
|-----------|-------------|
| `catalog/recipes/gitlab-mr-flow/recipe.toml` | Recipe manifest with `vcs-pr-flow`, `provider=gitlab`, `base_branch=development`, validate hook, skill/command/docs provisions |
| `catalog/recipes/gitlab-mr-flow/skills/gitlab-merge-workflow/SKILL.md` | GitLab MR workflow with `glab` preflight checks, explicit push-before-create, approval-gated merge, blocker messaging |
| `catalog/recipes/gitlab-mr-flow/commands/mr-create.md` | Thin slash command: reads config, checks `glab`, pushes explicitly, creates MR, stops after MR URL |
| `catalog/recipes/gitlab-mr-flow/README.md` | Enablement docs, config reference, explicit binding TOML, `glab` prerequisites, safety policy |
| `docs/recipes-catalog.md` (modified) | At-a-glance row + full `## gitlab-mr-flow` section with config, TOML example, cross-link |
| `docs/capabilities.md` (modified) | Lists `gitlab-mr-flow` as `vcs-pr-flow` provider alongside `git-pr-flow` |
| `tests/test_gitlab_mr_flow_recipe.py` | 27 unit/golden tests: manifest, materialization, binding, golden content assertions |
| `tests/test_recipes_catalog.py` (modified) | 15 new docs contract tests for README, catalog, and capabilities |

## Files Added/Modified

**Created** (new files under `catalog/recipes/gitlab-mr-flow/`):
- `recipe.toml` — recipe manifest
- `skills/gitlab-merge-workflow/SKILL.md` — GitLab workflow skill
- `commands/mr-create.md` — slash command
- `README.md` — user-facing documentation
- `tests/test_gitlab_mr_flow_recipe.py` — recipe test suite (27 tests)

**Modified** (pre-existing files):
- `docs/recipes-catalog.md` — catalog entry and section
- `docs/capabilities.md` — provider listing
- `tests/test_recipes_catalog.py` — 15 new docs contract tests

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Provider model | New sibling recipe (not refactoring `git-pr-flow`) | Existing `[[bindings]]` model supports provider swapping; separate IDs avoid primitive conflicts |
| MR command sequence | Explicit `git push -u origin` before `glab mr create` | Preserves GitHub safety model; `--fill` can push implicitly (forbidden by spec) |
| Error handling | Stop before push on `glab` missing/unauthenticated | Avoids hidden side effects; keeps failures actionable |
| Config validation | Existing shared `validate-config` hook (no recipe-local script) | Only validates manifest shape — runtime auth belongs in skill/command |
| Command args | `/mr-create [title] [description]` only; no `--base`, `--fill`, `--merge` | Provider/base come from config; merge automation is out of scope |

## Test Coverage

| Suite | Tests | Status |
|-------|-------|--------|
| `tests/test_gitlab_mr_flow_recipe.py` — Manifest & materialization | 13 | ✅ All passing |
| `tests/test_gitlab_mr_flow_recipe.py` — Golden content | 14 | ✅ All passing |
| `tests/test_recipes_catalog.py` — Docs contracts | 15 | ✅ All passing |
| Full regression suite | 593 | ✅ All passing (0 failed, 0 skipped) |
| **Total new tests** | **27 from recipe + 15 from docs = 42 new** | |

Additional 8 golden content tests added during Judgment Day remediation, bringing total new tests to **50**.

## Judgment Day Results

- **4 rounds** of blind dual review conducted
- **11 issues** identified and fixed
- Issue categories: golden test coverage gaps, assertion quality, spec compliance, cross-PR consistency
- Key remediation: added 3-5 focused golden assertions for exact blocker text, preflight ordering, STOP/report-MR-URL, and explicit-approval wording

## Known Limitations

- `glab` install/auth checks are runtime-only — `validate-config` does not verify tooling
- Dual-provider (`git-pr-flow` + `gitlab-mr-flow`) setups require explicit `[[bindings]]` or `vcs-pr-flow` stays unbound
- `coverage` tool not installed — changed-file coverage analysis is unavailable
- Renderer does not emit `(glab CLI)` suffix for `provider = "gitlab"` (GitLab clarity from recipe prose)
- `base_branch` defaults to `development` (intentionally differs from `git-pr-flow`'s `main`)

## Stale Checkbox Reconciliation

The `openspec/changes/gitlab-mr-flow/tasks.md` file contained `[ ]` (unchecked) markers for all 14 tasks despite the work being fully completed. `sdd-apply` updated the Engram `apply-progress` observation (#790) correctly but did not persist checkbox state to the OpenSpec tasks file. The archive agent reconciled this at archive time, using `apply-progress` (#790) and verify reports (PR1 #791, PR2 #794, PR3 #797) as proof of completion. All tasks are now marked `[x]` in the archived artifact.

## Engram Observation IDs

| Artifact | Engram ID |
|----------|-----------|
| Explore | #782 |
| Proposal | #784 |
| Spec | #785 |
| Design | #787 |
| Tasks | #789 |
| Apply Progress | #790 |
| Verify Report (PR1) | #791 |
| Verify Report (PR2) | #794 |
| Verify Report (PR3) | #797 |
| Archive Report | (this save) |

## SDD Cycle Complete

The change has been fully planned, explored, proposed, specified, designed, implemented, verified (with Judgment Day), and archived. Ready for the next change.
