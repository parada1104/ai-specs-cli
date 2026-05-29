#!/usr/bin/env bash
# skills.sh — sub-dispatcher for skills commands.
#
# Usage:
#   ai-specs skills add <url> [path] [flags]
#   ai-specs skills list [path]
#   ai-specs skills remove <id> [path]
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_SPECS_HOME="${AI_SPECS_HOME:-$(cd "$SCRIPT_DIR/.." && pwd)}"
LIB_DIR="$AI_SPECS_HOME/lib"

usage() {
    cat <<'EOF'
Usage: ai-specs skills <subcommand> [args]
Subcommands:
  add <url> [path] [flags]   Register a vendored skill ([[deps]]) and sync
                             Flags: --id, --subdir, --scope, --license,
                                    --attribution, --trigger, --no-sync
  list [path]                List registered and installed skills
  remove <id> [path]         Remove a vendored skill from the manifest
Path defaults to current directory. Run 'ai-specs skills add --help' for
full add flags.
EOF
}

subcmd="${1:-}"
shift || true

case "$subcmd" in
    add) bash "$LIB_DIR/skills-add.sh" "$@" ;;
    list) bash "$LIB_DIR/skills-list.sh" "$@" ;;
    remove) bash "$LIB_DIR/skills-remove.sh" "$@" ;;
    --help|-h|help) usage; exit 0 ;;
    "") usage >&2; exit 2 ;;
    *) echo "ai-specs skills: unknown subcommand '$subcmd'" >&2
       echo "Run 'ai-specs skills --help' for usage." >&2
       exit 1 ;;
esac
