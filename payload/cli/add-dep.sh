#!/usr/bin/env bash
# add-dep.sh — Agrega una skill VENDOREADA al ai-specs.toml.
#
# Mutador del manifiesto: appendea un bloque [[deps]] y re-corre init para
# clonar y regenerar AGENTS.md. Para skills locales usar `add-skill`.
#
# Usage:
#   ai-specs add-dep <url> [--id <id>] [--scope <scope>] [--auto-invoke "..."] [--license MIT]

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
AI_SPECS_TOML="$REPO_ROOT/ai-specs/ai-specs.toml"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BOLD='\033[1m'
NC='\033[0m'

URL=""
ID=""
SCOPE="root"
AUTO_INVOKE=""
LICENSE=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --id)          ID="$2"; shift 2 ;;
        --scope)       SCOPE="$2"; shift 2 ;;
        --auto-invoke) AUTO_INVOKE="$2"; shift 2 ;;
        --license)     LICENSE="$2"; shift 2 ;;
        --help|-h)
            cat <<EOF
Usage: ai-specs add-dep <url> [OPTIONS]

Append a [[deps]] entry to ai-specs.toml and re-run init.

Options:
  --id <id>            Skill id (default: derived from URL slug).
  --scope <s>          Scope (default: root).
  --auto-invoke "..."  Trigger phrase for AGENTS.md (root scope).
  --license <lic>      License string (default: MIT).
  --help               Show this help
EOF
            exit 0 ;;
        -*)
            echo -e "${RED}Unknown option: $1${NC}"; exit 1 ;;
        *)
            if [ -z "$URL" ]; then URL="$1"
            else echo -e "${RED}Unexpected arg: $1${NC}"; exit 1; fi
            shift ;;
    esac
done

if [ -z "$URL" ]; then
    echo -e "${RED}error: <url> is required${NC}"
    exit 1
fi

if [ ! -f "$AI_SPECS_TOML" ]; then
    echo -e "${RED}error: $AI_SPECS_TOML not found${NC}"
    exit 1
fi

# Derive id from URL if not provided: github.com/owner/repo → owner-repo
if [ -z "$ID" ]; then
    ID=$(echo "$URL" | sed -E 's#.*github\.com/##; s#/#-#g; s#\.git$##')
fi

LICENSE="${LICENSE:-MIT}"

# Check for duplicate id
if grep -qE "^id\s*=\s*\"$ID\"" "$AI_SPECS_TOML"; then
    echo -e "${RED}error: dep with id '$ID' already exists in $AI_SPECS_TOML${NC}"
    exit 1
fi

# Append [[deps]] block
{
    echo ""
    echo "[[deps]]"
    echo "id = \"$ID\""
    echo "source = \"$URL\""
    echo "scope_at_monorepo_root = \"$SCOPE\""
    if [ -n "$AUTO_INVOKE" ]; then
        echo "auto_invoke_root = [\"$AUTO_INVOKE\"]"
    else
        echo "auto_invoke_root = []"
    fi
    echo "auto_invoke_subrepo = []"
    echo "install_into_subrepos = false"
    echo "only_subrepos = []"
    echo "license = \"$LICENSE\""
    echo "vendor_attribution = \"$ID\""
} >> "$AI_SPECS_TOML"

echo -e "${GREEN}✓ Added [[deps]] entry id=\"$ID\" to $AI_SPECS_TOML${NC}"
echo ""
echo -e "${YELLOW}Re-running init to vendor the new skill...${NC}"
echo ""

bash "$SCRIPT_DIR/init.sh"

echo ""
echo -e "${BOLD}Next:${NC} ai-specs sync-agent --all  (to redistribute to agents)"
