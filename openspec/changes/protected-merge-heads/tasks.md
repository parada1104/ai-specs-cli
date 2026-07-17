# Tasks: protected-merge-heads

Depth: standard

Scope note: same change / worktree / PR #127. Phase A (merge-skill policy) is
done. Phase B (VCS recipe behavior evals for all three providers) is planned
below — **do not implement Phase B until authorized**.

## Phase A — merge-skill policy (done)

- [x] Update `git-merge-workflow`: protected vs feature head cleanup + GitHub `delete_branch_on_merge` preflight
- [x] Update `gitlab-merge-workflow` / `bitbucket-merge-workflow`: same protected-head policy (no GitHub API block)
- [x] Dogfood runtime skill via catalog (`.claude/skills` is gitignored symlink to sync cache)
- [x] README + `docs/recipes-catalog.md` long-lived branch notes
- [x] Golden tests for protected-head policy + `delete_branch_on_merge` needles
- [x] Dogfood: `delete_branch_on_merge=false` on `parada1104/ai-specs-cli`
- [x] `./tests/run.sh` and `./tests/validate.sh` green
- [x] SDD verify PASS + PR #127 opened

## Phase B — VCS capability behavior evals

- [x] Extend harness smoke: load + materialize fixtures for `git-pr-flow`,
      `gitlab-mr-flow`, and `bitbucket-pr-flow` (dry, no LLM)
- [x] Add `tests/evals/scenarios/git-pr-flow/` scenarios with natural prompts:
      - `ac_protected_head_no_delete` — head `development`; agent must not propose
        `--delete-branch` / worktree delete for that head
      - `ac_feature_head_cleanup` — head `feat/…`; agent must propose source-branch
        delete + worktree/local cleanup
      - `ac_delete_branch_on_merge_warn` — seeded setting true; agent must warn and
        cite PATCH remediation without auto-applying
      - `ac_release_head_preferred` — shipping to `main`; prefer `release/v*` head
        over `development`
- [x] Add `tests/evals/scenarios/gitlab-mr-flow/` scenarios mirroring protected /
      feature / release (assert `--remove-source-branch` policy; no GitHub API)
- [x] Add `tests/evals/scenarios/bitbucket-pr-flow/` scenarios mirroring protected /
      feature / release (assert `--close-source-branch` policy; no GitHub API)
- [x] Add `tests/evals/eval_vcs_pr_flow_live.py` (EVALS_LIVE gated) covering all
      three recipe_id scenario trees; reuse N-of-M / runtime selection patterns
- [x] Wire assertions via `ai-specs/eval-notes/merge-plan.md` required/forbidden
      content — no real remote merges
- [x] Update `tests/evals/README.md` second-client table for vcs-pr-flow siblings
- [x] Dry: `tests/evals/run.sh` green offline; unit suite still green
- [x] Add `cursor-agent` as first-class EVALS_RUNTIME (Cursor subscription;
      default `composer-2.5`; skills under `.cursor/skills`)
- [x] Re-verify change (update `verify-report.md`) before merge
