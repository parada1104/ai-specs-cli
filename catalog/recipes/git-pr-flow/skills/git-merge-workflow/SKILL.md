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

## Runtime Preflight

Before pushing, run the account-match preflight when `expected_owner` is set in
`[recipes.git-pr-flow.config]`:

```bash
# Runtime Preflight: Account Match (GitHub)
# Config-gated: runs only when expected_owner is a non-empty string.
EXPECTED_OWNER="{config.expected_owner}"
AUTO_SWITCH="{config.auto_switch_account}"   # "true" | "false"

if [ -n "$EXPECTED_OWNER" ]; then
  # 1. Version guard
  GH_VER=$(gh --version | head -1 | awk '{print $3}')
  SWITCH_OK=1
  if ! printf '%s\n%s\n' "2.50.0" "$GH_VER" | sort -V -C; then
    echo "⚠ ai-specs: gh auth switch requires gh >= 2.50.0 (have $GH_VER); auto-switch disabled."
    SWITCH_OK=0
  fi

  # 2. Active account (supports multiple logged-in accounts)
  ACTIVE=$(gh auth status 2>&1 | awk '
    /Logged in to .* account/ {
      if (match($0, /account [^ ]+ \(/))      { a=substr($0, RSTART+8, RLENGTH-2) }
      else if (match($0, /account [^ ]+$/))   { a=substr($0, RSTART+8) }
    }
    /Active account: true/ { print a }
  ' | head -1)

  # 3. Target owner — prefer expected_owner
  TARGET="$EXPECTED_OWNER"

  # 4. Compare & react
  if [ "$ACTIVE" = "$TARGET" ]; then
    : # proceed
  elif [ "$AUTO_SWITCH" = "true" ] && [ "$SWITCH_OK" -eq 1 ]; then
    if ! gh auth switch --user "$TARGET" 2>&1; then
      echo "**Blocker**: gh auth switch failed for '$TARGET'. Aborting before push."
      return 1
    fi
    ACTIVE=$(gh auth status 2>&1 | awk '
      /Logged in to .* account / { if (match($0, /account [^ ]+ \(/)) { a=substr($0, RSTART+8, RLENGTH-2) } else if (match($0, /account [^ ]+$/)) { a=substr($0, RSTART+8) } }
      /Active account: true/ { print a }' | head -1)
    [ "$ACTIVE" = "$TARGET" ] || { echo "**Blocker**: switch did not land. Aborting."; return 1; }
  else
    echo "**Blocker**: active gh account is '$ACTIVE'; expected '$TARGET'."
    echo "Run: gh auth switch --user $TARGET   (or set auto_switch_account = true in ai-specs.toml)"
    return 1
  fi
fi
```

When `expected_owner` is empty (default), skip this block entirely — no extra CLI calls.

### Runtime Preflight: delete_branch_on_merge (GitHub)

GitHub's `delete_branch_on_merge` is **repo-wide** — it cannot exempt `development`
or other long-lived heads. Before merge (and when advising release PRs), check it:

```bash
# Runtime Preflight: delete_branch_on_merge (GitHub)
REPO=$(gh repo view --json nameWithOwner -q .nameWithOwner)
AUTO_DELETE=$(gh api "repos/$REPO" --jq .delete_branch_on_merge)
if [ "$AUTO_DELETE" = "true" ]; then
  echo "⚠ ai-specs: delete_branch_on_merge is true — long-lived heads"
  echo "  (development/staging) can be wiped if used as a PR head."
  echo "Remediation (needs admin; run only with explicit user approval):"
  echo "  gh api -X PATCH repos/$REPO -f delete_branch_on_merge=false"
  echo "Do not rely on GitHub auto-delete for feature cleanup — this skill"
  echo "deletes feature heads explicitly after merge."
fi
```

Do **not** auto-PATCH without explicit user approval.

## Head branch class

Before merge and cleanup, resolve `HEAD_BRANCH` = the PR head (source) branch name.

**Protected heads** (exact match): `main`, `master`, `development`, `staging`,
plus configured `[recipes.git-pr-flow.config].base_branch` and (when set)
`[recipes.worktree-flow.config].integration_branch`.

**Feature heads**: everything else — including `release/vX.Y.Z`, `feat/*`, `fix/*`.

Prefer shipping to `main` from a disposable `release/vX.Y.Z` head, not from
`development` as the PR head.

## Workflow

1. Inspect current branch, worktree path, and `git status`.
2. Run or confirm any verification required before merge.
3. Run **Runtime Preflight: Account Match** and **delete_branch_on_merge** (above).
4. Push the feature branch:

```bash
git push -u origin <branch-name>
```

5. Create a PR with the configured base branch:

```bash
gh pr create --base <integration-branch> --title "<title>" --body "<summary and verification>"
```

6. Before merging, archive and record SDD/OpenSpec artifacts for the change
   while still on the review branch. The archive boundary is the pre-merge
   branch state — never defer this step until after the merge lands on the base
   branch. Commit and push any archive commits to the review branch before
   proceeding.

7. **Pre-merge guardian (hard stop):** confirm the change is archived and has
   tier-minimum files. Prefer:

```bash
python3 "${AI_SPECS_HOME:-$HOME/.ai-specs}/lib/_internal/premerge_guardian.py" \
  <slug> --root <repo-root>
```

The helper ships with the CLI install under `~/.ai-specs` (not copied into
consumer projects).

Do **not** merge if `openspec/changes/<slug>/` still exists, or if
`openspec/changes/archive/<slug>/` is missing tier files.

8. Classify `HEAD_BRANCH` (see **Head branch class**). Merge only after explicit
   user approval, required checks/review, archive on the review branch, and a
   clean guardian result:

```bash
# Feature head — delete remote source via gh
gh pr merge --squash --delete-branch

# Protected head — never pass --delete-branch
gh pr merge --squash
```

9. After the PR is merged, sync the integration branch. **Post-merge worktree /
   local / remote branch cleanup runs only for feature heads.** For a protected
   head, skip worktree remove, `git branch -D`, and `git push origin --delete`
   for that head — only sync the base:

```bash
git checkout <integration-branch>
git pull --ff-only origin <integration-branch>
```

For a **feature** head, leave the worktree first (`cd` to the main repo root —
never remove while `$PWD` is inside the worktree). Prefer the worktree-flow
cleanup script:

```bash
cd <main-repo-root>
bash ai-specs/recipes/worktree-flow/bin/worktree-cleanup.sh \
  --dir .worktrees --base <integration-branch>
```

Manual fallback only if the script is unavailable:

```bash
git worktree remove <absolute-path-to-worktree>
git branch -D <branch-name>
```

> **Note**: `git branch -D` (capital D) is required because `gh pr merge --squash`
> rewrites history — the feature branch commits are not ancestors of the target
> branch, so `git branch -d` would refuse with "not fully merged". Force-delete
> is safe here because the PR was already merged. Stop without deleting if the
> worktree is dirty.

If the remote feature branch still exists after merge (e.g. merge without
`--delete-branch`), delete it explicitly — **feature heads only**:

```bash
git push origin --delete <branch-name>
```

## Guardrails

- Never merge locally with `git merge` for feature work that should go through PR.
- Never push, merge, delete branches, or remove worktrees without explicit user instruction.
- Never remove a worktree before confirming the PR is merged and no uncommitted work remains.
- Never delete a protected head (`main` / `master` / `development` / `staging` /
  configured base or integration branch) via `--delete-branch`, worktree cleanup,
  or `git push --delete`.
- Preserve unrelated changes; stop and ask if cleanup would touch them.
- If `gh` is unavailable or unauthenticated, stop with the exact blocker.
