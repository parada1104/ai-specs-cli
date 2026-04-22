#!/usr/bin/env bash
# add-skill.sh — Crea una skill LOCAL nueva (no toca ai-specs.toml).
#
# Skills locales se autodescubren del filesystem por sync.sh. Esta función
# solo scaffoldea ai-specs/skills/<name>/SKILL.md desde el template y luego
# corre skill-sync para actualizar la tabla Auto-invoke en AGENTS.md.
#
# Para skills externas (vendoreadas), usar: ai-specs add-dep <url>
#
# Usage:
#   ai-specs add-skill <name> [--scope <scope>] [--description "..."] [--trigger "..."]

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
TEMPLATE="$REPO_ROOT/ai-specs/skills/skill-creator/assets/SKILL-TEMPLATE.md"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BOLD='\033[1m'
NC='\033[0m'

NAME=""
SCOPE="root"
DESCRIPTION=""
TRIGGER=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --scope)       SCOPE="$2"; shift 2 ;;
        --description) DESCRIPTION="$2"; shift 2 ;;
        --trigger)     TRIGGER="$2"; shift 2 ;;
        --help|-h)
            cat <<EOF
Usage: ai-specs add-skill <name> [OPTIONS]

Create a local skill at ai-specs/skills/<name>/SKILL.md from the template.

Options:
  --scope <s>        Scope (default: root). For monorepos: root or <subrepo-id>.
  --description "..."  Brief description (placeholder otherwise).
  --trigger "..."      Auto-invoke trigger phrase.
  --help             Show this help

Note: Local skills are NOT listed in ai-specs.toml. They are autodiscovered
from ai-specs/skills/. To add a vendored skill from a Git URL, use:
  ai-specs add-dep <url>
EOF
            exit 0 ;;
        -*)
            echo -e "${RED}Unknown option: $1${NC}"; exit 1 ;;
        *)
            if [ -z "$NAME" ]; then NAME="$1"
            else echo -e "${RED}Unexpected arg: $1${NC}"; exit 1; fi
            shift ;;
    esac
done

if [ -z "$NAME" ]; then
    echo -e "${RED}error: skill name is required${NC}"
    echo "Usage: ai-specs add-skill <name> [--scope <scope>]"
    exit 1
fi

if [ ! -f "$TEMPLATE" ]; then
    echo -e "${RED}error: template not found at $TEMPLATE${NC}"
    exit 1
fi

DEST_DIR="$REPO_ROOT/ai-specs/skills/$NAME"
DEST_FILE="$DEST_DIR/SKILL.md"

if [ -e "$DEST_DIR" ]; then
    echo -e "${RED}error: $DEST_DIR already exists${NC}"
    exit 1
fi

mkdir -p "$DEST_DIR"

# Render template with substitutions. Using env vars (not ${VAR:-default}
# inline) to avoid bash mis-parsing braces in default values.
DESCRIPTION_FB="$DESCRIPTION"
[ -z "$DESCRIPTION_FB" ] && DESCRIPTION_FB="(brief description — fill in)"
TRIGGER_FB="$TRIGGER"
[ -z "$TRIGGER_FB" ] && TRIGGER_FB="(when to load — fill in)"

export TEMPLATE_PATH="$TEMPLATE"
export DEST_PATH="$DEST_FILE"
export SKILL_NAME="$NAME"
export SKILL_SCOPE="$SCOPE"
export SKILL_DESCRIPTION="$DESCRIPTION_FB"
export SKILL_TRIGGER="$TRIGGER_FB"

python3 - <<'PY'
import os, re
from pathlib import Path

template = Path(os.environ["TEMPLATE_PATH"]).read_text()
out = template
out = out.replace("{skill-name}", os.environ["SKILL_NAME"])
out = out.replace("{Brief description of what this skill enables}", os.environ["SKILL_DESCRIPTION"])
out = out.replace("{When the AI should load this skill - be specific}", os.environ["SKILL_TRIGGER"])
out = out.replace("{Primary action phrase for AGENTS.md sync}", os.environ["SKILL_TRIGGER"])
out = re.sub(r"scope:\s*\[root\]", f"scope: [{os.environ['SKILL_SCOPE']}]", out)
Path(os.environ["DEST_PATH"]).write_text(out)
PY

echo -e "${GREEN}✓ Created $DEST_FILE${NC}"
echo ""
echo -e "${YELLOW}Next:${NC}"
echo -e "  1. Edit $DEST_FILE — fill in patterns, commands, examples."
echo -e "  2. Run: ${BOLD}ai-specs init${NC}  (regenerates AGENTS.md auto-invoke)"
echo -e "  3. Run: ${BOLD}ai-specs sync-agent --all${NC}  (re-distributes to agents)"
