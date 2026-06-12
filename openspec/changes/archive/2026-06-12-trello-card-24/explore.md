# Exploration: trello-card-24 — worktree-cleanup false negative on regular merge commits

### Current State

The cleanup script's canonical source in this repo is `catalog/recipes/worktree-flow/templates/worktree-cleanup.sh`. It is materialized for consumer projects to `ai-specs/recipes/worktree-flow/bin/worktree-cleanup.sh` by `catalog/recipes/worktree-flow/recipe.toml` lines 67-70, using `condition = "not_exists"`.

The script parses `git worktree list --porcelain`, filters worktrees under the configured directory, skips detached worktrees, skips dirty worktrees before merge detection, then calls `is_merged "$sha" "$BASE_BRANCH"` (`catalog/recipes/worktree-flow/templates/worktree-cleanup.sh:68-80`). If merged, it removes the worktree and deletes the branch with `git branch -d` or `git branch -D` (`:83-92`).

`is_merged` currently uses a two-step heuristic:

1. `git merge-base --is-ancestor "$sha" "$base"` for regular merge / fast-forward ancestry (`:97-102`).
2. If `git rev-list "$base..$sha"` is non-empty, run `git cherry "$base" "$sha"` and treat the branch as merged only when `git cherry` emits entries and none starts with `+` (`:103-113`). This is the squash/rebase patch-id path.

### Affected Areas

- `catalog/recipes/worktree-flow/templates/worktree-cleanup.sh` — canonical cleanup script and `is_merged` implementation.
- `catalog/recipes/worktree-flow/recipe.toml` — materializes the script to consumer projects with `condition = "not_exists"`, which affects rollout behavior.
- `tests/test_worktree_cleanup.py` — existing hermetic behavior tests cover regular local merge, squash merge, unmerged, dirty, and dry-run, but not stale local base vs remote-tracking base.
- `catalog/recipes/worktree-flow/README.md` and `commands/worktree-clean.md` — user-facing cleanup contract and invocation docs.

### Root Cause

The false negative is not caused by a missing `merge-base` check; that positive check already runs first (`catalog/recipes/worktree-flow/templates/worktree-cleanup.sh:99-101`). The failure occurs when the caller passes `--base development` after `gh pr merge --merge`: GitHub creates the merge commit on the remote base, but local `development` can remain stale. The card's manual proof used `origin/development`, while the script compares only the exact `--base` argument (`development`) in both `git merge-base --is-ancestor "$sha" "$base"` and `git cherry "$base" "$sha"` (`:100`, `:107`, `:109`). In that state, the branch tip is an ancestor of `origin/development` but not of local `development`; the ancestry check fails, then `git cherry development <branch-tip>` reports `+` entries because local `development` lacks the merged commits, so line 110 rejects the branch as `unmerged`.

Synthetic reproduction confirmed this shape: with local `main` intentionally stale and `refs/remotes/origin/main` updated to a regular merge commit containing the feature branch, `--base main --dry-run` printed `skipped feat-regular (unmerged)`, while `--base origin/main --dry-run` printed `would remove feat-regular`.

### Edge Case Verdicts

| Edge case | Current heuristic verdict | Notes |
|---|---|---|
| Regular merge commit | `merged` if the exact base ref includes the merge commit; false `unmerged` if local base is stale while `origin/<base>` has the merge | This matches the PR #93 failure mode. |
| Squash merge | `merged` when all branch-unique commits are patch-equivalent to base via `git cherry`; otherwise `unmerged` | Existing test covers happy path. |
| Rebase merge | `merged` when rebased commits on base have matching patch IDs; otherwise `unmerged` | Same `git cherry` path as squash. |
| Fast-forward | `merged` | Branch tip is an ancestor of base, so `merge-base --is-ancestor` succeeds. |
| Local-only branch | Usually `unmerged` if it has commits not in base; `merged` if it points at or behind base | Remote existence is not checked. |
| Branch deleted on remote but local still exists | Depends only on local branch tip and base refs; remote deletion does not matter | Safe if base contains the branch; preserved if not. |
| Branch ahead of base | `unmerged` | `git cherry` should emit `+` entries. |
| Branch with uncommitted changes | `dirty`, not merge-evaluated | Dirty check runs before `is_merged` (`:73-80`). |
| Branch with merge conflicts resolved in a merge commit | `merged` if the exact base ref contains the merge commit; false `unmerged` if local base is stale | Ancestry handles conflict-resolution merge commits because the branch tip is still a merge parent. |

### Approaches

1. **Positive remote-tracking candidate check** — Expand `is_merged` to consider safe candidate base refs: exact `--base`, configured upstream of that local branch when available, and `origin/<base>` when it exists. Return merged if any candidate contains the branch tip by ancestry before falling back to `git cherry`.
   - Pros: Fixes the observed `gh pr merge --merge` workflow without weakening conservative cleanup; local-only/offline usage still works.
   - Cons: Requires careful ref resolution and deterministic ordering; remote-tracking refs may be stale unless the user fetched.
   - Effort: Low/Medium

