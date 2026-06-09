#!/usr/bin/env bash
# skills-list.sh — list registered and installed skills.
#
# Usage:
#   ai-specs skills list [path] [--help]
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_SPECS_HOME="${AI_SPECS_HOME:-$(cd "$SCRIPT_DIR/.." && pwd)}"

usage() {
    cat <<'EOF'
Usage: ai-specs skills list [path] [--help]
List vendored ([[deps]]) and local skills for a project.

Shows:
  - Installed deps from ai-specs.toml
  - Local skills in ai-specs/skills/ (excluding vendored)
  - Available catalog skills shipped with the CLI
EOF
}

TARGET_PATH=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --help|-h) usage; exit 0 ;;
        --) shift; break ;;
        -*) echo "ERROR: unknown flag: $1" >&2
            echo "Run 'ai-specs skills list --help' for usage." >&2
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
TOML_PATH="$TARGET_PATH/ai-specs/ai-specs.toml"
SKILLS_DIR="$TARGET_PATH/ai-specs/skills"
CATALOG_DIR="$AI_SPECS_HOME/catalog/skills"

# Extract the leading front-matter `description:` from a SKILL.md (stdlib only).
skill_description() {
    python3 - "$1" <<'PY'
import sys, re
try:
    text = open(sys.argv[1], encoding="utf-8").read()
except OSError:
    sys.exit(0)
m = re.match(r'^---\s*\n(.*?)\n---', text, re.DOTALL)
if not m:
    sys.exit(0)
front = m.group(1)
dm = re.search(r'^description:\s*(.+?)\s*$', front, re.MULTILINE)
if dm:
    desc = dm.group(1).strip()
    # Handle YAML quoted strings
    if desc.startswith('"') and desc.endswith('"'):
        desc = desc[1:-1].replace('\\"', '"').replace('\\\\', '\\')
    elif desc.startswith("'") and desc.endswith("'"):
        desc = desc[1:-1]
    if desc:
        print(desc[:80])
PY
}

echo "=== ai-specs skills list ==="
echo "Project: $TARGET_PATH"
echo ""

# ── Registered deps from ai-specs.toml ──
echo "── Registered deps ([[deps]]) ──"
if [[ -f "$TOML_PATH" ]]; then
    python3 - "$TOML_PATH" "$SKILLS_DIR" <<'PY'
import sys, tomllib, pathlib

toml_path = sys.argv[1]
skills_dir = pathlib.Path(sys.argv[2])

try:
    with open(toml_path, "rb") as f:
        data = tomllib.load(f)
except tomllib.TOMLDecodeError as e:
    print(f"  ✗ manifest invalid: {e}", file=sys.stderr)
    print("  (skipped — fix ai-specs.toml to list deps)")
    sys.exit(0)

deps = data.get("deps", []) or []
if not deps:
    print("  (none)")
else:
    for dep in deps:
        dep_id = dep.get("id", "?")
        source = dep.get("source", "?")
        subdir = dep.get("path", "")
        installed = (skills_dir / dep_id).is_dir()
        status = "✓ installed" if installed else "✗ not synced"
        print(f"  {dep_id}")
        print(f"    source:     {source}")
        if subdir:
            print(f"    subdir:     {subdir}")
        print(f"    status:     {status}")
        license_ = dep.get("license", "")
        if license_:
            print(f"    license:    {license_}")
        print()
PY
else
    echo "  (ai-specs.toml not found — run 'ai-specs init' first)"
fi
echo ""

# ── Local skills in ai-specs/skills/ ──
echo "── Local skills (ai-specs/skills/) ──"
if [[ -d "$SKILLS_DIR" ]]; then
    # Collect registered dep IDs to exclude from local skills listing
    REGISTERED_IDS=()
    if [[ -f "$TOML_PATH" ]]; then
        while IFS='"' read -r _ id _; do
            [[ -n "$id" ]] && REGISTERED_IDS+=("$id")
        done < <(grep -E '^id = "' "$TOML_PATH" 2>/dev/null || true)
    fi
    has_entries=0
    for d in "$SKILLS_DIR"/*/; do
        [[ -d "$d" ]] || continue
        name="$(basename "$d")"
        # Skip synced vendored deps (they appear in the deps section above)
        for rid in "${REGISTERED_IDS[@]}"; do
            [[ "$name" == "$rid" ]] && continue 2
        done
        has_entries=1
        if [[ -f "$d/SKILL.md" ]]; then
            desc="$(skill_description "$d/SKILL.md")"
            echo "  $name"
            if [[ -n "$desc" ]]; then
                echo "    $desc"
            fi
        else
            echo "  $name  (no SKILL.md)"
        fi
    done
    if [[ $has_entries -eq 0 ]]; then
        echo "  (empty)"
    fi
else
    echo "  (not found)"
fi
echo ""

# ── Available catalog skills ──
echo "── Available catalog skills (catalog/skills/) ──"
if [[ -d "$CATALOG_DIR" ]]; then
    count=0
    for d in "$CATALOG_DIR"/*/; do
        [[ -d "$d" ]] || continue
        name="$(basename "$d")"
        if [[ -f "$d/SKILL.md" ]]; then
            desc="$(skill_description "$d/SKILL.md")"
            echo "  $name"
            if [[ -n "$desc" ]]; then
                echo "    $desc"
            fi
            count=$((count + 1))
        fi
    done
    if [[ $count -eq 0 ]]; then
        echo "  (empty)"
    fi
else
    echo "  (not found)"
fi
