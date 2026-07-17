#!/usr/bin/env bash
# Opt-in live eval runner for vcs-pr-flow sibling recipes only.
# Does NOT run plan-build-flow or other capability clients.
#
# Usage:
#   ./tests/evals/run-live-vcs.sh
#   EVALS_RUNTIMES=opencode,claude \
#     EVALS_SCENARIOS=git-pr-flow/ac_protected_head_no_delete ./tests/evals/run-live-vcs.sh
#
# Models (override with EVALS_MODEL):
#   claude              → opus
#   opencode / pi / omp → cursorapi/composer-2.5  (API for Cursor provider)
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

export EVALS_LIVE=1
export EVALS_PREFER="${EVALS_PREFER:-opencode,pi,omp,claude}"
export EVALS_MODEL="${EVALS_MODEL:-}"
export EVALS_TRIALS="${EVALS_TRIALS:-1}"
export EVALS_TIMEOUT_SEC="${EVALS_TIMEOUT_SEC:-600}"
export EVALS_MAX_TURNS="${EVALS_MAX_TURNS:-16}"

echo "client=vcs-pr-flow"
echo "EVALS_PREFER=$EVALS_PREFER"
echo "EVALS_RUNTIMES=${EVALS_RUNTIMES:-"(from prefer)"}"
echo "EVALS_SCENARIOS=${EVALS_SCENARIOS:-"(all vcs live)"}"
echo "EVALS_MODEL=${EVALS_MODEL:-"(runtime defaults: claude=opus; others=cursorapi/composer-2.5)"}"

python3 -m unittest tests.evals.eval_vcs_pr_flow_live -v
