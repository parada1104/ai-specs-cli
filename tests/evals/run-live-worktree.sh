#!/usr/bin/env bash
# Opt-in live eval runner for worktree-flow only.
# Does NOT run plan-build-flow, vcs-pr-flow, or other capability clients.
#
# Usage:
#   ./tests/evals/run-live-worktree.sh
#   EVALS_RUNTIMES=opencode,claude \
#     EVALS_SCENARIOS=ac_submodule_create_uses_subrepo_contract ./tests/evals/run-live-worktree.sh
#
# Models:
#   claude              → opus (Claude Code subscription)
#   cursor-agent        → composer-2.5 (Cursor Agent subscription; binary cursor-agent|agent)
#   opencode / pi / omp → cursorapi/composer-2.5 only (API for Cursor)
# Override: EVALS_MODEL / EVALS_MODEL_CURSOR_AGENT / EVALS_MODEL_OPENCODE …
#   OpenCode-family must stay cursorapi/*; cursor-agent rejects cursorapi/*
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

export EVALS_LIVE=1
export EVALS_PREFER="${EVALS_PREFER:-claude,cursor-agent,opencode,pi,omp}"
export EVALS_MODEL="${EVALS_MODEL:-}"
export EVALS_TRIALS="${EVALS_TRIALS:-1}"
export EVALS_TIMEOUT_SEC="${EVALS_TIMEOUT_SEC:-600}"
export EVALS_MAX_TURNS="${EVALS_MAX_TURNS:-16}"

echo "client=worktree-flow"
echo "EVALS_PREFER=$EVALS_PREFER"
echo "EVALS_RUNTIMES=${EVALS_RUNTIMES:-"(from prefer)"}"
echo "EVALS_SCENARIOS=${EVALS_SCENARIOS:-"(all worktree-flow live)"}"
echo "EVALS_MODEL=${EVALS_MODEL:-"(defaults: claude=opus; cursor-agent=composer-2.5; others=cursorapi/composer-2.5)"}"

python3 -m unittest tests.evals.eval_worktree_flow_live -v
