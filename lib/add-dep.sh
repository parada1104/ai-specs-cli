#!/usr/bin/env bash
# add-dep.sh — register a new vendored skill in ai-specs.toml [[deps]] and sync.
#
# The vendored skill is later cloned into <path>/ai-specs/skills/<id>/ by
# `lib/_internal/vendor-skills.py` (which `specs-ai sync` runs).
#
# Usage:
#   specs-ai add-dep <git-url> [path]
#                    [--id <id>]                 (default: derived from URL)
#                    [--subdir <subpath>]        (subdir within the repo where SKILL.md lives)
#                    [--scope <s1,s2,...>]       (default: root)
#                    [--license <license>]       (default: empty)
#                    [--attribution <author>]    (default: derived from URL)
#                    [--trigger <text>]          (auto_invoke entry)
#                    [--no-sync]                 (skip 'specs-ai sync' at the end)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SPECS_AI_HOME="$(cd "$SCRIPT_DIR/.." && pwd)"

usage() {
    cat <<'EOF'
Usage: specs-ai add-dep <git-url> [path] [flags]

Register a vendored skill in ai-specs.toml (under [[deps]]) and run sync.

Arguments:
  git-url               Source URL (e.g. https://github.com/obra/superpowers)
  path                  Project root (default: current directory)

Flags:
  --id <id>             Skill ID (default: last URL path component, .git stripped)
  --subdir <subpath>    Subdir within the repo where SKILL.md lives (multi-skill repos)
  --scope <s1,s2,...>   Comma-list for metadata.scope (default: root)
  --license <license>   License string (default: empty)
  --attribution <auth>  vendor_attribution (default: URL author)
  --trigger <text>      auto_invoke entry (default: "When working on <id>")
  --no-sync             Don't run 'specs-ai sync' after registering
EOF
}

URL=""
TARGET_PATH=""
ID=""
SUBDIR=""
SCOPE="root"
LICENSE=""
ATTRIBUTION=""
TRIGGER=""
RUN_SYNC=1

while [[ $# -gt 0 ]]; do
    case "$1" in
        --id)              ID="${2:-}"; shift 2 ;;
        --id=*)            ID="${1#*=}"; shift ;;
        --subdir)          SUBDIR="${2:-}"; shift 2 ;;
        --subdir=*)        SUBDIR="${1#*=}"; shift ;;
        --scope)           SCOPE="${2:-}"; shift 2 ;;
        --scope=*)         SCOPE="${1#*=}"; shift ;;
        --license)         LICENSE="${2:-}"; shift 2 ;;
        --license=*)       LICENSE="${1#*=}"; shift ;;
        --attribution)     ATTRIBUTION="${2:-}"; shift 2 ;;
        --attribution=*)   ATTRIBUTION="${1#*=}"; shift ;;
        --trigger)         TRIGGER="${2:-}"; shift 2 ;;
        --trigger=*)       TRIGGER="${1#*=}"; shift ;;
        --no-sync)         RUN_SYNC=0; shift ;;
        -h|--help)         usage; exit 0 ;;
        --)                shift; break ;;
        -*)
            echo "ERROR: unknown flag: $1" >&2
            echo "Run 'specs-ai add-dep --help' for usage." >&2
            exit 2
            ;;
        *)
            if [[ -z "$URL" ]]; then
                URL="$1"
            elif [[ -z "$TARGET_PATH" ]]; then
                TARGET_PATH="$1"
            else
                echo "ERROR: unexpected positional argument: $1" >&2
                exit 2
            fi
            shift
            ;;
    esac
done

if [[ -z "$URL" ]]; then
    echo "ERROR: <git-url> is required." >&2
    usage
    exit 2
fi

[[ -z "$TARGET_PATH" ]] && TARGET_PATH="$(pwd)"
TARGET_PATH="$(cd "$TARGET_PATH" && pwd)"
TOML_PATH="$TARGET_PATH/ai-specs/ai-specs.toml"

if [[ ! -f "$TOML_PATH" ]]; then
    echo "ERROR: $TOML_PATH not found. Run 'specs-ai init $TARGET_PATH' first." >&2
    exit 1
fi

# Derive defaults from URL
url_basename="${URL##*/}"
url_basename="${url_basename%.git}"

[[ -z "$ID" ]] && ID="$url_basename"
[[ -z "$TRIGGER" ]] && TRIGGER="When working on ${ID}"

if [[ -z "$ATTRIBUTION" ]]; then
    # Derive from .../<author>/<repo>(.git)?
    no_proto="${URL#*://}"
    no_host="${no_proto#*/}"
    ATTRIBUTION="${no_host%%/*}"
fi

# Validate ID
if ! [[ "$ID" =~ ^[a-z0-9][a-z0-9_-]*$ ]]; then
    echo "ERROR: derived/provided id is not kebab-case: '$ID' — pass --id explicitly." >&2
    exit 2
fi

# Confirm not already registered
existing="$(python3 - "$TOML_PATH" "$ID" <<'PY'
import sys, tomllib
with open(sys.argv[1], "rb") as f:
    data = tomllib.load(f)
ids = [d.get("id") for d in (data.get("deps", []) or [])]
print("YES" if sys.argv[2] in ids else "NO")
PY
)"
if [[ "$existing" == "YES" ]]; then
    echo "ERROR: dep with id '$ID' already exists in $TOML_PATH" >&2
    exit 1
fi

echo ""
echo "specs-ai add-dep"
echo "  url:         $URL"
echo "  id:          $ID"
echo "  subdir:      ${SUBDIR:-(none)}"
echo "  scope:       $SCOPE"
echo "  license:     ${LICENSE:-(none)}"
echo "  attribution: $ATTRIBUTION"
echo ""

# Append [[deps]] block. Use Python to escape strings safely.
python3 - "$TOML_PATH" "$ID" "$URL" "$SUBDIR" "$SCOPE" "$TRIGGER" "$LICENSE" "$ATTRIBUTION" <<'PY'
import sys, pathlib

toml_path, dep_id, url, subdir, scope_csv, trigger, license_, attribution = sys.argv[1:9]

def s(x: str) -> str:
    return '"' + x.replace("\\", "\\\\").replace('"', '\\"') + '"'

scopes = [x.strip() for x in scope_csv.split(",") if x.strip()] or ["root"]

block = ["", "[[deps]]"]
block.append(f"id = {s(dep_id)}")
block.append(f"source = {s(url)}")
if subdir:
    block.append(f"path = {s(subdir)}")
block.append("scope = [" + ", ".join(s(x) for x in scopes) + "]")
if trigger:
    block.append(f"auto_invoke = [{s(trigger)}]")
if license_:
    block.append(f"license = {s(license_)}")
if attribution:
    block.append(f"vendor_attribution = {s(attribution)}")
block.append("")

p = pathlib.Path(toml_path)
content = p.read_text()
if not content.endswith("\n"):
    content += "\n"
p.write_text(content + "\n".join(block))
print(f"  ✓ appended [[deps]] for '{dep_id}' to {toml_path}")
PY

# Run sync (unless --no-sync)
if [[ $RUN_SYNC -eq 1 ]]; then
    echo ""
    echo "▸ specs-ai sync $TARGET_PATH"
    bash "$SPECS_AI_HOME/lib/sync.sh" "$TARGET_PATH"
else
    echo ""
    echo "✓ dep registered. Run 'specs-ai sync $TARGET_PATH' to vendor it."
fi
