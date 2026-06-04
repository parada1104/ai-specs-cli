#!/usr/bin/env bash
# rules-audit.sh — read-only legacy rules inventory for ai-specs migration planning.
#
# Usage:
# ai-specs rules-audit [path] [--help]
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_SPECS_HOME="$(cd "$SCRIPT_DIR/.." && pwd)"
RULES_AUDIT_PY="$AI_SPECS_HOME/lib/_internal/rules-inventory.py"
usage() {
    cat <<'EOF'
Usage: ai-specs rules-audit [path] [--help]
Inventory legacy Cursor rules and emit a JSON migration inventory.
This command is read-only and never modifies any files.
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
    echo "Run 'ai-specs rules-audit --help' for usage." >&2
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
if [[ ! -d "$TARGET_PATH" ]]; then
    echo "ERROR: not a directory: $TARGET_PATH" >&2
    exit 2
fi
TARGET_PATH="$(cd "$TARGET_PATH" && pwd)"
exec python3 "$RULES_AUDIT_PY" "$TARGET_PATH"
