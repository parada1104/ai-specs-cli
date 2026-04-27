#!/usr/bin/env bash
# sdd.sh — Spec-driven development (SDD) onboarding via OpenSpec and manifest [sdd].
#
# Usage:
#   ai-specs sdd --help
#   ai-specs sdd enable [path] [options]
#   ai-specs sdd disable [path]
#   ai-specs sdd status [path]
#
# See README and `ai-specs sdd --help` for flags (install-provider-cli, dry-run, etc.).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_SPECS_HOME="$(cd "$SCRIPT_DIR/.." && pwd)"
SDD_PY="$AI_SPECS_HOME/lib/_internal/sdd.py"

exec python3 "$SDD_PY" "$@"
