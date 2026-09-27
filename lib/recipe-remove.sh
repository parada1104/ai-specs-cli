#!/usr/bin/env bash
# recipe-remove.sh — remove a recipe section from ai-specs.toml.
#
# Usage:
#   ai-specs recipe remove <id> [path] [--help]
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

usage() {
    cat <<'EOF'
Usage: ai-specs recipe remove <id> [path] [--help]
Remove a recipe ([recipes.<id>]) from ai-specs.toml.
Arguments:
  id      Recipe identifier to remove
  path    Target project root (default: current directory)
Flags:
  --help  Show this help
EOF
}

RECIPE_ID=""
TARGET_PATH=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --help|-h) usage; exit 0 ;;
        --) shift; break ;;
        -*) echo "ERROR: unknown flag: $1" >&2
            echo "Run 'ai-specs recipe remove --help' for usage." >&2
            exit 2 ;;
        *)  if [[ -z "$RECIPE_ID" ]]; then
                RECIPE_ID="$1"
            elif [[ -z "$TARGET_PATH" ]]; then
                TARGET_PATH="$1"
            else
                echo "ERROR: unexpected positional argument: $1" >&2
                exit 2
            fi
            shift ;;
    esac
done

if [[ -z "$RECIPE_ID" ]]; then
    echo "ERROR: missing recipe id" >&2
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

# Remove the [recipes.<id>] section (and any sub-tables like [recipes.<id>.config])
# using the same text-based segment approach as skills-remove.sh.
#
# Strategy: split the manifest into segments delimited by lines that START with
# '[' at column 0. Drop any segment whose header matches [recipes.<id>] or
# [recipes.<id>.*] (sub-tables), including all lines until the next top-level
# header.
#
# Everything outside the removed segments is preserved byte-for-byte (no
# whole-file whitespace normalization), the result is validated before writing
# (a deletion that would break a previously valid manifest is refused), and
# the write is atomic (mkstemp + os.replace, mirroring lib/_internal/lock.py).
python3 - "$TOML_PATH" "$RECIPE_ID" <<'PY'
import os
import pathlib
import re
import sys
import tempfile
import tomllib

toml_path = sys.argv[1]
recipe_id = sys.argv[2]

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

# Match [recipes.<id>] or any of its sub-tables like [recipes.<id>.config]
pattern = re.compile(
    r'^\s*\[\s*recipes\s*\.\s*' + re.escape(recipe_id) + r'(\s*[.\]]|\s*$)',
    re.MULTILINE,
)

target_indices = []
for i, seg in enumerate(segments):
    header = seg["header"] or ""
    # TOML allows comments: [recipes.foo] # comment
    stripped = header.strip()
    if pattern.search(stripped):
        target_indices.append(i)

if not target_indices:
    print(f"  ✗ recipe '{recipe_id}' not found in {toml_path}", file=sys.stderr)
    sys.exit(1)

# Remove matching segments in reverse index order (preserves earlier indices).
for idx in reversed(target_indices):
    del segments[idx]

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
            f"ERROR: removing recipe '{recipe_id}' would produce invalid TOML: {exc}",
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

print(f"  ✓ removed {len(target_indices)} section(s) for recipe '{recipe_id}' from {toml_path}")
PY

# Also clean up stale lock entries for this recipe.
LOCK_PATH="$TARGET_PATH/ai-specs/.ai-specs.lock"
if [[ -f "$LOCK_PATH" ]]; then
    # Delegate to lock.py, the module that owns lock writes: canonical
    # sections, _toml_string escaping, single LOCK_HEADER, the Go
    # worktree-gate --write-lock authority, and an atomic mkstemp+os.replace
    # write. Unrelated sections ([managed.*], [agents.*]) are preserved; only
    # this recipe's entries are removed, and the lock is only rewritten when
    # something was actually removed.
    python3 - "$SCRIPT_DIR/_internal" "$LOCK_PATH" "$RECIPE_ID" <<'PY'
import importlib.util
import sys
from pathlib import Path

internal_dir = Path(sys.argv[1])
lock_path = Path(sys.argv[2])
recipe_id = sys.argv[3]

spec = importlib.util.spec_from_file_location(
    "lock_internal", internal_dir / "lock.py"
)
if spec is None or spec.loader is None:
    print(
        f"ERROR: unable to load lock module from {internal_dir / 'lock.py'}",
        file=sys.stderr,
    )
    sys.exit(1)
lock_mod = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = lock_mod
spec.loader.exec_module(lock_mod)

lock = lock_mod.load_lock(lock_path)
if lock_mod.remove_recipe_lock_entries(lock, recipe_id):
    lock_mod.write_lock(lock_path, lock)
    print(f"  ✓ cleaned lock entries for recipe '{recipe_id}'")
PY
fi
