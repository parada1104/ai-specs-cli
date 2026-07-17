## Verification Report

**Change**: protected-merge-heads  
**Mode**: standard (spec + tasks only; no design / proposal)  
**Worktree**: `.worktrees/protected-merge-heads` @ `feat/protected-merge-heads`  
**Base**: `development`  
**Phases**: A (merge-skill policy) + B (VCS behavior evals)  
**Verified**: 2026-07-17 (Phase B re-verify)

### Completeness

| Metric | Value |
|--------|-------|
| Tasks total | 7 Phase A + 9 Phase B |
| Tasks complete | all checked `[x]` after dry green |
| Tasks incomplete | 0 (pending only live LLM runs, opt-in) |

Depth: `standard`. Specs: `specs/vcs-pr-flow/spec.md` + `specs/recipe-evals/spec.md`.

### Build & Tests Execution

**Unit**: `./tests/run.sh` — ✅ 961 passed  

```text
Ran 961 tests in 212.386s
OK
EXIT:0
```

**Evals dry**: `./tests/evals/run.sh` — ✅ 19 tests (5 live skipped without `EVALS_LIVE`)  

```text
Ran 19 tests in 0.101s
OK (skipped=5)
```

Live VCS scenarios are opt-in (`EVALS_LIVE=1`); not required for this verify pass.

### Spec Compliance Matrix

| Requirement | Scenario | Evidence | Result |
|-------------|----------|----------|--------|
| Protected vs feature head cleanup | Protected / feature | Golden skill tests + three provider skills | ✅ |
| GitHub delete_branch_on_merge preflight | Documents check | Golden + dogfood `false` on repo | ✅ |
| Release heads preferred | release/v mentioned | Golden on all three skills | ✅ |
| Behavior evals for all VCS siblings | GitHub protected fixture exists | `tests/evals/scenarios/git-pr-flow/ac_protected_head_no_delete/` + smoke load | ✅ |
| Behavior evals | GitLab/Bitbucket feature fixtures | scenario dirs + smoke | ✅ |
| Live VCS eval module opt-in | skips without EVALS_LIVE | `eval_vcs_pr_flow_live` skipped in dry run | ✅ |
| recipe-evals second client | VCS discoverable + README | smoke `test_vcs_scenario_fixtures_load` + README table | ✅ |

**Compliance summary**: all listed scenarios ✅ COMPLIANT for dry/static gates. Live LLM pass rates ➖ not measured in this verify (opt-in).

### Correctness (Phase B)

| Item | Status | Evidence |
|------|--------|----------|
| `resolve_recipe_skill` for VCS skill ids | ✅ | smoke `test_resolve_vcs_skill_ids` |
| 10 scenario fixtures (4+3+3) | ✅ | `LIVE_SCENARIOS` + natural prompts |
| `eval_vcs_pr_flow_live.py` | ✅ | gated; asserts `merge-plan.md` content |
| `forbidden_content` support | ✅ | plan-build + vcs live modules |
| README second client | ✅ | `tests/evals/README.md` |

### Issues Found

**CRITICAL**: None.  
**WARNING**: None.  
**SUGGESTION**: Run a smoke live subset before release, e.g.  
`EVALS_LIVE=1 EVALS_RUNTIMES=opencode EVALS_SCENARIOS=git-pr-flow/ac_protected_head_no_delete,git-pr-flow/ac_feature_head_cleanup ./tests/evals/run.sh`

### Verdict

**PASS**

Phase A policy + Phase B dry eval harness for all three VCS providers are in place. Unit suite and dry evals green. Live LLM trials remain optional/nightly.

**Next recommended**: commit + push to PR #127; archive before merge when approved.
