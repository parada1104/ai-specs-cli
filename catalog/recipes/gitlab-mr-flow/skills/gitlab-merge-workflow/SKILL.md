---
name: gitlab-merge-workflow
description: >
  Provider-oriented merge workflow for feature branches created in worktrees.
  Uses the configured base branch from
  [recipes.gitlab-mr-flow.config] (base_branch). GitLab via the glab CLI.
license: MIT
metadata:
  author: ai-specs
  version: "1.0"
  generatedBy: "manual-runtime"
  scope: [root]
  auto_invoke:
    - "Creating a merge request on GitLab"
    - "Merging a feature branch via GitLab MR"
    - "Cleaning up a worktree after merge"
    - "Finishing work on a feature branch"
    - "Syncing development after a merge"
---

# GitLab Merge Workflow

Use this skill only when the user explicitly asks to create an MR, merge, finish
a branch, or clean up after merge on GitLab.

Use the configured base branch from `[recipes.gitlab-mr-flow.config]` (`base_branch`).
This recipe implements GitLab through the `glab` CLI. Honor any no-push/no-merge rules
declared for the project.

## Artifact ownership

This skill owns provider transport, MR review and merge, and worktree/branch
cleanup only. It does not own SDD/OpenSpec planning, spec promotion, archive, or
the pre-merge artifact guardian, and it imposes no planning-artifact
precondition on the branch.

When the `plan-build-flow` recipe is enabled, that recipe owns artifact
planning, promotion, and archive, together with its own pre-archive and
pre-merge gates; finish them before requesting this skill's merge. A project
that does not enable Plan Build can create and merge an MR with no OpenSpec
change tree.

## Preconditions

- User explicitly requested MR/merge/cleanup.
- Working branch belongs to one focused change.
- Worktree has no unrelated uncommitted changes.
- Required verification evidence is complete or the user accepts the gap.
- `glab` is installed and authenticated.

## Runtime Preflight

Before any push or MR creation, verify the GitLab CLI is available:

```bash
command -v glab
```

If `glab` is not found, stop and report:

> **Blocker**: `glab` is not installed. Install it from https://gitlab.com/gitlab-org/cli
> and retry.

Then verify authentication:

```bash
glab auth status
```

If authentication fails, stop and report:

> **Blocker**: `glab` is not authenticated. Run `glab auth login` and retry.

Then verify `jq` is available (required for SHA pinning during merge):

```bash
command -v jq
```

If `jq` is not found, stop and report:

> **Blocker**: `jq` is not installed. Install it from https://jqlang.github.io/jq/download/ and retry.

Then run **Runtime Preflight: Account Match** when `expected_owner` is set in
`[recipes.gitlab-mr-flow.config]` (skip when empty — no extra CLI calls):

```bash
# Runtime Preflight: Account Match (GitLab)
EXPECTED_OWNER="{config.expected_owner}"
if [ -n "$EXPECTED_OWNER" ]; then
  # glab has no "Active account" marker: only trust the result when the
  # status lists exactly one login, otherwise leave it empty so the mismatch
  # blocker fires.
  ACTIVE=$(glab auth status 2>&1 | awk '
    /Logged in to .* as / {
      for (i = 1; i <= NF; i++) if ($i == "as") { a = $(i + 1) }
      n += 1
    }
    END { if (n == 1) print a }
  ')
  if [ "$ACTIVE" != "$EXPECTED_OWNER" ]; then
    echo "**Blocker**: active glab account is '$ACTIVE'; expected '$EXPECTED_OWNER'."
    echo "glab has no 'auth switch'. Run: glab auth login   (or export GLAB_TOKEN=<token>)."
    return 1
  fi
fi
```

## Head branch class

Before merge and cleanup, resolve `HEAD_BRANCH` = the MR source branch name.

**Protected heads** (exact match): `main`, `master`, `development`, `staging`,
plus configured `[recipes.gitlab-mr-flow.config].base_branch` and (when set)
`[recipes.worktree-flow.config].integration_branch`.

**Feature heads**: everything else — including `release/vX.Y.Z`, `feat/*`, `fix/*`.

Prefer shipping to `main` from a disposable `release/vX.Y.Z` head, not from
`development` as the MR source. Keep GitLab UI "Delete source branch" off for
protected heads; this skill only passes `--remove-source-branch` for feature heads.

## Workflow

