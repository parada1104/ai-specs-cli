## Verification Report

**Change**: protected-merge-heads  
**Mode**: standard (spec + tasks only; no design / proposal)  
**Worktree**: `.worktrees/protected-merge-heads` @ `feat/protected-merge-heads`  
**Base**: `development`  
**Implementation**: uncommitted at verify (working tree); HEAD `6f2fae5`  
**Verified**: 2026-07-17

### Completeness

| Metric | Value |
|--------|-------|
| Tasks total | 7 |
| Tasks complete | 7 |
| Tasks incomplete | 0 |

All checkboxes in `tasks.md` are `[x]`. Depth line reads `standard`; no `design.md` / `proposal.md` required for this tier.

### Build & Tests Execution

**Command**: `./tests/run.sh` (unit suite) — re-run at verify in the worktree.

**Tests**: ✅ 961 passed / ❌ 0 failed / ⚠️ 0 skipped

```text
Ran 961 tests in 188.532s

OK
EXIT:0
```

Focused golden re-check at verify (7 tests) also OK covering protected-head + `delete_branch_on_merge` needles across git/gitlab/bitbucket.

`./tests/validate.sh` was green during apply (961 OK); not separately re-run at verify. Not a blocker.

**Coverage**: ➖ Not available (no coverage tool in project capabilities).

### Spec Compliance Matrix

| Requirement | Scenario | Test / static evidence | Result |
|-------------|----------|------------------------|--------|
| Protected vs feature head cleanup | Protected head skips source-branch delete | Golden: `test_skill_classifies_protected_heads`, gitlab `test_skill_merge_removes_source_branch`, bitbucket `test_skill_merge_closes_source_branch` assert `Head branch class`, `development`/`staging`, and `never pass --delete-branch` / `--remove-source-branch` / `--close-source-branch`. Static: all three skills document protected merge without delete flags and skip worktree/`branch -D` for protected heads. | ✅ COMPLIANT |
| Protected vs feature head cleanup | Feature head still cleans up | Golden: `test_skill_requires_post_merge_branch_cleanup` (`git branch -D`, `git worktree remove`, `git push origin --delete`); gitlab/bitbucket still assert provider delete flags for feature path. Static: feature merge lines keep `--delete-branch` / `--remove-source-branch` / `--close-source-branch` plus worktree cleanup. | ✅ COMPLIANT |
| GitHub delete_branch_on_merge preflight | git-merge-workflow documents check | `test_skill_preflight_checks_delete_branch_on_merge` asserts `delete_branch_on_merge` + `gh api -X PATCH repos/$REPO -f delete_branch_on_merge=false`. Static: `git-merge-workflow` Runtime Preflight block + “Do **not** auto-PATCH”. GitLab/Bitbucket document UI delete/close off for protected heads (no GitHub API). Dogfood: `gh api … delete_branch_on_merge` → `false` on this repo. | ✅ COMPLIANT |
| Release heads preferred over long-lived heads into main | Release convention mentioned | `test_skill_prefers_release_head_for_main` + gitlab/bitbucket golden assert `release/v`. Static: all three skills prefer `release/vX.Y.Z` → `main`. | ✅ COMPLIANT |

**Compliance summary**: 4/4 scenarios ✅ COMPLIANT · 0 PARTIAL · 0 GAP (3 requirements, all covered).

### Correctness (Static Evidence)

| Item | Status | Evidence |
|------|--------|----------|
| Protected head set includes defaults + config | ✅ | Skills list `main`/`master`/`development`/`staging` + `base_branch` + `integration_branch` |
| Conditional provider delete flags | ✅ | Feature vs protected merge command pairs in all three SKILL.md files |
| GitHub preflight human-gated | ✅ | Warn + PATCH remediation; explicit no auto-PATCH |
| Docs/README long-lived notes | ✅ | VCS recipe READMEs + `docs/recipes-catalog.md` git-pr-flow blurb |
| Dogfood repo setting | ✅ | Live `delete_branch_on_merge=false` on `parada1104/ai-specs-cli` |

### Coherence

- **Design coherence**: ➖ N/A — standard tier, no `design.md` / `proposal.md`.
- **tasks ↔ spec ↔ code alignment**: ✅ Consistent. Catalog skills are source of truth (`.claude/skills` is gitignored sync symlink — task notes that correctly).
- Diff scoped to VCS recipes, docs, tests, and openspec change folder.

### Issues Found

**CRITICAL**: None.

**WARNING**: None.

**SUGGESTION**:
1. Evidence is golden text / static skill content (appropriate for LLM-facing skills). Optional later: a tiny helper that classifies head names in Python if cleanup ever becomes a script.
2. Re-run `./tests/validate.sh` once more before merge if desired; apply already passed it.

### Verdict

**PASS**

7/7 tasks complete, 961/961 unit tests green at verify, and every spec requirement/scenario (4/4) maps to golden tests plus corroborating skill/README evidence. Harness policy lives in VCS recipes; this repo dogfoods `delete_branch_on_merge=false`.

**Next recommended**: commit planning + implementation, open PR to `development`.
