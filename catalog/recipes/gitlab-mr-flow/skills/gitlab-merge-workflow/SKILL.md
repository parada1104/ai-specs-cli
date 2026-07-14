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

## Preconditions

- User explicitly requested MR/merge/cleanup.
- Working branch belongs to one focused change.
- Worktree has no unrelated uncommitted changes.
- Required verification evidence is complete or the user accepts the gap.
- A change folder under `openspec/changes/<slug>/` (excluding `archive/`) exists
  on the branch with at least `tasks.md` committed. If missing, stop before PR
  creation and complete planning first.
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
  ACTIVE=$(glab auth status 2>&1 | awk '
    /Logged in to gitlab\.com account/ {
      if (match($0, /account [^ ]+ \(/))      { a=substr($0, RSTART+8, RLENGTH-2) }
      else if (match($0, /account [^ ]+$/))   { a=substr($0, RSTART+8) }
    }
    /Active account: true/ { print a }' | head -1)
  if [ "$ACTIVE" != "$EXPECTED_OWNER" ]; then
    echo "**Blocker**: active glab account is '$ACTIVE'; expected '$EXPECTED_OWNER'."
    echo "glab has no 'auth switch'. Run: glab auth login   (or export GLAB_TOKEN=<token>)."
    return 1
  fi
fi
```

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

4. Create a merge request with the configured base branch:

```bash
glab mr create --source-branch <branch-name> --target-branch <base_branch> --title "<title>" --description "<summary and verification>" --yes
```

5. STOP. Do not merge. Report the MR URL and wait for explicit user approval.

6. Before merging, capture the approved MR head SHA to prevent merging unreviewed commits:

```bash
APPROVED_SHA=$(glab mr view <mr-number> --output json | jq -r '.sha')
```

7. Before merging, archive and record SDD/OpenSpec artifacts for the change
   while still on the review branch. The archive boundary is the pre-merge
   branch state — never defer this step until after the merge lands on the base
   branch. Commit and push any archive commits to the review branch before
   proceeding.

8. **Pre-merge guardian (hard stop):** confirm the change is archived and has
   tier-minimum files. Prefer:

```bash
python3 ai-specs/bin/premerge_guardian.py <slug> --root <repo-root>
```

Sync materializes that helper into consumer projects. In the ai-specs monorepo,
`lib/_internal/premerge_guardian.py` is the same script.

Do **not** merge if `openspec/changes/<slug>/` still exists, or if
`openspec/changes/archive/<slug>/` is missing tier files.

9. Merge only after explicit user approval, required checks/review, the
   pre-merge archive step above, a clean guardian result, and pinning the
   approved SHA:

```bash
glab mr merge <mr-number> --squash --yes --remove-source-branch --sha $APPROVED_SHA
```

> **Note**: The `--sha` flag ensures that only the reviewed commit is merged. If the branch was updated between approval and merge, the command will fail, preventing unreviewed commits from being merged.

10. After the MR is merged, **leave the worktree first** (`cd` to the main repo
   root — never remove while `$PWD` is inside the worktree). Prefer the
   worktree-flow cleanup script:

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

> **Note**: `git branch -D` (capital D) is required because `glab mr merge --squash`
> rewrites history — the feature branch commits are not ancestors of the target
> branch, so `git branch -d` would refuse with "not fully merged". Force-delete
> is safe here because the MR was already merged. Stop without deleting if the
> worktree is dirty.

11. Sync the integration branch:

```bash
git checkout <base_branch>
git pull --ff-only origin <base_branch>
```

## Guardrails

- Never merge locally with `git merge` for feature work that should go through MR.
- Never push, merge, delete branches, or remove worktrees without explicit user instruction.
- Never remove a worktree before confirming the MR is merged and no uncommitted work remains.
- Preserve unrelated changes; stop and ask if cleanup would touch them.
- Never use implicit push options on `glab mr create` — always push explicitly before creating the MR.
- Never use options that merge without explicit user approval.
- If `glab` is unavailable or unauthenticated, stop with the exact blocker before pushing or creating an MR.
