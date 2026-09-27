#!/usr/bin/env bash
# doctor.sh — project health diagnostic for ai-specs projects.
#
# Diagnostics include template-override ownership and generated gate-hook
# provenance (baseline match → quiet; byte mismatch or missing provenance →
# WARN with refresh guidance). The command never modifies project files, but
# it is not fully read-only: it writes Python bytecode caches into the CLI
# home and executes recipe dep version checks plus the gate binary selftest.
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
consistent state. Never modifies project files; runs external version
checks (recipe deps, gate binary selftest).
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