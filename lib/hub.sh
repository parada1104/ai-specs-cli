#!/usr/bin/env bash
# hub.sh — interactive front door for bare `ai-specs`.
#
# Usage: ai-specs hub [path] [--help]
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_SPECS_HOME="$(cd "$SCRIPT_DIR/.." && pwd)"
HUB_PY="$AI_SPECS_HOME/lib/_internal/hub.py"

usage() {
    cat <<'USAGE'
Usage: ai-specs hub [path] [--help]
Open the interactive ai-specs hub: project status + command menu.
With no TTY, prints a non-interactive status summary (initialized) or errors (uninitialized).
Arguments:
  path    Target project root (default: current directory)
Flags:
  --help  Show this help
USAGE
}

TARGET_PATH=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --help|-h) usage; exit 0 ;;
        --) shift; break ;;
        -*) echo "ERROR: unknown flag: $1" >&2
            echo "Run 'ai-specs hub --help' for usage." >&2
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

# Bash-level guard for the one state that must never launch Python:
# uninitialized + no TTY. Guarantees no hang, no deps requirement, fast exit 2
# in CI/pipes. All other states are decided authoritatively by hub.py.
if [[ ! -t 0 || ! -t 1 ]]; then
    if [[ ! -f "$TARGET_PATH/ai-specs/ai-specs.toml" ]]; then
        echo "ERROR: no ai-specs project at $TARGET_PATH" >&2
        echo "Run 'ai-specs init' to create one." >&2
        exit 2
    fi
fi

exec python3 "$HUB_PY" "$TARGET_PATH"