1. Inspect current branch, worktree path, and `git status`.
2. Run or confirm any verification required before merge.
3. Run **Runtime Preflight** (CLI checks + account match above).
4. Resolve the GitLab remote and push the feature branch explicitly:

```bash
REMOTE=$(git remote | grep -E '^(origin|gitlab|upstream)$' | head -1 || echo "origin")
git push -u $REMOTE <branch-name>
```

> **Note**: The remote is resolved dynamically to support repos where the GitLab remote is named `gitlab` or `upstream` instead of `origin`. Falls back to `origin` if no known name matches.

5. Create a merge request with the configured base branch:

```bash
glab mr create --source-branch <branch-name> --target-branch <base_branch> --title "<title>" --description "<summary and verification>" --yes
```

6. STOP. Do not merge. Report the MR URL and wait for explicit user approval.

7. Before merging, capture the approved MR head SHA to prevent merging unreviewed commits:

```bash
APPROVED_SHA=$(glab mr view <mr-number> --output json | jq -r '.sha')
```

8. Classify `HEAD_BRANCH` (see **Head branch class**). Before the provider merge
   command, report the Tracker item state with the tracker gate's direct host
   mode. This is Tracker item reporting, not OpenSpec artifact or archive
   validation; the verb is the single host to the verified Go `--ledger`
   predicate, and it is safe when dormant or unbound. Tracker checkpoints only
   exist when the tracker recipe is installed: without that recipe, skip this
   step (there is no tracker lifecycle to report). The Tracker gate is advisory:
   it reports a `block` / `ask` / `needs-item` verdict on stderr and exits `0`,
   so a non-zero Tracker verdict never blocks the merge or unrelated source work:

```bash
ai-specs tracker-card-gate --root <planning-root> --checkpoint pre-merge <slug>
```

   Merge only after explicit user approval, required checks/review, and
   pinning the approved SHA. The merge strategy is the user's choice: ask the
   user which one to use (merge commit, squash, rebase, or fast-forward where
   the provider allows it) and pass exactly that, never a strategy you picked
   yourself:

```bash
# <strategy>: omit it for a merge commit, or add --squash / --rebase, whichever the user chose

# Feature head — remove source branch
glab mr merge <mr-number> <strategy> --yes --remove-source-branch --sha $APPROVED_SHA

# Protected head — never pass --remove-source-branch
glab mr merge <mr-number> <strategy> --yes --sha $APPROVED_SHA
```

> **Note**: The `--sha` flag ensures that only the reviewed commit is merged. If the branch was updated between approval and merge, the command will fail, preventing unreviewed commits from being merged.

9. After the MR is merged, sync the integration branch. **Post-merge worktree /
    local branch cleanup runs only for feature heads.** For a protected head,
    skip worktree remove and `git branch -D` for that head — only sync the base:

```bash
git checkout <base_branch>
REMOTE=$(git remote | grep -E '^(origin|gitlab|upstream)$' | head -1 || echo "origin")
git pull --ff-only $REMOTE <base_branch>
```

For a **feature** head, leave the worktree first (`cd` to the main repo root —
never remove while `$PWD` is inside the worktree). Prefer the worktree-flow
cleanup script:

```bash
cd <main-repo-root>
ai-specs worktree-cleanup
```

Manual fallback only if the script is unavailable:

```bash
git worktree remove <absolute-path-to-worktree>
git branch -d <branch-name>
```

> **Note**: `git branch -d` works after a merge commit or a fast-forward, which
> leave the feature commits as ancestors of the target. A squash or a rebase
> rewrites them, so `-d` would refuse with "not fully merged" and `git branch -D`
> (capital D) is required. Force-delete is safe there because the MR was already
> merged. Stop without deleting if the worktree is dirty.

## Guardrails

- Never merge locally with `git merge` for feature work that should go through MR.
- Never push, merge, delete branches, or remove worktrees without explicit user instruction.
- Never remove a worktree before confirming the MR is merged and no uncommitted work remains.
- Never delete a protected head (`main` / `master` / `development` / `staging` /
  configured base or integration branch) via `--remove-source-branch`, worktree
  cleanup, or remote branch delete.
- Preserve unrelated changes; stop and ask if cleanup would touch them.
- Never use implicit push options on `glab mr create` — always push explicitly before creating the MR.
- Never use options that merge without explicit user approval.
- If `glab` is unavailable or unauthenticated, stop with the exact blocker before pushing or creating an MR.
