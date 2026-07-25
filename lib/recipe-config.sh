#!/usr/bin/env bash
# recipe-config.sh — configure recipe config / CLI deps / harness env.
#
# Usage:
#   ai-specs configure-recipes [path] [--help]
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_SPECS_HOME="${AI_SPECS_HOME:-$(cd "$SCRIPT_DIR/.." && pwd)}"
WIZARD_PY="$AI_SPECS_HOME/lib/_internal/config_wizard.py"

usage() {
    cat <<'EOF'
Usage: ai-specs configure-recipes [path] [--help]
Configure enabled recipes (config fields, CLI deps, ai-specs/.env + root .envrc).
Arguments:
  path    Target project root (default: current directory)
Flags:
  --help  Show this help
EOF
}

TARGET_PATH=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --help|-h) usage; exit 0 ;;
        --) shift; break ;;
        -*) echo "ERROR: unknown flag: $1" >&2
            echo "Run 'ai-specs configure-recipes --help' for usage." >&2
            exit 2 ;;
        *)  if [[ -z "$TARGET_PATH" ]]; then
                TARGET_PATH="$1"
            else
                echo "ERROR: unexpected positional argument: $1" >&2
                exit 2
            fi
            shift ;;
    esac
done

[[ -z "$TARGET_PATH" ]] && TARGET_PATH="$(pwd)"
TARGET_PATH="$(cd "$TARGET_PATH" && pwd)"

exec python3 "$WIZARD_PY" "$TARGET_PATH"
