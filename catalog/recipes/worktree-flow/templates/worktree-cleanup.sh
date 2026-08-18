#!/usr/bin/env bash
# worktree-cleanup.sh — verified Go launcher for post-merge worktree cleanup.
#
# Cleanup MUST run from the main repository worktree. The Go implementation owns
# the existing merge proof, protected-branch checks, local removal, remote
# deletion, and `git ls-remote --heads` verification. There is deliberately no
# destructive Bash fallback: an unavailable or unverified binary fails closed.
#
# The path and flags remain stable for materialized projects:
#   --dir <worktrees_dir> --base <integration_branch> --dry-run
#   --topology <value> --submodule/--subrepo <path> (repeatable)
set -euo pipefail

stamped_gate_version="__WORKTREE_GATE_VERSION__"
stamped_repo_topology="__WORKTREE_REPO_TOPOLOGY__"

recipe_root() {
    local src="${BASH_SOURCE[0]:-}"
    [ -n "$src" ] || return 1
    case "$src" in
        /*) ;;
        *) src="$PWD/$src" ;;
    esac
    local dir
    dir="$(cd "$(dirname "$src")" 2>/dev/null && pwd -P)" || return 1
    # Catalog source lives in templates/; materialized copies live in
    # overrides/bin/. Both reach the recipe root by walking two parents.
    cd "$dir/../.." 2>/dev/null && pwd -P
}

verified_candidate() {
    local candidate="$1"
    [ -x "$candidate" ] || return 1
    # The receipt is written atomically by gate_binary.py after digest, version,
    # and self-test verification. Cleanup never trusts an executable alone.
    [ -f "$candidate.verified" ] || return 1
    printf '%s\n' "$candidate"
}

resolve_cleanup_binary() {
    local candidate=""
    local root=""
    if root="$(recipe_root 2>/dev/null)"; then
        if candidate="$(verified_candidate "$root/bin/worktree-gate")"; then
            printf '%s\n' "$candidate"
            return 0
        fi
    fi
    if [ -n "${WORKTREE_CLEANUP_BIN:-}" ]; then
        if candidate="$(verified_candidate "$WORKTREE_CLEANUP_BIN")"; then
            printf '%s\n' "$candidate"
            return 0
        fi
        echo "worktree-cleanup: WORKTREE_CLEANUP_BIN is not a verified executable; ignoring" >&2
    fi

    if [ -n "${WORKTREE_CLEANUP_BIN:-}" ]; then
        # An explicit but invalid override must not silently select another
        # implementation: it is a diagnostic pin and its failure is actionable.
        return 1
    fi

    local home="${AI_SPECS_HOME:-$HOME/.ai-specs}"
    local version="$stamped_gate_version"
    if [ -z "$version" ] || [ "$version" = "__WORKTREE_GATE_VERSION__" ]; then
        [ -f "$home/VERSION" ] || return 1
        version="$(tr -d '[:space:]' < "$home/VERSION")"
    fi
    local goos goarch
    case "$(uname -s)" in Darwin) goos=darwin ;; Linux) goos=linux ;; *) return 1 ;; esac
    case "$(uname -m)" in arm64|aarch64) goarch=arm64 ;; x86_64|amd64) goarch=amd64 ;; *) return 1 ;; esac
    local cache="$home/cache/bin/worktree-gate/$version/$goos-$goarch/worktree-gate"
    verified_candidate "$cache"
}

# Keep the resolver's stderr: it distinguishes "your WORKTREE_CLEANUP_BIN
# override was rejected as unverified" from "no binary was ever acquired".
# Discarding it left only a generic message for two different problems.
bin="$(resolve_cleanup_binary || true)"
if [ -z "$bin" ]; then
    echo "worktree-cleanup: no verified Go implementation available; no destructive action taken" >&2
    echo "worktree-cleanup: run ai-specs sync (or provide WORKTREE_CLEANUP_BIN with a verified receipt)" >&2
    exit 2
fi

exec "$bin" --cleanup --topology "${TOPOLOGY:-$stamped_repo_topology}" "$@"
