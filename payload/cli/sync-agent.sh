#!/usr/bin/env bash
# sync-agent.sh — Distribuye skills + MCP a cada agente habilitado.
#
# Por cada agente:
#   - Symlink ai-specs/skills/ → <agent>/skills/
#   - Si fallback (Claude/Gemini): symlink AGENTS.md → CLAUDE.md/GEMINI.md
#   - Si Copilot: copia AGENTS.md → .github/copilot-instructions.md
#   - Render MCP servers al formato del agente (merge-safe).
#
# Usage:
#   ai-specs sync-agent --all
#   ai-specs sync-agent --claude --cursor
#   ai-specs sync-agent --claude --dry-run

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
AI_SPECS_TOML="$REPO_ROOT/ai-specs/ai-specs.toml"
LIB_DIR="$SCRIPT_DIR/lib"

# shellcheck source=lib/platform.sh
source "$LIB_DIR/platform.sh"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
RED='\033[0;31m'
BOLD='\033[1m'
NC='\033[0m'

DRY_RUN=false
AGENTS_REQUESTED=()

if [ ! -f "$AI_SPECS_TOML" ]; then
    echo -e "${RED}error: $AI_SPECS_TOML not found. Run \`ai-specs init\` first.${NC}"
    exit 1
fi

while [[ $# -gt 0 ]]; do
    case "$1" in
        --all)
            AGENTS_REQUESTED=(claude cursor opencode codex copilot gemini) ;;
        --claude)   AGENTS_REQUESTED+=(claude) ;;
        --cursor)   AGENTS_REQUESTED+=(cursor) ;;
        --opencode) AGENTS_REQUESTED+=(opencode) ;;
        --codex)    AGENTS_REQUESTED+=(codex) ;;
        --copilot)  AGENTS_REQUESTED+=(copilot) ;;
        --gemini)   AGENTS_REQUESTED+=(gemini) ;;
        --dry-run)  DRY_RUN=true ;;
        --help|-h)
            cat <<EOF
Usage: ai-specs sync-agent [OPTIONS]

Distribute skills and MCP configs to agents enabled in ai-specs.toml.

Options:
  --all          Sync to every agent in [agents].enabled
  --claude       Sync to Claude (.claude/skills/, .mcp.json, CLAUDE.md)
  --cursor       Sync to Cursor (.cursor/skills/, .cursor/mcp.json)
  --opencode     Sync to OpenCode (.opencode/skills/, opencode.json)
  --codex        Sync to Codex (.codex/skills/, .codex/config.toml)
  --copilot      Sync to Copilot (.github/copilot-instructions.md)
  --gemini       Sync to Gemini (.gemini/skills/, .gemini/settings.json, GEMINI.md)
  --dry-run      Show what would change without writing files
  --help         Show this help
EOF
            exit 0 ;;
        *) echo -e "${RED}Unknown option: $1${NC}"; exit 1 ;;
    esac
    shift
done

# Filter against [agents].enabled in ai-specs.toml
ENABLED_JSON=$(python3 "$LIB_DIR/toml-read.py" "$AI_SPECS_TOML" agents)
mapfile -t ENABLED < <(echo "$ENABLED_JSON" | python3 -c "import json,sys; [print(a) for a in json.load(sys.stdin)]")

is_enabled() {
    local target="$1"
    for a in "${ENABLED[@]}"; do
        [ "$a" = "$target" ] && return 0
    done
    return 1
}

