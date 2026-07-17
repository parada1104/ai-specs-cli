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
- A change folder under `openspec/changes/<slug>/` (excluding `archive/`) exists
  on the branch with at least `tasks.md` committed. If missing, stop before PR
  creation and complete planning first.
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
bb auth show
```

If authentication fails (output includes "Not logged in"), stop and report:

> **Blocker**: `bb` is not authenticated. Run `bb auth login` and retry.

Then run **Runtime Preflight: Account Match** when `expected_owner` is set in
`[recipes.bitbucket-pr-flow.config]` (skip when empty — no extra CLI calls):

```bash
# Runtime Preflight: Account Match (Bitbucket)
# Fix: bb has no `bb auth status`; the correct command is `bb auth show`.
EXPECTED_OWNER="{config.expected_owner}"
if [ -n "$EXPECTED_OWNER" ]; then
  ACTIVE=$(bb auth show 2>&1 | awk '/Username|username/ {print $2}' | head -1)
  if [ "$ACTIVE" != "$EXPECTED_OWNER" ]; then
    echo "**Blocker**: active bb account is '$ACTIVE'; expected '$EXPECTED_OWNER'."
    echo "bb has no 'auth switch'. Run: bb auth login"
    return 1
  fi
fi
```

## Head branch class

Before merge and cleanup, resolve `HEAD_BRANCH` = the PR source branch name.

**Protected heads** (exact match): `main`, `master`, `development`, `staging`,
plus configured `[recipes.bitbucket-pr-flow.config].base_branch` and (when set)
`[recipes.worktree-flow.config].integration_branch`.

**Feature heads**: everything else — including `release/vX.Y.Z`, `feat/*`, `fix/*`.

Prefer shipping to `main` from a disposable `release/vX.Y.Z` head, not from
`development` as the PR source. Keep Bitbucket UI "Close source branch" off for
protected heads; this skill only passes `--close-source-branch` for feature heads.

## Workflow

1. Inspect current branch, worktree path, and `git status`.
2. Run or confirm any verification required before merge.
3. Run **Runtime Preflight** (CLI checks + account match above).
4. Resolve the Bitbucket remote and push the feature branch explicitly:

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

7. Before merging, archive and record SDD/OpenSpec artifacts for the change
   while still on the review branch. The archive boundary is the pre-merge
   branch state — never defer this step until after the merge lands on the base
   branch. Commit and push any archive commits to the review branch before
   proceeding.

8. **Pre-merge guardian (hard stop):** confirm the change is archived and has
   tier-minimum files. Prefer:

```bash
python3 "${AI_SPECS_HOME:-$HOME/.ai-specs}/lib/_internal/premerge_guardian.py" \
  <slug> --root <repo-root>
```

The helper ships with the CLI install under `~/.ai-specs` (not copied into
consumer projects).

Do **not** merge if `openspec/changes/<slug>/` still exists, or if
`openspec/changes/archive/<slug>/` is missing tier files.

9. Merge only after explicit user approval, required checks/review, the
   pre-merge archive step above, a clean guardian result, and a matching
   approved source commit. Re-fetch the source commit and stop if it changed
   since approval:

```bash
CURRENT_SHA=$(bb pr view <pr-id> --json --jq '.source.commit.hash')
```

If `CURRENT_SHA` differs from `APPROVED_SHA`, stop and report that the branch
moved after approval. Otherwise classify `HEAD_BRANCH` (see **Head branch class**)
and merge with squash:

```bash
# Feature head — close source branch
bb pr merge <pr-id> --strategy squash --close-source-branch

# Protected head — never pass --close-source-branch
bb pr merge <pr-id> --strategy squash
```

> **Note**: Re-checking the source commit ensures only the reviewed revision is merged. If the branch was updated between approval and merge, stop and ask the user to re-review.

10. After the PR is merged, sync the integration branch. **Post-merge worktree /
    local branch cleanup runs only for feature heads.** For a protected head,
    skip worktree remove and `git branch -D` for that head — only sync the base:

```bash
git checkout <base_branch>
git pull --ff-only $REMOTE <base_branch>
```

For a **feature** head, leave the worktree first (`cd` to the main repo root —
never remove while `$PWD` is inside the worktree). Prefer the worktree-flow
cleanup script:

```bash
cd <main-repo-root>
bash ai-specs/recipes/worktree-flow/bin/worktree-cleanup.sh \
  --dir .worktrees --base <base_branch>
```

Manual fallback only if the script is unavailable:

```bash
git worktree remove <absolute-path-to-worktree>
git branch -D <branch-name>
```

> **Note**: `git branch -D` (capital D) is required because `bb pr merge --strategy squash`
> rewrites history — the feature branch commits are not ancestors of the target
> branch, so `git branch -d` would refuse with "not fully merged". Force-delete
> is safe here because the PR was already merged. Stop without deleting if the
> worktree is dirty.

## Guardrails

- Never merge locally with `git merge` for feature work that should go through PR.
- Never push, merge, delete branches, or remove worktrees without explicit user instruction.
- Never remove a worktree before confirming the PR is merged and no uncommitted work remains.
- Never delete a protected head (`main` / `master` / `development` / `staging` /
  configured base or integration branch) via `--close-source-branch`, worktree
  cleanup, or remote branch delete.
- Preserve unrelated changes; stop and ask if cleanup would touch them.
- Never rely on implicit push behavior from the Bitbucket CLI — always push explicitly before creating the PR.
- Never use options that merge without explicit user approval.
- If `bb` is unavailable or unauthenticated, stop with the exact blocker before pushing or creating a PR.
