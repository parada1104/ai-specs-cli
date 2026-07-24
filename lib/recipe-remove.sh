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
python3 - "$TOML_PATH" "$RECIPE_ID" <<'PY'
import sys, pathlib, re

toml_path = sys.argv[1]
recipe_id = sys.argv[2]

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
    print(f"  \u2717 recipe '{recipe_id}' not found in {toml_path}", file=sys.stderr)
    sys.exit(1)

# Remove matching segments in reverse index order (preserves earlier indices)
for idx in reversed(target_indices):
    del segments[idx]

new_content = "".join("".join(seg["lines"]) for seg in segments)

# Collapse 3+ consecutive newlines left by removal into a single blank line.
new_content = re.sub(r"\n{3,}", "\n\n", new_content)

p.write_text(new_content)

print(f"  \u2713 removed {len(target_indices)} section(s) for recipe '{recipe_id}' from {toml_path}")
PY

# Also clean up stale lock entries for this recipe.
LOCK_PATH="$TARGET_PATH/ai-specs/.ai-specs.lock"
if [[ -f "$LOCK_PATH" ]]; then
    python3 - "$LOCK_PATH" "$RECIPE_ID" <<'PY'
import sys, pathlib, tomllib

lock_path = sys.argv[1]
recipe_id = sys.argv[2]

lock_path = pathlib.Path(lock_path)
content = lock_path.read_bytes()
data = tomllib.loads(content.decode("utf-8"))

recipes = data.get("recipes") or {}
if recipe_id not in recipes:
    sys.exit(0)

del recipes[recipe_id]

# Re-serialize as a provenance stamp (mirrors lock.py:write_lock). Skill/recipe/
# dep content hashes, and legacy [commands]/[opted-out] sections, are no
# longer tracked; any such legacy sections are dropped here.
out = []
out.append("# Managed by ai-specs. Do not edit by hand.")
out.append("# Provenance stamp: [meta] records the CLI version and last sync.")
out.append("# git covers integrity of the committed surface; skill/recipe/dep")
out.append("# hashes are not tracked.")
out.append("")

meta = data.get("meta") or {}
if meta:
    out.append("[meta]")
    if meta.get("cli_version"):
        out.append(f'cli_version = "{meta["cli_version"]}"')
    if meta.get("synced_at"):
        out.append(f'synced_at = "{meta["synced_at"]}"')
    out.append("")

agents = data.get("agents") or {}
for harness in sorted(agents):
    files = agents[harness]
    if not files:
        continue
    out.append(f'[agents."{harness}"]')
    for name in sorted(files):
        out.append(f'"{name}" = "{files[name]}"')
    out.append("")

lock_path.write_text("\n".join(out).rstrip("\n") + "\n")
print(f"  \u2713 cleaned lock entries for recipe '{recipe_id}'")
PY
fi
