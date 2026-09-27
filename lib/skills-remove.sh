#!/usr/bin/env bash
# skills-remove.sh — remove a vendored skill from ai-specs.toml.
#
# Usage:
#   ai-specs skills remove <id> [path] [--help]
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

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

# Remove the [[deps]] block matching the given id using Python.
#
# Strategy (stdlib only, text-based, robust): split the manifest into segments
# delimited by lines that START with '[' at column 0 — these are TOML
# section/table headers. Array VALUE lines like `scope = ["root"]` never start
# with '[' at column 0, so they stay attached to their owning block. We drop
# exactly the one [[deps]] segment whose id matches the target.
#
# Everything outside the removed segment is preserved byte-for-byte (no
# whole-file whitespace normalization), the result is validated before writing
# (a deletion that would break a previously valid manifest is refused), and
# the write is atomic (mkstemp + os.replace, mirroring lib/_internal/lock.py).
python3 - "$TOML_PATH" "$DEP_ID" <<'PY'
import os
import pathlib
import re
import sys
import tempfile
import tomllib

toml_path = sys.argv[1]
dep_id = sys.argv[2]

p = pathlib.Path(toml_path)
original = p.read_text(encoding="utf-8")
lines = original.splitlines(keepends=True)

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

# Validate the result before writing (mirrors recipe-config-write.py): a
# deletion that would break a previously valid manifest is refused and the
# original bytes stay untouched. Removal must still tolerate a manifest that
# is NOT currently valid TOML (frozen parity contract): only the regression
# from valid to invalid is guarded.
try:
    tomllib.loads(original)
    original_was_valid = True
except tomllib.TOMLDecodeError:
    original_was_valid = False
if original_was_valid:
    try:
        tomllib.loads(new_content)
    except tomllib.TOMLDecodeError as exc:
        print(
            f"ERROR: removing skill '{dep_id}' would produce invalid TOML: {exc}",
            file=sys.stderr,
        )
        sys.exit(1)

# Atomic replace (mirrors lock.py): a failed write never leaves a partially
# updated manifest behind, and the original file mode is preserved.
original_mode = p.stat().st_mode & 0o7777
fd, tmp = tempfile.mkstemp(dir=str(p.parent), prefix=".ai-specs.toml.", suffix=".tmp")
try:
    with os.fdopen(fd, "w", encoding="utf-8") as fh:
        fh.write(new_content)
    os.chmod(tmp, original_mode)
    os.replace(tmp, p)
except BaseException:
    try:
        os.unlink(tmp)
    except OSError:
        pass
    raise

print(f"  ✓ removed [[deps]] '{dep_id}' from {toml_path}")
PY