if [ ${#AGENTS_REQUESTED[@]} -eq 0 ]; then
    echo -e "${YELLOW}No agents specified. Use --all or --claude/--cursor/...${NC}"
    exit 1
fi

echo -e "${BOLD}ai-specs sync-agent${NC}"
echo "===================="
echo ""

SKILLS_SOURCE="$REPO_ROOT/ai-specs/skills"
SKILL_COUNT=$(find "$SKILLS_SOURCE" -maxdepth 2 -name "SKILL.md" 2>/dev/null | wc -l | tr -d ' ')
echo -e "${BLUE}Found $SKILL_COUNT skills in $SKILLS_SOURCE${NC}"
echo ""

# Check MCP availability
MCP_JSON=$(python3 "$LIB_DIR/toml-read.py" "$AI_SPECS_TOML" mcp)
MCP_COUNT=$(echo "$MCP_JSON" | python3 -c "import json,sys; print(len(json.load(sys.stdin)))")
echo -e "${BLUE}Found $MCP_COUNT MCP server(s)${NC}"
echo ""

link_skills() {
    local agent="$1"
    local rel_dir="$2"
    [ -z "$rel_dir" ] && return 0

    local target="$REPO_ROOT/$rel_dir"
    local parent
    parent=$(dirname "$target")

    if $DRY_RUN; then
        echo -e "${CYAN}    [dry-run] would symlink $rel_dir → ai-specs/skills${NC}"
        return 0
    fi

    mkdir -p "$parent"
    if [ -L "$target" ]; then
        rm "$target"
    elif [ -d "$target" ]; then
        mv "$target" "${target}.backup.$(date +%s)"
    fi
    ln -s "$SKILLS_SOURCE" "$target"
    echo -e "${GREEN}    ✓ $rel_dir → ai-specs/skills${NC}"
}

link_or_copy_instructions() {
    local agent="$1"
    local target_rel="$2"
    [ -z "$target_rel" ] && return 0

    local source="$REPO_ROOT/AGENTS.md"
    [ ! -f "$source" ] && {
        echo -e "${YELLOW}    skipped: AGENTS.md not found at root${NC}"
        return 0
    }

    local target="$REPO_ROOT/$target_rel"

    if $DRY_RUN; then
        if [ "$agent" = "copilot" ]; then
            echo -e "${CYAN}    [dry-run] would copy AGENTS.md → $target_rel${NC}"
        else
            echo -e "${CYAN}    [dry-run] would symlink $target_rel → AGENTS.md${NC}"
        fi
        return 0
    fi

    mkdir -p "$(dirname "$target")"

    # Copilot reads .github/copilot-instructions.md as a literal copy.
    # Fallback agents (Claude/Gemini) get a symlink.
    if [ "$agent" = "copilot" ]; then
        cp "$source" "$target"
        echo -e "${GREEN}    ✓ AGENTS.md → $target_rel (copy)${NC}"
    else
        [ -L "$target" ] && rm "$target"
        [ -f "$target" ] && rm "$target"
        # Compute relative path from target's parent to source
        local target_dir
        target_dir=$(dirname "$target")
        local rel
        rel=$(python3 -c "import os; print(os.path.relpath('$source', '$target_dir'))")
        ln -s "$rel" "$target"
        echo -e "${GREEN}    ✓ $target_rel → AGENTS.md (symlink)${NC}"
    fi
}

render_mcp() {
    local agent="$1"
    local target_rel
    local mcp_key
    target_rel=$(platform_get "$agent" mcp_config_path)
    mcp_key=$(platform_get "$agent" mcp_key)

    if [ -z "$target_rel" ] || [ -z "$mcp_key" ]; then
        echo -e "${BLUE}    no MCP support for $agent — skipped${NC}"
        return 0
    fi

    if [ "$MCP_COUNT" -eq 0 ]; then
        echo -e "${BLUE}    no [mcp.*] in TOML — skipped${NC}"
        return 0
    fi

    local target="$REPO_ROOT/$target_rel"
    local flags=()
    $DRY_RUN && flags+=(--dry-run)
    python3 "$LIB_DIR/mcp-render.py" "$AI_SPECS_TOML" "$agent" "$target" "$mcp_key" "${flags[@]}"
}

sync_one() {
    local agent="$1"

    if ! is_enabled "$agent"; then
        echo -e "${YELLOW}[$agent] not in [agents].enabled — skipped${NC}"
        echo ""
        return 0
    fi

    echo -e "${BOLD}[$agent]${NC}"
    link_skills "$agent" "$(platform_get "$agent" skills_dir)"
    link_or_copy_instructions "$agent" "$(platform_get "$agent" instructions_path)"
    render_mcp "$agent"
    echo ""
}

# Dedupe AGENTS_REQUESTED
declare -A SEEN
UNIQUE=()
for a in "${AGENTS_REQUESTED[@]}"; do
    [ -n "${SEEN[$a]}" ] && continue
    SEEN[$a]=1
    UNIQUE+=("$a")
done

for agent in "${UNIQUE[@]}"; do
    sync_one "$agent"
done

echo -e "${GREEN}sync-agent complete.${NC}"
