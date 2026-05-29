#!/usr/bin/env bash
# worktree-cleanup.sh — remove merged git worktrees under a configured directory.
#
# Removes each worktree located under <dir> whose branch is fully merged into
# the integration branch. Preserves worktrees that have uncommitted changes
# (dirty) or whose branch is not yet merged (unmerged). The main worktree and
# detached-HEAD worktrees are never touched.
#
# Usage:
#   worktree-cleanup.sh [--dir <worktrees_dir>] [--base <integration_branch>] [--dry-run]
#
# Defaults:
#   --dir   .worktrees
#   --base  current branch of the main worktree
#
# Output lines (stable, greppable):
#   removed <name>
#   would remove <name>            (with --dry-run)
#   skipped <name> (dirty)
#   skipped <name> (unmerged)
#   skipped <name> (detached)
set -euo pipefail

WORKTREES_DIR=".worktrees"
BASE_BRANCH=""
DRY_RUN=0

usage() {
    sed -n '2,26p' "$0" | sed 's/^# \{0,1\}//'
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --dir) WORKTREES_DIR="${2:?--dir requires a value}"; shift 2 ;;
        --base) BASE_BRANCH="${2:?--base requires a value}"; shift 2 ;;
        --dry-run) DRY_RUN=1; shift ;;
        -h|--help) usage; exit 0 ;;
        *) echo "worktree-cleanup: unknown argument '$1'" >&2; exit 2 ;;
    esac
done

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"

if [[ -z "$BASE_BRANCH" ]]; then
    BASE_BRANCH="$(git symbolic-ref --quiet --short HEAD || echo main)"
fi

# Absolute directory that holds the worktrees we are allowed to clean.
WT_PREFIX="$ROOT/${WORKTREES_DIR%/}/"

# Parse `git worktree list --porcelain` into (path, sha, branch) records.
wt_path="" wt_sha="" wt_branch=""

flush() {
    [[ -z "$wt_path" ]] && return 0
    local path="$wt_path" sha="$wt_sha" branch="$wt_branch"
    wt_path="" wt_sha="" wt_branch=""

    # Only consider worktrees under the configured directory.
    case "$path/" in
        "$WT_PREFIX"*) ;;
        *) return 0 ;;
    esac

    local name="${path#"$WT_PREFIX"}"

    if [[ -z "$branch" ]]; then
        echo "skipped $name (detached)"
        return 0
    fi

    if [[ -n "$(git -C "$path" status --porcelain)" ]]; then
        echo "skipped $name (dirty)"
        return 0
    fi

    if ! is_merged "$sha" "$BASE_BRANCH"; then
        echo "skipped $name (unmerged)"
        return 0
    fi

    if [[ "$DRY_RUN" -eq 1 ]]; then
        echo "would remove $name"
        return 0
    fi

    git worktree remove "$path"
    # -d refuses squash/rebase-merged branches (not ancestors); -D is safe here
    # because is_merged already confirmed the branch's changes are in base.
    git branch -d "$branch" >/dev/null 2>&1 || git branch -D "$branch" >/dev/null 2>&1 || true
    echo "removed $name"
}

# Decide whether a branch is fully merged into base, covering both regular
# (fast-forward / merge-commit) integration and squash/rebase merges.
is_merged() {
    local sha="$1" base="$2"
    # Regular / fast-forward merge: branch tip is an ancestor of base.
    if git merge-base --is-ancestor "$sha" "$base" 2>/dev/null; then
        return 0
    fi
    # Squash / rebase merge: the branch tip is not an ancestor, but every commit
    # unique to the branch is already present in base by patch-id. `git cherry`
    # prints '+ <sha>' for commits NOT yet in base and '- <sha>' for those that
    # are. No '+' lines => all of the branch's changes already landed in base.
    if [[ -n "$(git rev-list "$base..$sha" 2>/dev/null)" ]]; then
        local cherry
        cherry="$(git cherry "$base" "$sha" 2>/dev/null)"
        if [[ -n "$cherry" ]] && ! printf '%s\n' "$cherry" | grep -q '^+'; then
            return 0
        fi
    fi
    return 1
}

while IFS= read -r line; do
    case "$line" in
        "worktree "*) flush; wt_path="${line#worktree }" ;;
        "HEAD "*) wt_sha="${line#HEAD }" ;;
        "branch refs/heads/"*) wt_branch="${line#branch refs/heads/}" ;;
        "detached") wt_branch="" ;;
    esac
done < <(git worktree list --porcelain)
flush
