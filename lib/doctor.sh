#!/usr/bin/env bash
# doctor.sh — read-only diagnostic for ai-specs projects.
#
# Usage:
# ai-specs doctor [path] [--help]
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_SPECS_HOME="$(cd "$SCRIPT_DIR/.." && pwd)"
DOCTOR_PY="$AI_SPECS_HOME/lib/_internal/doctor.py"
usage() {
    cat <<'EOF'
Usage: ai-specs doctor [path] [--help]
Diagnose whether an ai-specs project is correctly initialized and in a
consistent state. This command is read-only and never modifies any files.
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
    echo "Run 'ai-specs doctor --help' for usage." >&2
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
exec python3 "$DOCTOR_PY" "$TARGET_PATH"