#!/usr/bin/env bash
# worktree-cleanup.sh — remove merged git worktrees under a configured directory.
#
# Removes each worktree located under <dir> whose branch is fully merged into
# the integration branch. Preserves worktrees that have uncommitted changes
# (dirty) or whose branch is not yet merged (unmerged). The main worktree and
# detached-HEAD worktrees are never touched.
#
# Under monorepo-submodules, enumerates each initialized submodule (never the
# superproject worktree list alone) and scans shared <worktrees_dir>/<module>-*
# linked worktrees. standalone / monorepo-apps keep a single-repo pass.
#
# Usage:
#   worktree-cleanup.sh [--dir <worktrees_dir>] [--base <integration_branch>]
#                       [--dry-run] [--submodule <path>|--subrepo <path>]...
#
# Defaults:
#   --dir   .worktrees
#   --base  current branch of each scanned repo (or --base when provided)
#   --submodule / --subrepo  (none = all initialized submodules)
#
# Environment variables:
#   WORKTREE_CLEANUP_DEBUG=1       print debug messages to stderr
#   WORKTREE_CLEANUP_SOURCE_ONLY=1  (test helper) skip execution loop when sourced
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
SUBMODULE_SCOPE=()

usage() {
    sed -n '2,30p' "$0" | sed 's/^# \{0,1\}//'
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --dir) WORKTREES_DIR="${2:?--dir requires a value}"; shift 2 ;;
        --base) BASE_BRANCH="${2:?--base requires a value}"; shift 2 ;;
        --dry-run) DRY_RUN=1; shift ;;
        --submodule|--subrepo)
            SUBMODULE_SCOPE+=("${2:?$1 requires a value}"); shift 2 ;;
        -h|--help) usage; exit 0 ;;
        *) echo "worktree-cleanup: unknown argument '$1'" >&2; exit 2 ;;
    esac
done

SUPER_ROOT="$(git rev-parse --show-toplevel)"
USER_BASE="$BASE_BRANCH"
WT_ROOT="$SUPER_ROOT/${WORKTREES_DIR%/}"

# Absolute directory that holds the worktrees we are allowed to clean.
# Shared across all modules (not recomputed per submodule).
WT_PREFIX="$WT_ROOT/"

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
# Prints one candidate per line: exact --base, configured upstream,
# configured remote-tracking ref, conditional origin/<base> fallback.
resolve_base_candidates() {
    local base="$1"
    local seen=" "

    # Emit a candidate ref if it resolves to a real object and hasn't been
    # emitted yet under this exact spelling. Every emission still goes
    # through `git rev-parse --verify --quiet` so only positively-proven
    # refs are ever printed (positive-proof-only semantics).
    #
    # Returns non-zero when the ref is a duplicate or does not resolve —
    # NEVER use this return value to decide whether a ref resolves (a
    # deduplicated ref resolves but returns 1). Callers under `set -e` are
    # safe here only because these functions run in conditional contexts
    # (`if ! is_merged ...`); guard any future unconditional call sites.
    emit_candidate() {
        local ref="$1"
        case "$seen" in
            *" $ref "*) return 1 ;;
        esac
        if git rev-parse --verify --quiet "$ref" >/dev/null 2>&1; then
            printf '%s\n' "$ref"
            seen="$seen$ref "
            return 0
        fi
        return 1
    }

    # 1. Exact base ref
    emit_candidate "$base"

    # 2. Configured upstream of the base branch, normalized to its full ref
    # name (refs/remotes/<remote>/<base>) via --symbolic-full-name rather
    # than --abbrev-ref. --abbrev-ref would print the short form
    # (e.g. "origin/main"), which does not match the "refs/remotes/..."
    # spelling used by candidates 3/4 and would defeat the dedup below.
    local upstream
    upstream="$(git rev-parse --verify --quiet --symbolic-full-name "${base}@{u}" 2>/dev/null)" || true
    [[ -n "$upstream" ]] && emit_candidate "$upstream"

    # 3. Remote-tracking ref for the base's configured remote. The
    # "resolved" flag is decided by a direct rev-parse check, NOT by
    # emit_candidate's return value: with full upstream tracking, candidate
    # 2 already emitted this exact ref, so emit_candidate would return
    # non-zero (dedup) even though the ref resolves — conflating the two
    # would wrongly re-enable the origin fallback below.
    local configured_remote configured_remote_resolved=0
    configured_remote="$(git config --get "branch.${base}.remote" 2>/dev/null)" || true
    if [[ -n "$configured_remote" ]]; then
        if git rev-parse --verify --quiet "refs/remotes/${configured_remote}/${base}" >/dev/null 2>&1; then
            configured_remote_resolved=1
            emit_candidate "refs/remotes/${configured_remote}/${base}" || true
        fi
    fi

    # 4. Conditional last-resort fallback to origin/<base>. This only runs
    # when the configured remote-tracking ref above did NOT resolve (no
    # branch.${base}.remote configured, or it points at a ref that doesn't
    # exist locally). If branch.${base}.remote already resolved to a
    # different, valid remote (e.g. "upstream"), that remote's ref is
    # authoritative and origin/<base> — which may belong to an unrelated
    # fork — must not be consulted (dual-remote safety).
    if [[ "$configured_remote_resolved" -eq 0 ]] && git config --get "remote.origin.url" >/dev/null 2>&1; then
        emit_candidate "refs/remotes/origin/${base}"
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
    local candidate candidates
    candidates="$(resolve_base_candidates "$base")"

    [[ -z "$candidates" ]] && return 1

    # First pass: ancestry check across all candidates
    while IFS= read -r candidate; do
        if candidate_has_merged_tip "$sha" "$candidate"; then
            debug_log "merged by ancestry: $candidate"
            return 0
        fi
    done <<< "$candidates"

    # Second pass: patch-id equivalence across all candidates
    while IFS= read -r candidate; do
        if candidate_has_patch_equivalence "$sha" "$candidate"; then
            debug_log "merged by patch-id: $candidate"
            return 0
        fi
    done <<< "$candidates"

    return 1
}


