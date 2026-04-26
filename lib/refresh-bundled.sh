#!/usr/bin/env bash
# refresh-bundled.sh — refresh bundled skills & commands, respecting user edits.
#
# Usage:
#   ai-specs refresh-bundled [path] [--preset NAME]
#
# Behavior (per bundled file):
#   - Untouched by user → auto-update to the latest CLI version
#   - Customized by user → drop the latest version as <name>.new alongside
#   - Deleted by user    → respected (removed from lock, not re-installed)
#
# A baseline of SHA-256 hashes is kept at <path>/ai-specs/.ai-specs.lock.
# Commit it with the rest of the project.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_SPECS_HOME="$(cd "$SCRIPT_DIR/.." && pwd)"

usage() {
    cat <<'EOF'
Usage: ai-specs refresh-bundled [path] [--preset NAME]

Refresh bundled skills and commands from the CLI, preserving user edits.

Arguments:
  path      Project root (default: current directory)

Flags:
  --init    (internal) First-time lock setup — do NOT write .new sidecars.
  --preset  Subset refresh (e.g. openspec for SDD enable); see refresh-bundled.py.
  -h, --help

Examples:
  ai-specs refresh-bundled
  ai-specs refresh-bundled ~/code/my-app
  ai-specs refresh-bundled --preset openspec
EOF
}

TARGET_PATH=""
INIT_FLAG=""
PRESET_ARGS=()
while [[ $# -gt 0 ]]; do
    case "$1" in
        --init)     INIT_FLAG="--init"; shift ;;
        --preset)
            PRESET_ARGS=(--preset "$2")
            shift 2
            ;;
        -h|--help)  usage; exit 0 ;;
        --)         shift; break ;;
        -*)
            echo "ERROR: unknown flag: $1" >&2
            echo "Run 'ai-specs refresh-bundled --help' for usage." >&2
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

REFRESH_PY="$AI_SPECS_HOME/lib/_internal/refresh-bundled.py"

echo ""
echo "ai-specs refresh-bundled"
echo "  target: $TARGET_PATH"
echo ""
python3 "$REFRESH_PY" "$TARGET_PATH" "$AI_SPECS_HOME" $INIT_FLAG "${PRESET_ARGS[@]}"
