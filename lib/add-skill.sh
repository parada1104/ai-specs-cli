#!/usr/bin/env bash
# add-skill.sh — scaffold a new local skill in <path>/ai-specs/skills/<name>/.
#
# Local skills are autodiscovered: they live in this repo's source tree and
# are NOT listed in ai-specs.toml. After scaffolding, this script runs
# skill-sync's sync.sh to refresh the AGENTS.md auto-invoke table.
#
# Usage:
#   specs-ai add-skill <name> [path] [--description <text>] [--trigger <text>]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SPECS_AI_HOME="$(cd "$SCRIPT_DIR/.." && pwd)"

usage() {
    cat <<'EOF'
Usage: specs-ai add-skill <name> [path] [--description <text>] [--trigger <text>]

Scaffold a new local skill at <path>/ai-specs/skills/<name>/SKILL.md.

Arguments:
  name                Skill name (kebab-case, e.g. my-feature)
  path                Project root (default: current directory)

Flags:
  --description <s>   One-line description (replaces {Brief description...})
  --trigger <s>       Auto-invoke trigger phrase (replaces {Primary action...})

After scaffolding, runs skill-sync to regenerate the AGENTS.md auto-invoke table.
EOF
}

NAME=""
TARGET_PATH=""
DESCRIPTION=""
TRIGGER=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --description)   DESCRIPTION="${2:-}"; shift 2 ;;
        --description=*) DESCRIPTION="${1#*=}"; shift ;;
        --trigger)       TRIGGER="${2:-}"; shift 2 ;;
        --trigger=*)     TRIGGER="${1#*=}"; shift ;;
        -h|--help)       usage; exit 0 ;;
        --)              shift; break ;;
        -*)
            echo "ERROR: unknown flag: $1" >&2
            echo "Run 'specs-ai add-skill --help' for usage." >&2
            exit 2
            ;;
        *)
            if [[ -z "$NAME" ]]; then
                NAME="$1"
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

if [[ -z "$NAME" ]]; then
    echo "ERROR: <name> is required." >&2
    usage
    exit 2
fi

if ! [[ "$NAME" =~ ^[a-z][a-z0-9_-]*$ ]]; then
    echo "ERROR: skill name must be kebab-case (lowercase, alnum/-/_): $NAME" >&2
    exit 2
fi

[[ -z "$TARGET_PATH" ]] && TARGET_PATH="$(pwd)"
TARGET_PATH="$(cd "$TARGET_PATH" && pwd)"

AI_SPECS_DIR="$TARGET_PATH/ai-specs"
SKILL_DIR="$AI_SPECS_DIR/skills/$NAME"
TEMPLATE="$AI_SPECS_DIR/skills/skill-creator/assets/SKILL-TEMPLATE.md"
SYNC_SH="$AI_SPECS_DIR/skills/skill-sync/assets/sync.sh"

if [[ ! -f "$TEMPLATE" ]]; then
    echo "ERROR: $TEMPLATE not found." >&2
    echo "       Run 'specs-ai init $TARGET_PATH' first." >&2
    exit 1
fi
if [[ -e "$SKILL_DIR" ]]; then
    echo "ERROR: skill already exists at $SKILL_DIR" >&2
    exit 1
fi

[[ -z "$DESCRIPTION" ]] && DESCRIPTION="TODO: describe what this skill enables."
[[ -z "$TRIGGER" ]]     && TRIGGER="TODO: when AGENTS.md should auto-invoke this skill"

mkdir -p "$SKILL_DIR/assets"

# Render template with sed (safe: substitution values are user-provided strings).
# We use Python for the substitution to avoid sed escape pitfalls with arbitrary text.
python3 - "$TEMPLATE" "$SKILL_DIR/SKILL.md" "$NAME" "$DESCRIPTION" "$TRIGGER" <<'PY'
import sys, pathlib
src, dst, name, description, trigger = sys.argv[1:6]
content = pathlib.Path(src).read_text()
content = content.replace("{skill-name}", name)
content = content.replace("{Brief description of what this skill enables}", description)
content = content.replace("{Primary action phrase for AGENTS.md sync}", trigger)
pathlib.Path(dst).write_text(content)
PY

echo "  ✓ scaffolded $SKILL_DIR/SKILL.md"

# Refresh AGENTS.md auto-invoke table
if [[ -f "$SYNC_SH" ]]; then
    echo "  ▸ skill-sync (refresh AGENTS.md)"
    bash "$SYNC_SH"
else
    echo "  ! skill-sync not installed at $SYNC_SH — skipping AGENTS.md refresh"
fi

cat <<EOF

✓ skill '$NAME' created.

Next:
  1. Edit $SKILL_DIR/SKILL.md
       - tighten the description / trigger
       - fill in 'When to Use', 'Critical Patterns', 'Commands'
  2. Add any helper files under $SKILL_DIR/assets/
  3. Commit ai-specs/skills/$NAME/  (local skills are committed)
EOF
