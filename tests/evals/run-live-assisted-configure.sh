#!/usr/bin/env bash
# Opt-in assisted recipe configuration eval client.
# It invokes the existing eval harness only; it is not a runtime or runner.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"
export EVALS_LIVE=1
export EVALS_PREFER="${EVALS_PREFER:-claude,cursor-agent,opencode,pi,omp}"
export EVALS_MODEL="${EVALS_MODEL:-}"
export EVALS_TRIALS="${EVALS_TRIALS:-1}"
export EVALS_TIMEOUT_SEC="${EVALS_TIMEOUT_SEC:-600}"
export EVALS_MAX_TURNS="${EVALS_MAX_TURNS:-16}"

echo "client=assisted-configure"
echo "EVALS_PREFER=$EVALS_PREFER"
echo "EVALS_RUNTIMES=${EVALS_RUNTIMES:-\"(from prefer)\"}"
echo "EVALS_SCENARIOS=${EVALS_SCENARIOS:-\"(all assisted-configure scenarios)\"}"
python3 -m unittest tests.evals.eval_assisted_configure_live -v
