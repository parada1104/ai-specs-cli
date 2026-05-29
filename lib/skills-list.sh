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

with open(toml_path, "rb") as f:
    data = tomllib.load(f)

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
    count=0
    for d in "$SKILLS_DIR"/*/; do
        [[ -d "$d" ]] || continue
        name="$(basename "$d")"
        if [[ -f "$d/SKILL.md" ]]; then
            desc="$(head -20 "$d/SKILL.md" | python3 -c "
import sys, re, yaml
text = sys.stdin.read()
m = re.search(r'^---\s*\n(.*?)\n---', text, re.DOTALL)
if m:
    try:
        fm = yaml.safe_load(m.group(1))
        if fm and 'description' in fm:
            d = fm['description']
            if isinstance(d, str):
                print(d.split(chr(10))[0][:80])
            elif isinstance(d, list):
                print(d[0][:80])
    except: pass
" 2>/dev/null || true)"
            echo "  $name"
            if [[ -n "$desc" ]]; then
                echo "    $desc"
            fi
            count=$((count + 1))
        else
            echo "  $name  (no SKILL.md)"
        fi
    done
    if [[ $count -eq 0 ]]; then
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
            desc="$(head -20 "$d/SKILL.md" | python3 -c "
import sys, re, yaml
text = sys.stdin.read()
m = re.search(r'^---\s*\n(.*?)\n---', text, re.DOTALL)
if m:
    try:
        fm = yaml.safe_load(m.group(1))
        if fm and 'description' in fm:
            d = fm['description']
            if isinstance(d, str):
                print(d.split(chr(10))[0][:80])
            elif isinstance(d, list):
                print(d[0][:80])
    except: pass
" 2>/dev/null || true)"
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
