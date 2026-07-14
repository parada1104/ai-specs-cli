# Proposal: plan-build-evals-v2

## Intent

Make ambient plan/build behavior real and measurable: natural user prompts
drive a depth classifier inside runtime **plan mode**; all planning artifacts
are created there; build runs only after authorization. Harden the pre-merge
guardian (artifacts present + archived on the review branch) and post-merge
worktree cleanup so agents cannot skip them.

## Problem

1. **Eval AC3 is wrong for ambient skills** — the live prompt still says
   "Run the `/plan` command…", which tests the opposite of ambient behavior.
2. **Harness is Claude-only** — prior multi-runtime work was never committed;
   OpenCode/Pi/OMP are first-class dogfood targets.
3. **Gates are soft** — `plan-build-flow` and VCS merge skills *describe*
   pre-merge archive and worktree cleanup, but nothing hard-blocks merge or
   forces cleanup after squash merge when the agent is still inside the
   worktree.

## Scope

### In scope

- Redesign `tests/evals/` for multi-runtime + plan/build mode invocation
- Natural prompts; classifier exercised; artifacts created only in plan mode
- Default live models: Claude `opus`; OpenCode/Pi/OMP via `opencode-go`
  (`glm-5.2` and/or `deepseek-v4-flash`)
- Strengthen pre-merge guardian across `plan-build-flow` + VCS merge skills
  (tier artifacts exist; change folder archived on review branch before merge)
- Harden post-merge worktree cleanup (`worktree-flow` script + VCS skills):
  always leave worktree before remove; prefer cleanup script; fail closed on
  dirty/unmerged
- Keep classifier tiers Full / Standard / Light as today

### Out of scope

- LLM-as-judge transcript layer (AC6 soft checks)
- Nightly CI workflow wiring
- Shipping evals as a catalog recipe
- Changing Light-tier minimum away from tasks-only

## Success criteria

- [ ] Live AC3 (plan mode + natural prompt) creates tier artifacts and does
      not modify production code
- [ ] Harness runs under `EVALS_RUNTIME=claude|opencode|pi|omp` with documented
      model defaults
- [ ] Merge path documents and tests a hard stop when archive/artifacts missing
- [ ] Post-merge cleanup leaves the worktree, runs cleanup script, and removes
      squash-merged worktrees reliably
- [ ] Dry eval smoke + `./tests/validate.sh` green

## Non-goals

- Replacing OpenSpec with another store
- Auto-merging PRs without explicit user approval
