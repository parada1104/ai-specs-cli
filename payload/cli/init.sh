#!/usr/bin/env bash
# init.sh — Bootstrap idempotente de ai-specs.
#
# Lee ai-specs/ai-specs.toml y:
#   1. Genera ai-specs/skills/vendor.manifest.toml derivado (compat con
#      vendor-skills.sh).
#   2. Si hay [[deps]], corre vendor-skills.sh para clonar/actualizar.
#   3. Corre skill-sync/sync.sh para regenerar tabla Auto-invoke en AGENTS.md.
#
# NO toca .claude/, .cursor/, etc. Para eso usar `ai-specs sync-agent`.
#
# Usage:
#   ai-specs init [--dry-run]

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
AI_SPECS_TOML="$REPO_ROOT/ai-specs/ai-specs.toml"
LIB_DIR="$SCRIPT_DIR/lib"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
RED='\033[0;31m'
BOLD='\033[1m'
NC='\033[0m'

DRY_RUN=false
for arg in "$@"; do
    [ "$arg" = "--dry-run" ] && DRY_RUN=true
done

if [ ! -f "$AI_SPECS_TOML" ]; then
    echo -e "${RED}error: $AI_SPECS_TOML not found${NC}"
    echo "Create one based on ai-specs/cli/templates/ai-specs.toml.example"
    exit 1
fi

echo -e "${BOLD}ai-specs init${NC}"
echo "=============="
echo ""

# 1. Render vendor.manifest.toml from [[deps]]
VENDOR_MANIFEST="$REPO_ROOT/ai-specs/skills/vendor.manifest.toml"
echo -e "${YELLOW}[1/3]${NC} Rendering vendor manifest"
if $DRY_RUN; then
    python3 "$LIB_DIR/deps-render.py" "$AI_SPECS_TOML" "/dev/stdout"
else
    python3 "$LIB_DIR/deps-render.py" "$AI_SPECS_TOML" "$VENDOR_MANIFEST"
fi

# 2. Vendor skills if any [[deps]] exist
DEPS_COUNT=$(python3 "$LIB_DIR/toml-read.py" "$AI_SPECS_TOML" deps | python3 -c "import json,sys; print(len(json.load(sys.stdin)))")
echo -e "${YELLOW}[2/3]${NC} Vendoring skills ($DEPS_COUNT dep(s))"
if [ "$DEPS_COUNT" -gt 0 ]; then
    VENDOR_FLAGS=()
    $DRY_RUN && VENDOR_FLAGS+=(--dry-run)
    bash "$REPO_ROOT/ai-specs/skills/skill-sync/assets/vendor-skills.sh" \
        --manifest "$VENDOR_MANIFEST" \
        --target-skills-dir "$REPO_ROOT/ai-specs/skills" \
        "${VENDOR_FLAGS[@]}"
else
    echo -e "${BLUE}  (no [[deps]] in ai-specs.toml — skipping vendor)${NC}"
fi

# 3. Regenerate AGENTS.md auto-invoke tables
echo -e "${YELLOW}[3/3]${NC} Regenerating AGENTS.md auto-invoke tables"
SYNC_FLAGS=()
$DRY_RUN && SYNC_FLAGS+=(--dry-run)
bash "$REPO_ROOT/ai-specs/skills/skill-sync/assets/sync.sh" "${SYNC_FLAGS[@]}" || {
    echo -e "${YELLOW}  (sync.sh exited non-zero — likely no skills found at expected paths; continuing)${NC}"
}

echo ""
echo -e "${GREEN}init complete.${NC}"
echo -e "Next: ${BOLD}ai-specs sync-agent --all${NC} to distribute to agents."
