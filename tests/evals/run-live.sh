#!/usr/bin/env bash
# Opt-in live eval runner for plan-build-flow only (do not mix capabilities).
# Usage:
#   ./tests/evals/run-live.sh
#   EVALS_RUNTIMES=opencode,pi EVALS_SCENARIOS=ac3_plan_stops_before_apply ./tests/evals/run-live.sh
#
# VCS capability live evals: ./tests/evals/run-live-vcs.sh
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

export EVALS_LIVE=1
export EVALS_PREFER="${EVALS_PREFER:-opencode,pi,omp,claude}"
export EVALS_MODEL="${EVALS_MODEL:-}"
export EVALS_TRIALS="${EVALS_TRIALS:-1}"
export EVALS_TIMEOUT_SEC="${EVALS_TIMEOUT_SEC:-600}"
export EVALS_MAX_TURNS="${EVALS_MAX_TURNS:-16}"

echo "client=plan-build-flow"
echo "EVALS_PREFER=$EVALS_PREFER"
echo "EVALS_RUNTIMES=${EVALS_RUNTIMES:-"(from prefer)"}"
echo "EVALS_SCENARIOS=${EVALS_SCENARIOS:-"(all plan-build live)"}"
echo "EVALS_MODEL=${EVALS_MODEL:-"(runtime defaults)"}"

python3 -m unittest tests.evals.eval_plan_build_flow_live -v
