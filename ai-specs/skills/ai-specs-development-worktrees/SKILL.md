---
name: ai-specs-development-worktrees
description: >
  Enforces the repository workflow where `main` is no longer the day-to-day
  integration branch, `development` is the base branch for ongoing work, and
  every implementation starts from a dedicated worktree created off
  `development`. Trigger: when starting work from `development`, planning the
  repository workflow, or deciding where code changes should start.
license: Apache-2.0
metadata:
  author: parada1104
  version: "1.0.1"
  scope: [root]
  auto_invoke:
    - "Starting work from development"
    - "Deciding where implementation should begin"
allowed-tools: Read, Edit, Write, Glob, Grep, Bash
---

## When to Use

- When a task will change code and needs a working branch
- When deciding whether to work on `main`, `development`, or a feature branch
- When creating or cleaning up git worktrees
- When onboarding collaborators to the repository workflow

## Critical Patterns

- **`main` is protected by convention**: treat it as the stable line. Do not start day-to-day feature work from `main`.
- **`development` is the integration base**: create it if missing, keep it updated, and branch new work from there.
- **Every implementation starts in a worktree**: one worktree per feature/fix/experiment.
- **Worktrees live under `.worktrees/` at the repo root** (ignored by git): use that path—not a sibling directory like `../worktrees/`—so any agent or collaborator opening the project finds the same convention.
- **Worktrees branch from `development`** unless the task is an explicit hotfix strategy agreed by the maintainer.
- **Do not pile unrelated tasks into one worktree**. One branch, one intent.
- Before creating a new worktree, ensure the base branch is clean and up to date.

## Standard Flow

1. Ensure `main` is clean and pushed.
2. Ensure `development` exists and is pushed.
3. Update `development` locally.
4. Create a feature branch from `development`.
5. Create a dedicated worktree for that branch.
6. Do all implementation work inside that worktree.

## Related docs

- OpenSpec apply commit grouping (phase-aligned commits, then archive):
  [`docs/ai/openspec-apply-commits.md`](../../../docs/ai/openspec-apply-commits.md)

## Commands

```bash
# create development once (from current main)
git checkout main
git pull --ff-only origin main
git checkout -b development
git push -u origin development

# keep development updated
git checkout development
git pull --ff-only origin development

# create a feature worktree from development (run from repo root: git rev-parse --show-toplevel)
git checkout development
git pull --ff-only origin development
mkdir -p .worktrees
git worktree add .worktrees/<branch-name> -b <branch-name> development

# list active worktrees
git worktree list

# remove a finished worktree
git worktree remove .worktrees/<branch-name>
git branch -d <branch-name>
```

## Anti-patterns

- Putting worktrees outside the repo (e.g. `../worktrees/...`) when the project already standardizes on `.worktrees/`
- Starting feature work directly on `main`
- Creating a feature branch from stale local state
- Reusing one worktree for multiple unrelated tasks
- Forgetting that the branch base must be `development`
- Deleting a worktree without verifying whether the branch is still needed
