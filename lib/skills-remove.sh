#!/usr/bin/env bash
# skills-remove.sh — remove a vendored skill from ai-specs.toml.
#
# Usage:
#   ai-specs skills remove <id> [path] [--help]
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_SPECS_HOME="${AI_SPECS_HOME:-$(cd "$SCRIPT_DIR/.." && pwd)}"

usage() {
    cat <<'EOF'
Usage: ai-specs skills remove <id> [path] [--help]
Remove a vendored skill ([[deps]]) from ai-specs.toml.

The on-disk skill directory under ai-specs/skills/<id>/ is preserved.
Run 'ai-specs sync' afterwards to clean up if needed.

Arguments:
  id      Skill identifier matching [[deps]].id
  path    Target project root (default: current directory)
EOF
}

DEP_ID=""
TARGET_PATH=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --help|-h) usage; exit 0 ;;
        --) shift; break ;;
        -*) echo "ERROR: unknown flag: $1" >&2
            echo "Run 'ai-specs skills remove --help' for usage." >&2
            exit 2 ;;
        *)  if [[ -z "$DEP_ID" ]]; then
                DEP_ID="$1"
            elif [[ -z "$TARGET_PATH" ]]; then
                TARGET_PATH="$1"
            else
                echo "ERROR: unexpected positional argument: $1" >&2
                exit 2
            fi
            shift ;;
    esac
done

if [[ -z "$DEP_ID" ]]; then
    echo "ERROR: missing skill id" >&2
    usage >&2
    exit 2
fi

[[ -z "$TARGET_PATH" ]] && TARGET_PATH="$(pwd)"
TARGET_PATH="$(cd "$TARGET_PATH" && pwd)"
TOML_PATH="$TARGET_PATH/ai-specs/ai-specs.toml"

if [[ ! -f "$TOML_PATH" ]]; then
    echo "ERROR: $TOML_PATH not found." >&2
    exit 1
fi

# Remove the [[deps]] block matching the given id using Python
python3 - "$TOML_PATH" "$DEP_ID" <<'PY'
import sys, pathlib, re

toml_path = sys.argv[1]
dep_id = sys.argv[2]

p = pathlib.Path(toml_path)
content = p.read_text()

# Match a [[deps]] block whose id line matches the given dep_id.
# Pattern: optional blank lines + [[deps]] + any lines until the next
# section header ([[...]] or [section]) or end of file.
pattern = re.compile(
    r'\n*\[\[deps\]\]\n(?:[^[\n]*\n)*id\s*=\s*"' + re.escape(dep_id) + r'"\n(?:[^[\n]*\n)*',
    re.MULTILINE
)

new_content, count = pattern.subn('\n', content, count=1)

if count == 0:
    print(f"  ✗ dep '{dep_id}' not found in {toml_path}", file=sys.stderr)
    sys.exit(1)

p.write_text(new_content)
print(f"  ✓ removed [[deps]] '{dep_id}' from {toml_path}")
PY
