#!/usr/bin/env bash
# recipe-init.sh — print an agent-readable recipe initialization brief.
#
# Usage:
#   ai-specs recipe init <id> [path] [--help]
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_SPECS_HOME="$(cd "$SCRIPT_DIR/.." && pwd)"
RECIPE_INIT_PY="$AI_SPECS_HOME/lib/_internal/recipe-init.py"

usage() {
    cat <<'EOF'
Usage: ai-specs recipe init <id> [path] [--help]
Print a read-only, agent-readable initialization brief for a recipe.
Arguments:
  id      Recipe identifier (directory name under catalog/recipes/)
  path    Target project root (default: current directory)
Flags:
  --help  Show this help
EOF
}

RECIPE_ID=""
TARGET_PATH=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --help|-h) usage; exit 0 ;;
        --) shift; break ;;
        -*) echo "ERROR: unknown flag: $1" >&2
            echo "Run 'ai-specs recipe init --help' for usage." >&2
            exit 2 ;;
        *)  if [[ -z "$RECIPE_ID" ]]; then
                RECIPE_ID="$1"
            elif [[ -z "$TARGET_PATH" ]]; then
                TARGET_PATH="$1"
            else
                echo "ERROR: unexpected positional argument: $1" >&2
                exit 2
            fi
            shift ;;
    esac
done

if [[ -z "$RECIPE_ID" ]]; then
    echo "ERROR: missing recipe id" >&2
    usage >&2
    exit 2
fi

[[ -z "$TARGET_PATH" ]] && TARGET_PATH="$(pwd)"
TARGET_PATH="$(cd "$TARGET_PATH" && pwd)"

exec python3 "$RECIPE_INIT_PY" "$TARGET_PATH" "$RECIPE_ID"
