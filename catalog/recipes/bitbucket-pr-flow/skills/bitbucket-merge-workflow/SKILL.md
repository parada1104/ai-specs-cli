---
name: bitbucket-merge-workflow
description: >
  Provider-oriented merge workflow for feature branches created in worktrees.
  Uses the configured base branch from
  [recipes.bitbucket-pr-flow.config] (base_branch). Bitbucket via the bb CLI.
license: MIT
metadata:
  author: ai-specs
  version: "1.0"
  generatedBy: "manual-runtime"
  scope: [root]
  auto_invoke:
    - "Creating a pull request on Bitbucket"
    - "Merging a feature branch via Bitbucket PR"
    - "Cleaning up a worktree after merge"
    - "Finishing work on a feature branch"
    - "Syncing development after a merge"
---

# Bitbucket Merge Workflow

Use this skill only when the user explicitly asks to create a PR, merge, finish
a branch, or clean up after merge on Bitbucket.

Use the configured base branch from `[recipes.bitbucket-pr-flow.config]` (`base_branch`).
This recipe implements Bitbucket through the `bb` CLI. Honor any no-push/no-merge rules
declared for the project.

## Preconditions

- User explicitly requested PR/merge/cleanup.
- Working branch belongs to one focused change.
- Worktree has no unrelated uncommitted changes.
- Required verification evidence is complete or the user accepts the gap.
- `bb` is installed and authenticated.

## Runtime Preflight

Before any push or PR creation, verify the Bitbucket CLI is available:

```bash
command -v bb
```

If `bb` is not found, stop and report:

> **Blocker**: `bb` is not installed. Install it from https://bitbucket-cli.paulvanderlei.com/getting-started/installation/
> and retry.

Then verify authentication:

```bash
bb auth status
```

If authentication fails (output includes "Not logged in"), stop and report:

> **Blocker**: `bb` is not authenticated. Run `bb auth login` and retry.

## Workflow

1. Inspect current branch, worktree path, and `git status`.
2. Run or confirm any verification required before merge.
3. Resolve the Bitbucket remote and push the feature branch explicitly:

```bash
REMOTE=$(git remote | grep -E '^(origin|bitbucket|upstream)$' | head -1 || echo "origin")
git push -u $REMOTE <branch-name>
```

> **Note**: The remote is resolved dynamically to support repos where the Bitbucket remote is named `bitbucket` or `upstream` instead of `origin`. Falls back to `origin` if no known name matches.

4. Create a pull request with the configured base branch:

```bash
bb pr create --source <branch-name> --destination <base_branch> --title "<title>" --body "<summary and verification>"
```

5. STOP. Do not merge. Report the PR URL and wait for explicit user approval.

6. Before merging, capture the approved PR source commit to prevent merging unreviewed commits:

```bash
APPROVED_SHA=$(bb pr view <pr-id> --json --jq '.source.commit.hash')
```

7. Merge only after explicit user approval and required checks/review. Re-fetch the source commit and stop if it changed since approval:

```bash
CURRENT_SHA=$(bb pr view <pr-id> --json --jq '.source.commit.hash')
```

If `CURRENT_SHA` differs from `APPROVED_SHA`, stop and report that the branch moved after approval. Otherwise merge with squash and close the source branch:

```bash
bb pr merge <pr-id> --strategy squash --close-source-branch
```

> **Note**: Re-checking the source commit ensures only the reviewed revision is merged. If the branch was updated between approval and merge, stop and ask the user to re-review.

8. After the PR is merged, navigate to the main repo root first (the agent may
   be running inside the worktree, and removing it while `$PWD` points there
   causes `fatal: Unable to read current working directory`). Then remove the
   worktree and force-delete the local branch:

```bash
cd <main-repo-root>
git worktree remove <absolute-path-to-worktree>
git branch -D <branch-name>
```

> **Note**: `git branch -D` (capital D) is required because `bb pr merge --strategy squash`
> rewrites history — the feature branch commits are not ancestors of the target
> branch, so `git branch -d` would refuse with "not fully merged". Force-delete
> is safe here because the PR was already merged.

9. Sync the integration branch:

```bash
git checkout <base_branch>
git pull --ff-only $REMOTE <base_branch>
```

## Guardrails

- Never merge locally with `git merge` for feature work that should go through PR.
- Never push, merge, delete branches, or remove worktrees without explicit user instruction.
- Never remove a worktree before confirming the PR is merged and no uncommitted work remains.
- Preserve unrelated changes; stop and ask if cleanup would touch them.
- Never rely on implicit push behavior from the Bitbucket CLI — always push explicitly before creating the PR.
- Never use options that merge without explicit user approval.
- If `bb` is unavailable or unauthenticated, stop with the exact blocker before pushing or creating a PR.
