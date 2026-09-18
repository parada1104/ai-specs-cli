#!/usr/bin/env bash
# post-merge — managed worktree-flow cleanup hook (fail-open).
#
# A merge boundary ends the matching tracker-ledger item. The close itself lives
# in the verified worktree cleanup launcher, which closes the item at
# archive-close before it removes a provably merged worktree or local branch.
# This hook only triggers that launcher, so the lifecycle is workflow-agnostic: it
# does not depend on SDD, ODD, OpenSpec, or any agent, and it performs no provider
# or network call.
#
# Cleanup is destructive, so the hook must hand the project's configured cleanup
# inputs (worktrees dir, integration branch, topology) to the launcher: without
# them cleanup silently falls back to `.worktrees` / the current HEAD for a
# customized project. The three values below are stamped by `ai-specs sync`.
#
# The merge already succeeded before this hook runs, so the hook MUST NOT change
# the merge outcome: every failure is reported to stderr and the hook exits 0. An
# existing user hook at this path is preserved by the recipe's not_exists
# materialization policy.
set -uo pipefail

# Stamped from [recipes.worktree-flow.config] at sync.
worktrees_dir="__WORKTREE_WORKTREES_DIR__"
integration_branch="__WORKTREE_INTEGRATION_BRANCH__"
topology="__WORKTREE_REPO_TOPOLOGY__"

# Cleanup runs Git internally; never re-enter this hook from a nested operation.
if [ "${WORKTREE_FLOW_POST_MERGE_RUNNING:-}" = "1" ]; then
    exit 0
fi

root="$(git rev-parse --show-toplevel 2>/dev/null || true)"
if [ -z "$root" ]; then
    exit 0
fi

launcher="$root/ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh"
if [ ! -x "$launcher" ]; then
    echo "worktree-flow post-merge: cleanup launcher not found at $launcher; skipping" >&2
    exit 0
fi

# The merge outcome is already sealed, so every cleanup failure is reported to
# stderr and the hook exits 0. Destructive cleanup itself fails closed: a failed
# ledger close preserves the candidate rather than removing unrecorded work.
WORKTREE_FLOW_POST_MERGE_RUNNING=1 "$launcher" \
    --topology "$topology" \
    --dir "$worktrees_dir" \
    --base "$integration_branch"
status=$?
if [ "$status" -ne 0 ]; then
    echo "worktree-flow post-merge: cleanup reported issues (exit $status); the merge is unaffected" >&2
fi

exit 0
