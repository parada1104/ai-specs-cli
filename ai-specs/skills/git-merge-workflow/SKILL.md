---
name: git-merge-workflow
description: >
  Unified merge workflow for feature branches created in worktrees.
  Covers: creating a PR via gh CLI, merging, cleaning up the worktree and local branch,
  and syncing the local base branch (development) with the freshly merged remote.
  Trigger: when finishing work on a feature branch or worktree, or when the user says merge.
license: MIT
metadata:
  author: ai-specs
  version: "1.0"
  generatedBy: "1.0.0"
  scope: [root]
  auto_invoke:
    - "Merging a feature branch into development"
    - "Creating a pull request from a worktree"
    - "Cleaning up a worktree after merge"
    - "Finishing work on a feature branch"
    - "Syncing development after a merge"
---

# Git Merge Workflow

## Overview

This skill defines the complete merge-and-cleanup workflow for feature work in the
`ai-specs-cli` repository. It is **independent of SDD** — it works for any change
created in a worktree, whether it followed OpenSpec phases or not.

**Core principle:** Every feature branch ends with a PR via `gh CLI`, a clean merge,
worktree removal, and a synced local `development` branch.

## When to Use

- The user says "merge" or "finish this branch"
- A feature is complete and needs a PR
- After a PR is merged and cleanup is needed
- Before starting new work to ensure `development` is up to date

## Prerequisites

- `gh` CLI installed and authenticated (`gh auth status`)
- Remote `origin` points to GitHub
- Worktree exists under `.worktrees/` (or was used for the branch)
- Base branch is `development` (configurable)

## Workflow Steps

### 1. Push the feature branch

From inside the worktree or with the branch checked out:

```bash
git push origin <branch-name>
```

If the branch does not exist on remote, create it:
```bash
git push -u origin <branch-name>
```

### 2. Create Pull Request via gh CLI

```bash
gh pr create \
  --base development \
  --title "<type>(<scope>): <short description>" \
  --body "$(cat .github/pull_request_template.md 2>/dev/null || echo 'See commit messages for details.')"
```

**Title conventions** (same as commits):
- `feat(recipe): add capability binding`
- `fix(schema): resolve backward compat edge case`
- `docs(readme): update recipe examples`

If the repo uses a PR template, use it. Otherwise provide a minimal body with:
- What changed
- Why
- How to test / verify

### 3. (Optional) Enable auto-merge

If CI passes and no review is required:

```bash
gh pr merge --auto --squash
```

Or wait for review, then merge manually:
```bash
gh pr merge --squash
```

### 4. Cleanup after merge

**Remove the worktree:**
```bash
git worktree remove .worktrees/<branch-name>
```

If the worktree is already gone (e.g. was removed manually), verify:
```bash
git worktree list
```

**Delete the local branch:**
```bash
git branch -d <branch-name>
```

If the branch was not fully merged (should not happen after a successful PR merge),
use `-D` instead, but warn the user.

### 5. Sync local development

**Checkout development and fast-forward from origin:**
```bash
git checkout development
git pull --ff-only origin development
```

If fast-forward is not possible, report the divergence and stop:
```bash
git pull --ff-only origin development || echo "Divergence detected — manual merge required"
```

## Guardrails

- **Never merge locally with `git merge`** — always go through `gh pr create` + `gh pr merge`.
- **Never delete a worktree without verifying the PR is merged** — check `gh pr view --json state`.
- **Never skip `git pull --ff-only`** — the local `development` must reflect the freshly merged remote.
- **If `gh` is not installed or not authenticated**, stop and ask the user to run `gh auth login`.

## Anti-patterns

- Running `git merge --no-ff` locally (bypasses PR/review/CI)
- Deleting the worktree before the PR is merged (loses uncommitted work)
- Leaving stale branches or worktrees after merge (repo clutter)
- Forgetting to pull `development` after merge (next worktree starts from stale base)

## Complete Workflow Example

```bash
# Inside the worktree, feature is done
git push origin feat/my-feature

# Create PR
gh pr create --base development --title "feat(module): description"

# Merge (after CI/review)
gh pr merge --squash

# Cleanup
cd ../..
git worktree remove .worktrees/feat/my-feature
git branch -d feat/my-feature

# Sync development
git checkout development
git pull --ff-only origin development
```

## Related Skills

- `openspec-sdd-workflow` — SDD phase orchestration (creates worktrees)
- `openspec-phase-orchestrator` — phase-by-phase SDD execution
- `using-git-worktrees` — worktree creation and isolation
