#!/usr/bin/env bash
# refresh-bundled.sh — flatten CLI-bundled skills & commands into the cache.
#
# Usage:
#   ai-specs refresh-bundled [path] [--init]
#
# Behavior: a pure cache-repair verb. Every CLI-bundled skill and command is
# flattened into {cache}/.bundled/skills/ and {cache}/.bundled/commands/
# respectively — never written into the project surface, never a `.new`
# sidecar. A pre-relocation project's committed bundled-skill/command copies
# (byte-identical to the CLI source, or matching a legacy lock hash) are
# removed as leftovers; genuinely customized or hand-authored files are kept.
#
# The lock (<path>/ai-specs/.ai-specs.lock) is stamped with [meta] provenance
# only — no per-file content hashes are tracked.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_SPECS_HOME="$(cd "$SCRIPT_DIR/.." && pwd)"

usage() {
    cat <<'EOF'
Usage: ai-specs refresh-bundled [path] [--init]

Flatten CLI-bundled skills and commands into the cache
({cache}/.bundled/skills/, {cache}/.bundled/commands/). Pure cache repair:
zero in-project writes, zero .new sidecars. Removes pre-relocation leftover
copies from ai-specs/skills/ and ai-specs/commands/ when byte-identical to
the bundled source (or a legacy lock hash); customized or hand-authored
files are kept.

Arguments:
  path      Project root (default: current directory)

Flags:
  --init    (internal) accepted for backward-compatible invocation from
            init.sh; flatten-only has no behavior to change for first install.
  -h, --help

Examples:
  ai-specs refresh-bundled
  ai-specs refresh-bundled ~/code/my-app
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
python3 "$REFRESH_PY" "$TARGET_PATH" "$AI_SPECS_HOME" $INIT_FLAG
python3 "$AI_SPECS_HOME/lib/_internal/cli_version.py" stamp-meta "$TARGET_PATH" "$AI_SPECS_HOME"