_in_scope() {
    local p="$1" s
    ((${#SUBMODULE_SCOPE[@]})) || return 0
    for s in "${SUBMODULE_SCOPE[@]}"; do
        [[ "$s" == "$p" ]] && return 0
    done
    return 1
}

# Topology self-detect (bash mirror of util.resolve_repo_topology):
# monorepo-submodules iff .gitmodules exists AND has >=1 non-'-' status line.
enumerate_modules() {
    # Avoid `grep -q` under `set -o pipefail`: with 2+ initialized modules,
    # grep -q exits early, SIGPIPEs git, and the pipeline fails → false
    # standalone classification. Drain status in a while-read instead.
    local has_init=0 line p
    if [[ -f "$SUPER_ROOT/.gitmodules" ]]; then
        while IFS= read -r line; do
            [[ -z "$line" ]] && continue
            if [[ "${line:0:1}" != "-" ]]; then
                has_init=1
                break
            fi
        done < <(git -C "$SUPER_ROOT" submodule status 2>/dev/null || true)
    fi
    if [[ "$has_init" -eq 1 ]]; then
        while IFS= read -r line; do
            [[ -z "$line" ]] && continue
            [[ "${line:0:1}" == "-" ]] && continue
            p="$(awk '{print $2}' <<<"${line:1}")"
            [[ -z "$p" ]] && continue
            if ((${#SUBMODULE_SCOPE[@]})); then
                _in_scope "$p" || continue
            fi
            printf '%s\n' "$SUPER_ROOT/$p"
        done < <(git -C "$SUPER_ROOT" submodule status 2>/dev/null || true)
    else
        printf '%s\n' "$SUPER_ROOT"
    fi
}

# One cleanup pass over the CURRENT cwd (unchanged inner scan→flush logic).
_cleanup_one() {
    while IFS= read -r line; do
        case "$line" in
            "worktree "*) flush; wt_path="${line#worktree }" ;;
            "HEAD "*) wt_sha="${line#HEAD }" ;;
            "branch refs/heads/"*) wt_branch="${line#branch refs/heads/}" ;;
            "detached") wt_branch="" ;;
        esac
    done < <(git worktree list --porcelain)
    flush
}

# Test-only hook: when sourced with WORKTREE_CLEANUP_SOURCE_ONLY=1, stop
# right after the function definitions above and skip the worktree-scanning
# loop below. This lets tests source the real script to exercise its
# functions directly (e.g. candidate_has_patch_equivalence) without running
# a full cleanup pass as a side effect. Never set in normal usage.
if [[ "${WORKTREE_CLEANUP_SOURCE_ONLY:-0}" == "1" ]]; then
    if [[ "${BASH_SOURCE[0]}" != "${0}" ]]; then
        return 0
    fi
    exit 0
fi


while IFS= read -r repo_dir; do
    cd "$repo_dir"
    WT_PREFIX="$WT_ROOT/"
    if [[ -z "$USER_BASE" ]]; then
        BASE_BRANCH="$(git symbolic-ref --quiet --short HEAD || echo main)"
    else
        BASE_BRANCH="$USER_BASE"
    fi
    _cleanup_one
done < <(enumerate_modules)
