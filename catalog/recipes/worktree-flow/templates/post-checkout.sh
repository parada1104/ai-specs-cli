#!/bin/sh
# post-checkout — historical reference for the worktree-flow cleanup trigger.
#
# `ai-specs sync` does NOT materialize these bytes: the CLI renders the live hook
# itself (worktreePostCheckoutHook in internal/sync/worktreecleanup.go) and writes
# it to `.git/hooks/post-checkout`, exactly as it does for post-merge.sh. This file
# is kept only as the reference for what that hook does and why, so the recipe's
# two declared hook sources stay the same kind of artifact.
#
# The live hook is one verb call: `ai-specs worktree-cleanup --if-integration`.
# post-checkout fires on every checkout, including a plain `git switch`, so the
# hook holds no branch policy of its own: the verb compares the checked-out branch
# of the primary checkout with the integration branch recorded in the project's
# last sync snapshot, runs the ordinary cleanup pass only when they match, and
# otherwise prints one line on stderr and exits 0. A missing CLI, or an installed
# CLI that does not serve the verb, also exits 0 with one actionable line, so a
# checkout never fails because of cleanup.
