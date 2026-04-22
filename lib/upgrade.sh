#!/usr/bin/env bash
# upgrade.sh — Refresh CLI payload (cli/ + skill-creator + skill-sync) in an
# existing ai-specs project, preserving manifest, local skills, and vendored deps.
#
# Usage:
#   specs-ai upgrade [path]

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
PAYLOAD="$REPO_ROOT/payload"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BOLD='\033[1m'
NC='\033[0m'

TARGET="${1:-$(pwd)}"
TARGET="$(cd "$TARGET" 2>/dev/null && pwd)" || {
    echo -e "${RED}error: target path does not exist: $TARGET${NC}" >&2
    exit 1
}

AI_SPECS_DIR="$TARGET/ai-specs"
if [ ! -d "$AI_SPECS_DIR" ]; then
    echo -e "${RED}error: $AI_SPECS_DIR does not exist${NC}" >&2
    echo "Run 'specs-ai init-project $TARGET' first." >&2
    exit 1
fi

VERSION="$(cat "$REPO_ROOT/VERSION" 2>/dev/null || echo "unknown")"

echo -e "${BOLD}specs-ai upgrade${NC}"
echo "================="
echo -e "Target:  ${BOLD}$AI_SPECS_DIR${NC}"
echo -e "Version: ${BOLD}$VERSION${NC}"
echo ""

# Refresh cli/ — overwrite verbatim from payload
echo -e "${YELLOW}[1/2]${NC} Refreshing cli/ (overwrite)"
mkdir -p "$AI_SPECS_DIR/cli/lib"
cp "$PAYLOAD/cli/ai-specs" "$AI_SPECS_DIR/cli/"
cp "$PAYLOAD/cli/init.sh" "$PAYLOAD/cli/sync-agent.sh" "$PAYLOAD/cli/add-skill.sh" "$PAYLOAD/cli/add-dep.sh" "$AI_SPECS_DIR/cli/"
cp "$PAYLOAD/cli/lib/"*.py "$PAYLOAD/cli/lib/"*.sh "$AI_SPECS_DIR/cli/lib/"
chmod +x "$AI_SPECS_DIR/cli/ai-specs" "$AI_SPECS_DIR/cli/"*.sh

# Refresh bundled skills (skill-creator + skill-sync) — these are NOT user content
echo -e "${YELLOW}[2/2]${NC} Refreshing bundled skills (skill-creator, skill-sync)"
rm -rf "$AI_SPECS_DIR/skills/skill-creator" "$AI_SPECS_DIR/skills/skill-sync"
cp -R "$PAYLOAD/skills/skill-creator" "$PAYLOAD/skills/skill-sync" "$AI_SPECS_DIR/skills/"

echo ""
echo -e "${GREEN}✓ Upgraded to $VERSION${NC}"
echo -e "  Run: ${BOLD}cd $TARGET && ./ai-specs/cli/ai-specs sync${NC} to re-distribute."
