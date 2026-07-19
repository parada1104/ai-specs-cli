#!/usr/bin/env bash
# Opt-in live eval runner for vault-canonical-store only.
# Does NOT run plan-build-flow or vcs-pr-flow clients.
#
# Usage:
#   ./tests/evals/run-live-vault.sh
#   EVALS_RUNTIMES=claude,cursor-agent \
#     EVALS_SCENARIOS=ac_vault_context_guidance ./tests/evals/run-live-vault.sh
#
# Models:
#   claude              → opus (Claude Code subscription)
#   cursor-agent        → composer-2.5 (Cursor Agent subscription)
#   opencode / pi / omp → cursorapi/composer-2.5 only (API for Cursor)
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

export EVALS_LIVE=1
export EVALS_PREFER="${EVALS_PREFER:-claude,cursor-agent,opencode,pi,omp}"
export EVALS_MODEL="${EVALS_MODEL:-}"
export EVALS_TRIALS="${EVALS_TRIALS:-1}"
export EVALS_TIMEOUT_SEC="${EVALS_TIMEOUT_SEC:-600}"
export EVALS_MAX_TURNS="${EVALS_MAX_TURNS:-16}"
export AI_SPECS_VENDOR_FIXTURE_ROOT="${AI_SPECS_VENDOR_FIXTURE_ROOT:-$ROOT/tests/fixtures/kepano-obsidian-skills}"

echo "client=vault-canonical-store"
echo "EVALS_PREFER=$EVALS_PREFER"
echo "EVALS_RUNTIMES=${EVALS_RUNTIMES:-"(from prefer)"}"
echo "EVALS_SCENARIOS=${EVALS_SCENARIOS:-"(all vault live)"}"
echo "EVALS_MODEL=${EVALS_MODEL:-"(defaults: claude=opus; cursor-agent=composer-2.5; others=cursorapi/composer-2.5)"}"

python3 -m unittest tests.evals.eval_vault_canonical_live -v
