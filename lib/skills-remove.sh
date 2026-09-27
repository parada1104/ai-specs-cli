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

Removes the [[deps]] block and prunes the vendored copy under
ai-specs/.deps/<id>/ (gitignored).

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

# Refuse ids that could escape the managed ai-specs/.deps tree (C2-style
# guard, mirroring hookRelPathEscapes in the gate): dep ids are kebab-case
# slugs, so refusing empty/dot/separator/control-char ids never rejects a
# real dep. A hand-edited manifest can carry a traversal id; removal must
# never turn it into an rm -rf outside .deps/.
invalid_dep_id() {
    local sanitized
    sanitized="$(printf '%s' "$1" | LC_ALL=C tr -c ' -~' '?')"
    echo "ERROR: refusing invalid dep id: '$sanitized'" >&2
    exit 2
}
case "$DEP_ID" in
    ""|"."|".."|*/*|*\\*) invalid_dep_id "$DEP_ID" ;;
esac
if printf '%s' "$DEP_ID" | LC_ALL=C grep -q '[^ -~]'; then
    invalid_dep_id "$DEP_ID"
fi

if [[ ! -f "$TOML_PATH" ]]; then
    echo "ERROR: $TOML_PATH not found." >&2
    exit 1
fi

# Remove the [[deps]] block matching the given id using Python.
#
# Strategy (stdlib only, text-based, robust): split the manifest into segments
# delimited by lines that START with '[' at column 0 — these are TOML
# section/table headers. Array VALUE lines like `scope = ["root"]` never start
# with '[' at column 0, so they stay attached to their owning block. We drop
# exactly the one [[deps]] segment whose id matches the target.
python3 - "$TOML_PATH" "$DEP_ID" <<'PY'
import sys, pathlib, re

toml_path = sys.argv[1]
dep_id = sys.argv[2]

p = pathlib.Path(toml_path)
content = p.read_text()
lines = content.splitlines(keepends=True)

# Build segments: a new segment begins at each line that starts with '[' at
# column 0. The text before the first header (preamble) is its own segment.
segments = []  # list of {"header": str|None, "lines": [str, ...]}
current = {"header": None, "lines": []}
for line in lines:
    if line.startswith("["):
        if current["lines"] or current["header"] is not None:
            segments.append(current)
        current = {"header": line, "lines": [line]}
    else:
        current["lines"].append(line)
if current["lines"] or current["header"] is not None:
    segments.append(current)

id_re = re.compile(r'^\s*id\s*=\s*"' + re.escape(dep_id) + r'"\s*$', re.MULTILINE)

target_idx = None
for i, seg in enumerate(segments):
    header = seg["header"] or ""
    # TOML allows inline comments after section headers: [[deps]] # comment
    if not header.strip().startswith("[[deps]]"):
        continue
    block_text = "".join(seg["lines"])
    if id_re.search(block_text):
        target_idx = i
        break

if target_idx is None:
    print(f"  ✗ dep '{dep_id}' not found in {toml_path}", file=sys.stderr)
    sys.exit(1)

del segments[target_idx]
new_content = "".join("".join(seg["lines"]) for seg in segments)

# Collapse 3+ consecutive newlines left by removal into a single blank line.
new_content = re.sub(r"\n{3,}", "\n\n", new_content)

p.write_text(new_content)
print(f"  ✓ removed [[deps]] '{dep_id}' from {toml_path}")
PY

# Prune the vendored copy (D18): the in-project .deps tree is regenerable
# from the declared source, so removal deletes it instead of orphaning it.
if [[ -d "$TARGET_PATH/ai-specs/.deps/$DEP_ID" ]]; then
    rm -rf "$TARGET_PATH/ai-specs/.deps/$DEP_ID"
    echo "  ✓ pruned ai-specs/.deps/$DEP_ID/"
fi

# Drop the dep's recorded content hashes from the lock (D17 schema).
python3 - "$TARGET_PATH/ai-specs/.ai-specs.lock" "$DEP_ID" \
    "$AI_SPECS_HOME/lib/_internal/lock.py" <<'PY'
import importlib.util
import sys
from pathlib import Path

lock_path = Path(sys.argv[1])
dep_id = sys.argv[2]
lock_module_path = Path(sys.argv[3])

if not lock_path.is_file():
    sys.exit(0)

spec = importlib.util.spec_from_file_location("lock_remove_dep", lock_module_path)
if spec is None or spec.loader is None:
    print(f"ERROR: unable to load lock.py at {lock_module_path}", file=sys.stderr)
    sys.exit(1)
lock_mod = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = lock_mod
spec.loader.exec_module(lock_mod)

lock = lock_mod.load_lock(lock_path)
if lock_mod.remove_dep_lock_entries(lock, dep_id):
    lock_mod.write_lock(lock_path, lock)
    print(f"  ✓ removed lock hashes for '{dep_id}'")
PY
