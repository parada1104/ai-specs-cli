#!/usr/bin/env bash
# recipe.sh — sub-dispatcher for recipe commands.
#
# Usage:
#   ai-specs recipe list [path]
#   ai-specs recipe add <id> [path]
#   ai-specs recipe init <id> [path]
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_SPECS_HOME="$(cd "$SCRIPT_DIR/.." && pwd)"
LIB_DIR="$AI_SPECS_HOME/lib"

usage() {
    cat <<'EOF'
Usage: ai-specs recipe <subcommand> [args]
Subcommands:
  list [path]   List available and installed recipes
  add <id> [path]  Add a recipe to the manifest
  init <id> [path] Print a read-only initialization brief
Path defaults to current directory.
EOF
}

# First arg after "recipe" is the subcommand
subcmd="${1:-}"
shift || true

case "$subcmd" in
    list) bash "$LIB_DIR/recipe-list.sh" "$@" ;;
    add) bash "$LIB_DIR/recipe-add.sh" "$@" ;;
    init) bash "$LIB_DIR/recipe-init.sh" "$@" ;;
    --help|-h|help) usage; exit 0 ;;
    "") usage >&2; exit 2 ;;
    *) echo "ai-specs recipe: unknown subcommand '$subcmd'" >&2
       echo "Run 'ai-specs recipe --help' for usage." >&2
       exit 1 ;;
esac
