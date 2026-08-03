# Archive Report

## Change

`2026-08-02-plan-build-delivery-contracts`

## Archive Date

2026-08-02

## Execution Mode

openspec — pre-merge archive-tail on the review branch (`feat/plan-build-delivery-contracts`), per `plan-build-flow` archive-tail. Change-folder close only; canonical spec sync and post-merge close are handled after the PR merges on the base branch.

## Change Summary

The `plan-build-flow` recipe previously declared how a project plans and builds, but not where its planning artifacts should live. This change adds one declarative, per-project contract:

- `artifact_store_default` — the project's default planning-artifact store, default `openspec`, accepted values `openspec | engram | both`, `required = false`, non-empty help text.
- The resolved value is materialized into the generated brief as one appended workflow rule via the existing `provides.brief.workflow_rules` mechanism and `{config.artifact_store_default}` interpolation (no recipe-specific renderer).
- Documentation updated in the recipe README and `docs/recipes-catalog.md`, including the external-runtime boundary.
- Focused contract tests replaced the prior "no config schema" assertion with schema, default/override/absent/invalid-enum, materialization, and negative surface coverage.

The review budget is intentionally excluded: it belongs to external session preflight (follow-ups #59/#60), not to this recipe contract. No gate, library, or generated-output path was touched.

## Verification

- Report read: `openspec/changes/archive/2026-08-02-plan-build-delivery-contracts/verify-report.md`
- Final verdict: **PASS**
- Critical blockers: None
- Both previous blockers resolved: `explore.md` records the amendment to the single `artifact_store_default` contract; the negative recipe-surface test explicitly rejects the three warning-section forms (`tests/test_plan_build_flow_recipe.py:212-215`).
- Focused suite: `python3 -m unittest discover -s tests -p 'test_plan_build_flow_recipe.py'` — 17 tests, OK (0.138s).
- Tasks complete: 22/22 checked; no unchecked implementation task markers (`^\s*- \[ \]`) remain in `tasks.md`.
- Apply state: `all_done`; verification-phase artifacts limited to the verify report itself.

## TDD Evidence Commits

Branch `feat/plan-build-delivery-contracts`, history `development..HEAD` (RED before GREEN):

| Commit | Role |
|---|---|
| `43503cd` | RED — `test(plan-build): add artifact store delivery contract regressions` |
| `ce3ad42` | GREEN — `feat(plan-build): declare artifact store delivery contract` |
| `fc69311` | Triangulation — `test(plan-build): triangulate store defaults and docs` |
| `ecd197e` | Refactor — `refactor(plan-build): centralize materialization assertions` |
| `551bb90` | Negative surface — `test(plan-build): keep removed controls out of test surface` |
| `5740a05` | Evidence — `chore(sdd): record apply completion and task evidence` |
| `4b48d0a` | Eval fix — `test(plan-build): fix delivery live eval setup` |
| `20eea35` | Blocker fix — `test(plan-build): reject token-free budget warning section` |
| `66c0e24` | Blocker fix — `docs(plan-build): reframe removed review budget exploration` |

## Specs Synced

None — pre-merge archive-tail. Canonical sync (`openspec/specs/plan-build-flow/spec.md`) is deferred to the post-merge flow on the base branch; `sync-report.md` is not part of this pre-merge archive per `plan-build-flow`.

## Archive Move

- Source: `openspec/changes/2026-08-02-plan-build-delivery-contracts/`
- Destination: `openspec/changes/archive/2026-08-02-plan-build-delivery-contracts/`
- Method: `git mv` (history preserved), staged together with the previously untracked `verify-report.md`.

## Archive Contents Verified

- `proposal.md`
- `explore.md`
- `specs/plan-build-flow/spec.md`
- `design.md`
- `tasks.md`
- `apply-progress.md`
- `verify-report.md`
- `archive-report.md`

## Delivery Decision

- **Depth:** standard (tasks.md + specs/**/*.md, plus proposal and design).
- **Delivery:** single PR, `single-pr` strategy, no chained PRs.
- **Review workload:** ~120–180 changed lines, within forecast; no `size:exception`.
- **Archive status:** **PASS** — archived on the review branch before merge; branch is PR-ready.
