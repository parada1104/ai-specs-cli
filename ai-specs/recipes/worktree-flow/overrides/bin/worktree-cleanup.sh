#!/usr/bin/env bash
# worktree-cleanup.sh — installed-CLI launcher for post-merge cleanup.
#
# Cleanup MUST run from the main repository worktree. The Go implementation owns
# merge proof, protected-branch checks, local removal, remote deletion, and
# `git ls-remote --heads` verification. There is deliberately no Bash fallback:
# an unavailable CLI fails closed.
#
# Stable flags: --dir <worktrees_dir> --base <integration_branch> --dry-run
# --topology <value> --submodule/--subrepo <path> (repeatable).
set -euo pipefail

stamped_repo_topology="auto"
# The retired override never runs: an unverified binary must not replace the
# CLI's merge-proof and protected-branch checks, so a set pin fails closed.
if [ -n "${WORKTREE_CLEANUP_BIN:-}" ]; then
    echo "worktree-cleanup: WORKTREE_CLEANUP_BIN is no longer supported; unset it to run cleanup through the installed ai-specs CLI" >&2
    exit 2
fi
cli_bin="${AI_SPECS_HOME:-$HOME/.ai-specs}/dist/ai-specs"
if [ ! -x "$cli_bin" ]; then
    echo "worktree-cleanup: no verified Go implementation available; no destructive action taken" >&2
    echo "worktree-cleanup: install or upgrade the ai-specs CLI (expected at $cli_bin)" >&2
    exit 2
fi
if ! "$cli_bin" worktree-gate --version >/dev/null 2>&1; then
    echo "worktree-cleanup: installed ai-specs CLI is too old (no worktree-gate verb); no destructive action taken" >&2
    echo "worktree-cleanup: run 'ai-specs upgrade' (expected at $cli_bin)" >&2
    exit 2
fi

exec "$cli_bin" worktree-gate --cleanup --topology "${TOPOLOGY:-$stamped_repo_topology}" "$@"
