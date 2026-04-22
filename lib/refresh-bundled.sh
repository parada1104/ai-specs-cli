#!/usr/bin/env bash
# refresh-bundled.sh — refresh bundled skills & commands, respecting user edits.
#
# Usage:
#   specs-ai refresh-bundled [path]
#
# Behavior (per bundled file):
#   - Untouched by user → auto-update to the latest CLI version
#   - Customized by user → drop the latest version as <name>.new alongside
#   - Deleted by user    → respected (removed from lock, not re-installed)
#
# A baseline of SHA-256 hashes is kept at <path>/ai-specs/.specs-ai.lock.
# Commit it with the rest of the project.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SPECS_AI_HOME="$(cd "$SCRIPT_DIR/.." && pwd)"

usage() {
    cat <<'EOF'
Usage: specs-ai refresh-bundled [path]

Refresh bundled skills and commands from the CLI, preserving user edits.

Arguments:
  path      Project root (default: current directory)

Flags:
  --init    (internal) First-time lock setup — do NOT write .new sidecars.
  -h, --help

Examples:
  specs-ai refresh-bundled
  specs-ai refresh-bundled ~/code/my-app
EOF
}

TARGET_PATH=""
INIT_FLAG=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --init)     INIT_FLAG="--init"; shift ;;
        -h|--help)  usage; exit 0 ;;
        --)         shift; break ;;
        -*)
            echo "ERROR: unknown flag: $1" >&2
            echo "Run 'specs-ai refresh-bundled --help' for usage." >&2
            exit 2
            ;;
        *)
            if [[ -z "$TARGET_PATH" ]]; then
                TARGET_PATH="$1"
            else
                echo "ERROR: unexpected positional argument: $1" >&2
                exit 2
            fi
            shift
            ;;
    esac
done

[[ -z "$TARGET_PATH" ]] && TARGET_PATH="$(pwd)"
if [[ ! -d "$TARGET_PATH" ]]; then
    echo "ERROR: target path does not exist: $TARGET_PATH" >&2
    exit 1
fi
TARGET_PATH="$(cd "$TARGET_PATH" && pwd)"

REFRESH_PY="$SPECS_AI_HOME/lib/_internal/refresh-bundled.py"

echo ""
echo "specs-ai refresh-bundled"
echo "  target: $TARGET_PATH"
echo ""
python3 "$REFRESH_PY" "$TARGET_PATH" "$SPECS_AI_HOME" $INIT_FLAG
