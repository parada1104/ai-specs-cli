# Tasks: plan-build-evals-v2

Depth: full  
Change: plan-build-evals-v2

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 500–800 |
| 400-line budget risk | High |
| Chained PRs recommended | Yes — consider split if review >400 |
| Suggested split | (A) skill/gate hardening + helper tests; (B) eval harness v2 |
| Delivery strategy | ask-on-risk |

## Phase A — Pre-merge guardian + worktree cleanup

- [ ] **T1** — RED: unit tests for archive/artifact pre-merge helper
      (active change folder → block; archive missing tier files → block;
      archive OK → pass). Helper location TBD under
      `lib/_internal/` or recipe `bin/` (prefer small shared python).
- [ ] **T2** — GREEN: implement helper; wire wording into
      `plan-build-flow` skill §7 merge hard-stop.
- [ ] **T3** — Update all three VCS merge skills
      (`git-merge-workflow`, GitLab, Bitbucket): pre-merge archive hard stop;
      post-merge leave-cwd + script-first cleanup.
- [ ] **T4** — Update `worktree-flow` skill (+ cleanup command docs) for
      leave-cwd + script-first; add/adjust unit tests for cwd-outside-remove
      if script changes.
- [ ] **T5** — Promote spec deltas for `plan-build-flow`, `vcs-pr-flow`,
      `worktree-flow` into `openspec/specs/` during archive (or apply as
      agreed); keep change-folder copies authoritative until then.

## Phase B — Eval harness v2

- [ ] **T6** — RED: smoke tests for `setup_runtime_skills`, model defaults,
      natural-prompt guard (prompt must not contain `/plan`), scenario loader
      with `mode = plan|build`.
- [ ] **T7** — GREEN: multi-runtime harness (`claude`/`opencode`/`pi`/`omp`),
      `EVALS_RUNTIME` / `EVALS_MODEL`, plan-mode invocation, NDJSON/text
      parsing as needed.
- [ ] **T8** — Rewrite AC3 scenario: natural prompt + seeded app + Standard
      assertions (`tasks.md` + `specs/**`); keep forbidden production globs.
- [ ] **T9** — Add stub scenarios AC4 (build after auth) / AC5 (archive) as
      fixtures (live optional); update `tests/evals/README.md` and
      `openspec/specs/recipe-evals/spec.md`.
- [ ] **T10** — Live smoke (manual/opt-in): AC3 with
      `EVALS_LIVE=1 EVALS_RUNTIME=claude EVALS_MODEL=opus` and at least one
      `opencode-go/glm-5.2` or `deepseek-v4-flash` run; record results in
      apply-progress / verify notes.

## Phase C — Validate

- [ ] **T11** — `./tests/run.sh` + `tests/evals/run.sh` (dry) +
      `./tests/validate.sh` green.
- [ ] **T12** — Open PR to `development` only after authorization +
      implementation complete; archive-tail before merge.

## Notes

- Do not implement until human authorizes this plan.
- Preferred live models are a product preference, not a hard schema field.
- If PR exceeds ~400 lines, split Phase A and Phase B into chained PRs.
