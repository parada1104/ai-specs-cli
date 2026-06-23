# Tasks: VCS Drop Deferred Cleanup

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 120–180 |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | Single PR to `development`, 3 commits (1 per item) |
| Delivery strategy | auto-forecast |
| Chain strategy | pending |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: pending
400-line budget risk: Low

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | 3 surgical fixes for items 1–3 from Trello #23 | PR 1 to `development` | 3 commits, TDD pair per item. Strict TDD: RED test first, then GREEN fix, both in same commit per item (or 2 commits per TDD item — design leaves it open, recommend 1 commit per item per user's Trello card). |

## Phase 1: RED tests (TDD step 1)

- [x] 1.1 Add fragment-isolation test in `tests/test_agents_render_brief_fragments.py`: 3 VCS recipes enabled (git/gitlab/bitbucket), 1 bound to `gitlab-mr-flow` → brief contains gitlab fragments, contains NO git or bitbucket fragments.
- [x] 1.2 Add no-binding edge test: 0 VCS bound → no VCS sibling fragments in brief.
- [x] 1.3 Add custom-id edge test: bound to a custom recipe id → that custom recipe can contribute its own fragments if enabled.
- [x] 1.4 Add `GitPrFlowDocsContractTests` in `tests/test_recipes_catalog.py`: mirror of gitlab/bitbucket — assert `catalog/recipes/git-pr-flow/README.md` has no `provider` row in config table; assert `docs/recipes-catalog.md` has no `provider` row for git-pr-flow.
- [x] 1.5 Add unknown-VCS warning test in `tests/test_sync_pipeline.py`: bound id not in `_VCS_RECIPE_LABELS` → assert `⚠ ai-specs:` warning to stderr once per id, and `VCS/PR provider: VCS PR (custom)` bullet in `AGENTS.md`.

## Phase 2: GREEN implementations

- [x] 2.1 Modify `lib/_internal/agents-render.py`: extend `collect_recipe_brief_fragments()` to accept optional `recipe_ids: set[str] | None`; in `_section_workflow_rules()`, pass `allowed_vcs={bindings["vcs-pr-flow"]}` when bound exists.
- [x] 2.2 Modify `lib/_internal/agents-render.py`: in `_section_runtime_flow()` VCS bullet lookup, add a local-set warning de-dupe; if recipe id is not in `_VCS_RECIPE_LABELS`, emit `print(..., file=sys.stderr)` with `⚠ ai-specs: VCS recipe '<id>' is not in the known label set; using generic label 'VCS PR (custom)'` and use `VCS PR (custom)` as the bullet label.
- [x] 2.3 (No code change for item 2 — test only.)

## Phase 3: Verify

- [x] 3.1 Run `./tests/run.sh` from the worktree — must be green.
- [x] 3.2 Run `./tests/validate.sh` — must be green.
- [x] 3.3 Run lint/format per repo conventions (check for pre-commit config).

## Phase 4: Commit & ship

- [x] 4.1 Commit 1 (item 1, TDD pair): `feat(render): isolate VCS workflow_rule fragments to bound recipe` — includes RED test 1.1, 1.2, 1.3 and GREEN impl 2.1.
- [x] 4.2 Commit 2 (item 2): `test(docs-contract): assert git-pr-flow README/catalog omit provider` — includes doc-contract test 1.4.
- [x] 4.3 Commit 3 (item 3, TDD pair): `feat(render): warn on unknown VCS recipe id, use generic label` — includes RED test 1.5 and GREEN impl 2.2.
- [x] 4.4 Push branch `feat/vcs-drop-deferred-cleanup` to origin and open PR to `development` with title `chore(renderer): vcs-drop deferred cleanup (3 items)` referencing Trello #23.
- [x] 4.5 Update Trello card #23: move to In Review, comment with PR URL.
- [ ] 4.6 After PR merge, run `sdd-verify` to produce verify-report marking all 3 items ✅ COMPLIANT.

## Implementation Order

Phase 1 (RED) → Phase 2 (GREEN, in commit order: item 1, item 2, item 3) → Phase 3 (verify all green) → Phase 4 (commit, push, PR, verify-report). The order matters because item 1 + item 3 both modify `lib/_internal/agents-render.py`; keep them in separate commits for cleaner review.