2. **Require callers to pass `origin/<base>` or fetch first** — Keep script behavior unchanged and update commands/docs.
   - Pros: Minimal code change.
   - Cons: Does not satisfy the card's target behavior; easy for agents/users to keep hitting the same false negative.
   - Effort: Low

3. **Fetch inside cleanup before checking** — Automatically fetch the base remote before cleanup.
   - Pros: Makes remote evidence fresher.
   - Cons: Adds network dependency and side effects to a cleanup script; not ideal for hermetic tests or offline flows.
   - Effort: Medium

### Recommendation

Use Approach 1. Keep the conservative philosophy: only add positive proof paths before declaring `unmerged`; do not remove if no candidate ref proves the branch is merged or patch-equivalent. The implementation shape should be minimal:

```bash
base_candidates() {
  printf '%s\n' "$1"
  if git rev-parse --verify --quiet "$1@{upstream}" >/dev/null; then
    git rev-parse --symbolic-full-name --abbrev-ref "$1@{upstream}"
  fi
  if [[ "$1" != */* ]] && git rev-parse --verify --quiet "origin/$1" >/dev/null; then
    printf '%s\n' "origin/$1"
  fi
}

is_merged() {
  local sha="$1" base="$2" candidate
  while IFS= read -r candidate; do
    [[ -z "$candidate" ]] && continue
    git merge-base --is-ancestor "$sha" "$candidate" 2>/dev/null && return 0
  done < <(base_candidates "$base" | awk '!seen[$0]++')

  while IFS= read -r candidate; do
    [[ -z "$candidate" ]] && continue
    if [[ -n "$(git rev-list "$candidate..$sha" 2>/dev/null)" ]]; then
      local cherry
      cherry="$(git cherry "$candidate" "$sha" 2>/dev/null)"
      [[ -n "$cherry" ]] && ! printf '%s\n' "$cherry" | grep -q '^+' && return 0
    fi
  done < <(base_candidates "$base" | awk '!seen[$0]++')

  return 1
}
```

The exact patch can avoid `awk` if project style prefers pure Bash arrays. The important behavior is: ancestry against remote/upstream candidates is a positive merged proof, never a destructive fallback.

### Test Strategy

Add hermetic tests to `tests/test_worktree_cleanup.py` using throwaway repositories, not live PRs. Existing helpers already create temp repos and worktrees. For the regression, create the feature branch/worktree, then synthesize a regular merge commit object with `git commit-tree <branch>^{tree} -p main -p <branch> -m merge` and update only `refs/remotes/origin/main` with `git update-ref`, leaving local `main` stale. Assert that `bash <script> --base main --dry-run` reports `would remove <branch>` once the fix is applied. This directly captures the PR #93 failure shape without network or GitHub.

Expand coverage with one test per contract edge: regular local merge, regular remote-only merge with stale local base, squash merge, rebase/patch-id equivalent merge, fast-forward, local-only unmerged branch, remote-deleted/local branch whose tip is already in base, branch ahead of base, dirty branch, and conflict-resolution merge commit. Keep tests hermetic by setting git author/committer env vars, using `tempfile.TemporaryDirectory`, avoiding remotes that require network, and using `git update-ref` for remote-tracking refs.

### Risks

- Candidate ref expansion can accidentally trust stale `origin/<base>` in the other direction; this is still conservative if removal requires positive ancestry or patch-id proof.
- Existing consumer projects have materialized copies under `ai-specs/recipes/worktree-flow/bin/worktree-cleanup.sh`; because the template is `not_exists`, a template fix may not overwrite already-materialized scripts unless versioning/migration/docs handle it.
- `git cherry` remains patch-id based and can be fooled by duplicate/reverted patches; this is pre-existing and should stay within the card's conservative-bias scope.

### Architectural Concerns

- The user-provided investigation path (`ai-specs/recipes/worktree-flow/bin/worktree-cleanup.sh`) is the materialized consumer path, but this worktree currently has no such tracked file. The repo source of truth is `catalog/recipes/worktree-flow/templates/worktree-cleanup.sh`.
- The recipe is dogfooded by this repo (`ai-specs/ai-specs.toml:39-44` enables `worktree-flow` with `integration_branch = "development"`), but the source template and any materialized local copy can drift.
- `condition = "not_exists"` means rollout needs attention: syncing existing projects may preserve the old script.

### Ready for Proposal

Yes. The proposal should frame this as a behavior bug in cleanup merge detection: local base refs can be stale after remote PR merges, so the script needs positive merged-proof checks against upstream/remote-tracking base candidates before declaring a clean worktree `unmerged`.
