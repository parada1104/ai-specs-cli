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

# Print a debug message to stderr when WORKTREE_CLEANUP_DEBUG=1.
debug_log() {
    if [[ "${WORKTREE_CLEANUP_DEBUG:-0}" == "1" ]]; then
        echo "[debug] $*" >&2
    fi
    return 0
}

# Resolve ordered base candidate refs for merge detection.
# Prints one candidate per line: exact --base, configured upstream, remote-tracking ref.
resolve_base_candidates() {
    local base="$1"
    local seen=" "

    # 1. Exact base ref
    if git rev-parse --verify --quiet "$base" >/dev/null 2>&1; then
        printf '%s\n' "$base"
        seen="$seen$base "
    fi

    # 2. Configured upstream of the base branch
    local upstream
    upstream="$(git rev-parse --verify --quiet --abbrev-ref "${base}@{u}" 2>/dev/null)" || true
    if [[ -n "$upstream" ]] && git rev-parse --verify --quiet "$upstream" >/dev/null 2>&1; then
        case "$seen" in
            *" $upstream "*) ;;
            *) printf '%s\n' "$upstream"; seen="$seen$upstream " ;;
        esac
    fi

    # 3. Remote-tracking ref for the base
    local remote
    remote="$(git config --get "branch.${base}.remote" 2>/dev/null)" || true
    if [[ -z "$remote" ]]; then
        if git config --get "remote.origin.url" >/dev/null 2>&1; then
            remote="origin"
        fi
    fi
    if [[ -n "$remote" ]]; then
        local remote_ref="refs/remotes/${remote}/${base}"
        if git rev-parse --verify --quiet "$remote_ref" >/dev/null 2>&1; then
            case "$seen" in
                *" $remote_ref "*) ;;
                *) printf '%s\n' "$remote_ref"; seen="$seen$remote_ref " ;;
            esac
        fi
    fi

    # 4. Last-resort fallback to origin/<base>, regardless of branch.${base}.remote.
    # If the configured remote is stale or doesn't track the base, this catches
    # the case where origin/<base> locally proves the merge.
    if git config --get "remote.origin.url" >/dev/null 2>&1; then
        local origin_ref="refs/remotes/origin/${base}"
        if git rev-parse --verify --quiet "$origin_ref" >/dev/null 2>&1; then
            case "$seen" in
                *" $origin_ref "*) ;;
                *) printf '%s\n' "$origin_ref"; seen="$seen$origin_ref " ;;
            esac
        fi
    fi
}

# Check if sha is an ancestor of candidate (regular / fast-forward merge).
candidate_has_merged_tip() {
    local sha="$1" candidate="$2"
    git merge-base --is-ancestor "$sha" "$candidate" 2>/dev/null
}

# Check if all unique commits in sha are present in candidate by patch-id.
candidate_has_patch_equivalence() {
    local sha="$1" candidate="$2"
    if [[ -n "$(git rev-list "${candidate}..${sha}" 2>/dev/null)" ]]; then
        local cherry
        cherry="$(git cherry "$candidate" "$sha" 2>/dev/null)"
        # Avoid `printf | grep -q` here: under `set -o pipefail`, grep -q
        # exits early on the first match, SIGPIPE kills printf (exit 141),
        # and pipefail propagates that as a pipeline failure — a false positive.
        if [[ -n "$cherry" ]]; then
            local line
            while IFS= read -r line; do
                [[ "$line" == +* ]] && return 1
            done <<< "$cherry"
            return 0
        fi
    fi
    return 1
}

# Decide whether a branch is fully merged into base, covering both regular
# (fast-forward / merge-commit) integration and squash/rebase merges.
# Evaluates ordered base candidates (exact base, upstream, remote-tracking).
is_merged() {
    local sha="$1" base="$2"
    local candidate

    # First pass: ancestry check across all candidates
    while IFS= read -r candidate; do
        if candidate_has_merged_tip "$sha" "$candidate"; then
            debug_log "merged by ancestry: $candidate"
            return 0
        fi
    done < <(resolve_base_candidates "$base")

    # Second pass: patch-id equivalence across all candidates
    while IFS= read -r candidate; do
        if candidate_has_patch_equivalence "$sha" "$candidate"; then
            debug_log "merged by patch-id: $candidate"
            return 0
        fi
    done < <(resolve_base_candidates "$base")

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
