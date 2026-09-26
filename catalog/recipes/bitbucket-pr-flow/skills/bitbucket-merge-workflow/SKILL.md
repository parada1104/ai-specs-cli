---
name: bitbucket-merge-workflow
description: >
  Provider-oriented merge workflow for feature branches created in worktrees.
  Uses the configured base branch from
  [recipes.bitbucket-pr-flow.config] (base_branch). Bitbucket via PHP bb-cli.
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
This recipe implements Bitbucket through PHP [`bb-cli`](https://bb-cli.github.io)
(Homebrew formula `bb-cli`, binary `bb`). Honor any no-push/no-merge rules
declared for the project. `command -v bb` cannot tell PHP `bb-cli` from a
leftover TypeScript Bitbucket CLI. Positively confirm PHP `bb-cli` with
`bb --version` plus the PHP `bb auth show` shape; if the binary is not PHP
`bb-cli`, stop and install from https://bb-cli.github.io (`brew install bb-cli`).

## Artifact ownership

This skill owns provider transport, PR review and merge, and worktree/branch
cleanup only. It does not own SDD/OpenSpec planning, spec promotion, archive, or
the pre-merge artifact guardian, and it imposes no planning-artifact
precondition on the branch.

When the `plan-build-flow` recipe is enabled, that recipe owns artifact
planning, promotion, and archive, together with its own pre-archive and
pre-merge gates; finish them before requesting this skill's merge. A project
that does not enable Plan Build can create and merge a PR with no OpenSpec
change tree.

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
BB_VERSION=$(bb --version 2>&1)
printf '%s\n' "$BB_VERSION"
if ! printf '%s\n' "$BB_VERSION" | grep -q 'Version:'; then
  echo "**Blocker**: \`bb\` on PATH is not PHP bb-cli. Install PHP bb-cli from https://bb-cli.github.io (Homebrew: \`brew install bb-cli\`; never Homebrew formula/cask \`bb\`) and retry."
  return 1
fi
```

If `bb` is not found, stop and report:

> **Blocker**: `bb` is not installed. Install it from https://bb-cli.github.io
> (Homebrew: `brew install bb-cli`; never Homebrew formula/cask `bb`) and retry.

If `bb --version` does not identify PHP `bb-cli`, stop — that binary is
not PHP `bb-cli`.

Then verify authentication with a redacted capture (emit only `Username`;
never print `AppPassword`). Parse conservatively and block on a missing,
empty, or multiple Username line:

```bash
AUTH_CAPTURE=$(bb auth show 2>&1)
USERNAME_LINES=$(printf '%s\n' "$AUTH_CAPTURE" | awk -F': ' '$1 == "Username" { print }')
USERNAME_COUNT=$(printf '%s\n' "$USERNAME_LINES" | awk 'NF { n++ } END { print n+0 }')
if [ "$USERNAME_COUNT" -ne 1 ]; then
  echo "**Blocker**: missing, empty, or multiple Username lines. \`bb\` is not authenticated. Run \`bb auth save\` and retry."
  return 1
fi
USERNAME=$(printf '%s\n' "$USERNAME_LINES" | awk -F': ' '{ gsub(/^ +| +$/, "", $2); print $2 }')
if [ -z "$USERNAME" ]; then
  echo "**Blocker**: missing or empty Username. \`bb\` is not authenticated. Run \`bb auth save\` and retry."
  return 1
fi
printf '%s\n' "$USERNAME_LINES"
```

Observed PHP output shape (do not print `AppPassword`): `Username: <value>` and
`AppPassword: <secret>`. A missing or empty `Username`, or multiple Username
lines, means credentials are absent or ambiguous. If unauthenticated, stop
and report:

> **Blocker**: `bb` is not authenticated. Run `bb auth save` and retry.

**Open verification gap:** `bb auth save` prompt text is undocumented — run it
with no invented flags. See https://bb-cli.github.io/authentication

Then run **Runtime Preflight: Account Match** when `expected_owner` is set in
`[recipes.bitbucket-pr-flow.config]` (skip when empty — no extra CLI calls):

```bash
# Runtime Preflight: Account Match (Bitbucket)
# PHP bb-cli: capture bb auth show (not bb auth status). Username parse is conservative.
EXPECTED_OWNER="{config.expected_owner}"
if [ -n "$EXPECTED_OWNER" ]; then
  AUTH_CAPTURE=$(bb auth show 2>&1)
  USERNAME_LINES=$(printf '%s\n' "$AUTH_CAPTURE" | awk -F': ' '$1 == "Username" { print }')
  USERNAME_COUNT=$(printf '%s\n' "$USERNAME_LINES" | awk 'NF { n++ } END { print n+0 }')
  if [ "$USERNAME_COUNT" -ne 1 ]; then
    echo "**Blocker**: missing, empty, or multiple Username lines. Stop rather than guessing."
    echo "bb has no 'auth switch'. Run: bb auth save"
    return 1
  fi
  ACTIVE=$(printf '%s\n' "$USERNAME_LINES" | awk -F': ' '{ gsub(/^ +| +$/, "", $2); print $2 }')
  if [ -z "$ACTIVE" ] || [ "$ACTIVE" != "$EXPECTED_OWNER" ]; then
    echo "**Blocker**: active bb account is '$ACTIVE'; expected '$EXPECTED_OWNER'."
    echo "bb has no 'auth switch'. Run: bb auth save"
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
protected heads; never pass --close-source-branch to `bb` (that flag is not a
verified PHP option). Feature-head source-branch closure is git/UI policy
(worktree remove + `git push $REMOTE --delete` + `git branch -D`), not a
PHP merge-method flag. Never remotely delete a protected head.

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

5. Create a pull request with the configured base branch (positional source then
   destination; never `-i` as the agent default):

```bash
bb pr create <branch-name> <base_branch> --title "<title>" --description "<summary>"
```

6. STOP. Do not merge. Report the PR URL and wait for explicit user approval.

7. `bb pr show <pr-id>` displays **comments** (optional second argument `true`
   for unresolved inline comments only). It is not PR JSON and must not be used
   with `--json` / `--jq`.

   Before merging, the approval-SHA policy still applies: do not merge a branch
   that moved after approval. **Open verification gap:** there is no verified
   command in this recipe to retrieve `APPROVED_SHA` / `CURRENT_SHA` from PHP
   `bb-cli` without a forbidden probe. Stop and ask the user to confirm the
   reviewed revision in the Bitbucket UI rather than guessing flags or running
   `bb pr commits` as discovery.

8. Classify `HEAD_BRANCH` (see **Head branch class**). Before the provider merge
   command, report the Tracker item state with the direct host mode of the tracker
   gate script. This is Tracker item reporting, not OpenSpec artifact or archive
   validation; the bridge is the single shell host to the verified Go `--ledger`
   predicate, and it is safe when dormant or unbound. Tracker checkpoints only
   exist when the tracker recipe is installed: if the script is absent, skip this
   step (there is no tracker lifecycle to report). The Tracker host is advisory:
   it reports a `block` / `ask` / `needs-item` verdict on stderr and exits `0`, so
   a non-zero Tracker verdict never blocks the merge or unrelated source work:

```bash
GATE=ai-specs/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh
if [ -f "$GATE" ]; then
  bash "$GATE" --root <planning-root> --checkpoint pre-merge <slug>
fi
```

   Merge only after explicit user approval, required checks/review, and a
   matching approved source commit (see the open verification gap above), then
   merge with method + id only:

```bash
bb pr merge <pr-id>
```

**Open verification gap:** squash strategy and source-branch closure are not
verified PHP `bb pr merge` options. Do not add `--strategy squash` or
`--close-source-branch`. Protected heads must not be deleted via Bitbucket UI
"Close source branch", worktree cleanup, or remote branch delete.

9. After the PR is merged, sync the integration branch. **Post-merge worktree /
    local / remote branch cleanup runs only for feature heads.** For a
    protected head, skip worktree remove, `git push $REMOTE --delete`, and
    `git branch -D` for that head — only sync the base:

```bash
git checkout <base_branch>
REMOTE=$(git remote | grep -E '^(origin|bitbucket|upstream)$' | head -1 || echo "origin")
git pull --ff-only $REMOTE <base_branch>
```

For a **feature** head, leave the worktree first (`cd` to the main repo root —
never remove while `$PWD` is inside the worktree). Prefer the worktree-flow
cleanup script:

```bash
cd <main-repo-root>
bash ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh \
  --dir .worktrees --base <base_branch>
```

Manual fallback only if the script is unavailable. After a successful merge,
delete the **feature** remote branch using the same dynamic `$REMOTE`
resolution. Never run remote delete for a protected head (`main`, `master`,
`development`, `staging`, configured base/integration). Require explicit user
instruction and stop without deleting if the worktree is dirty:

```bash
REMOTE=$(git remote | grep -E '^(origin|bitbucket|upstream)$' | head -1 || echo "origin")
git worktree remove <absolute-path-to-worktree>
git push "$REMOTE" --delete <branch-name>
git branch -D <branch-name>
```

> **Note**: `git branch -D` (capital D) is required when the merged PR does not
> leave the feature commits as ancestors of the target (common after a UI
> squash). `git branch -d` would refuse with "not fully merged". Force-delete
> is safe here because the PR was already merged. Stop without deleting if the
> worktree is dirty. Remote delete is `git push $REMOTE --delete` for feature
> heads only.

## Guardrails

- Never merge locally with `git merge` for feature work that should go through PR.
- Never push, merge, delete branches, or remove worktrees without explicit user instruction.
- Never remove a worktree before confirming the PR is merged and no uncommitted work remains.
- Never delete a protected head (`main` / `master` / `development` / `staging` /
  configured base or integration branch) via Bitbucket close-source UI, worktree
  cleanup, or remote branch delete.
- Preserve unrelated changes; stop and ask if cleanup would touch them.
- Never rely on implicit push behavior from the Bitbucket CLI — always push explicitly before creating the PR.
- Never use options that merge without explicit user approval.
- If `bb` is unavailable or unauthenticated, stop with the exact blocker before pushing or creating a PR.
