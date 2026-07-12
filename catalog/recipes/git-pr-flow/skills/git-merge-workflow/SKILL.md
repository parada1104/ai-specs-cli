[SKILL.md#643A]
---
name: git-merge-workflow
description: >
  Provider-oriented merge workflow for feature branches created in worktrees.
  Uses the configured base branch from [recipes.git-pr-flow.config]
  (base_branch). GitHub via the gh CLI.
license: MIT
metadata:
  author: ai-specs
  version: "2.0"
  generatedBy: "manual-runtime"
  scope: [root]
  auto_invoke:
    - "Merging a feature branch into development"
    - "Creating a pull request from a worktree"
    - "Cleaning up a worktree after merge"
    - "Finishing work on a feature branch"
    - "Syncing development after a merge"
---

# Git Merge Workflow

Use this skill only when the user explicitly asks to create a PR, merge, finish a branch, or clean up after merge.

Use the configured base branch from `[recipes.git-pr-flow.config]` (`base_branch`).
This recipe implements GitHub through the `gh` CLI. Honor any no-push/no-merge rules
declared for the project.

## Preconditions

- User explicitly requested PR/merge/cleanup.
- Working branch belongs to one focused change.
- Worktree has no unrelated uncommitted changes.
- Required verification evidence is complete or the user accepts the gap.
- A change folder under `openspec/changes/<slug>/` (excluding `archive/`) exists
  on the branch with at least `tasks.md` committed. If missing, stop before PR
  creation and complete planning first.
- `gh` is installed and authenticated when GitHub is the provider.

## Workflow

1. Inspect current branch, worktree path, and `git status`.
2. Run or confirm any verification required before merge.
3. Push the feature branch:

```bash
git push -u origin <branch-name>
```

4. Create a PR with the configured base branch:

```bash
gh pr create --base <integration-branch> --title "<title>" --body "<summary and verification>"
```

5. Before merging, archive and record SDD/OpenSpec artifacts for the change
   while still on the review branch. The archive boundary is the pre-merge
   branch state — never defer this step until after the merge lands on the base
   branch. Commit and push any archive commits to the review branch before
   proceeding.

6. Merge only after explicit user approval, required checks/review, and the
   pre-merge archive step above:

```bash
gh pr merge --squash
```

7. After the PR is merged, navigate to the main repo root first (the agent may
   be running inside the worktree, and removing it while `$PWD` points there
   causes `fatal: Unable to read current working directory`). Then remove the
   worktree and force-delete the local feature branch:

```bash
cd <main-repo-root>
git worktree remove <absolute-path-to-worktree>
git branch -D <branch-name>
```

> **Note**: `git branch -D` (capital D) is required because `gh pr merge --squash`
> rewrites history — the feature branch commits are not ancestors of the target
> branch, so `git branch -d` would refuse with "not fully merged". Force-delete
> is safe here because the PR was already merged.

If the remote feature branch still exists after merge, delete it explicitly:

```bash
git push origin --delete <branch-name>
```

8. Sync the integration branch:

```bash
git checkout <integration-branch>
git pull --ff-only origin <integration-branch>
```

## Guardrails

- Never merge locally with `git merge` for feature work that should go through PR.
- Never push, merge, delete branches, or remove worktrees without explicit user instruction.
- Never remove a worktree before confirming the PR is merged and no uncommitted work remains.
- Preserve unrelated changes; stop and ask if cleanup would touch them.
- If `gh` is unavailable or unauthenticated, stop with the exact blocker.
