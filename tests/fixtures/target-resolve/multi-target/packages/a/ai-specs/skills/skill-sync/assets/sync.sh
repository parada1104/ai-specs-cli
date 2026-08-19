#!/usr/bin/env bash
# Validate skill metadata (scope, auto_invoke) across all skill sources.
# This is a lint/check step — it does not generate any registry artifact.
# Vendoring of external skills is NOT done here — that lives in the CLI
# (ai-specs sync → lib/_internal/vendor-skills.py).
#
# Usage: ./sync.sh [--dry-run]

set -e
shopt -s nullglob

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(dirname "$(dirname "$(dirname "$(dirname "$SCRIPT_DIR")")")")"
SKILL_CONTRACT_HOME="${AI_SPECS_HOME:-$REPO_ROOT}"
SKILL_CONTRACT_PY="$SKILL_CONTRACT_HOME/lib/_internal/skill_contract.py"

if [ ! -f "$SKILL_CONTRACT_PY" ]; then
    echo -e "${RED}Missing skill contract helper: $SKILL_CONTRACT_PY${NC}" >&2
    exit 1
fi

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Options
DRY_RUN=false

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --help|-h)
            echo "Usage: $0 [--dry-run]"
            echo ""
            echo "Validate skill metadata (scope, auto_invoke) across all sources."
            echo "Reports skills with missing or invalid sync metadata."
            echo ""
            echo "Options:"
            echo "  --dry-run    Show what would be checked without side effects"
            exit 0
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            exit 1
            ;;
    esac
done

echo -e "${BLUE}Skill Sync — Validating metadata${NC}"
echo "========================================================"
echo ""

# Skip when there is no manifest (e.g. subrepo fan-out).
if [ ! -f "$REPO_ROOT/ai-specs/ai-specs.toml" ]; then
    echo "Skipping skill validation — no manifest found at $REPO_ROOT/ai-specs/ai-specs.toml"
    exit 0
fi

# Show skills without complete sync metadata
echo -e "${BLUE}Skills missing sync metadata:${NC}"
missing=0
while IFS= read -r skill_file; do
    [ -f "$skill_file" ] || continue

    if ! metadata_json=$(python3 "$SKILL_CONTRACT_PY" sync-metadata "$skill_file"); then
        echo -e "  ${RED}invalid${NC} - $skill_file"
        missing=$((missing + 1))
        continue
    fi

    skill_name=$(python3 -c 'import json,sys; print(json.loads(sys.argv[1]).get("name",""))' "$metadata_json")
    scope_raw=$(python3 -c 'import json,sys; print("|".join(json.loads(sys.argv[1]).get("scope",[])))' "$metadata_json")
    auto_invoke_raw=$(python3 -c 'import json,sys; print("|".join(json.loads(sys.argv[1]).get("auto_invoke",[])))' "$metadata_json")

    if [ -z "$scope_raw" ] && [ -z "$auto_invoke_raw" ]; then
        echo -e "  ${YELLOW}$skill_name${NC} - missing scope and auto_invoke"
        missing=$((missing + 1))
    elif [ -z "$scope_raw" ]; then
        echo -e "  ${YELLOW}$skill_name${NC} - missing scope"
        missing=$((missing + 1))
    elif [ -z "$auto_invoke_raw" ]; then
        echo -e "  ${YELLOW}$skill_name${NC} - missing auto_invoke"
        missing=$((missing + 1))
    fi
done < <(find "$REPO_ROOT" \
    \( -name node_modules -o -name .git -o -name .worktrees -o -name .opencode -o -name .claude -o -name .cursor -o -name .gemini -o -name .codex \) -prune -o \
    -type f -path "*/skills/*/SKILL.md" -print 2>/dev/null | LC_ALL=C sort)

if [ $missing -eq 0 ]; then
    echo -e "  ${GREEN}All skills have sync metadata${NC}"
fi

echo ""
echo -e "${GREEN}Done!${NC}"