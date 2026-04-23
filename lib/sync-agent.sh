#!/usr/bin/env bash
# sync-agent.sh — fan out skills + MCP + slash commands to per-agent locations.
#
# For every selected agent:
#   - skills_dir         (Claude/Gemini): symlink → <path>/ai-specs/skills
#   - instructions_path  (Claude/Gemini/Copilot): symlink AGENTS.md → CLAUDE.md / etc.
#   - mcp_config_path    (Claude/Cursor/OpenCode/Codex/Gemini): merged write via mcp-render.py
#   - commands_dir       (Claude/Cursor/OpenCode): copy ai-specs/commands/*.md
#
# Native agents (Cursor/OpenCode/Codex/Copilot) read AGENTS.md directly — no
# instructions symlink, no skills_dir.
#
# Usage:
#   ai-specs sync-agent [path] (--all | --claude | --cursor | --opencode | --codex | --copilot | --gemini)...

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_SPECS_HOME="$(cd "$SCRIPT_DIR/.." && pwd)"

# shellcheck source=lib/_internal/platform.sh
source "$AI_SPECS_HOME/lib/_internal/platform.sh"

TOML_READ="$AI_SPECS_HOME/lib/_internal/toml-read.py"
MCP_RENDER="$AI_SPECS_HOME/lib/_internal/mcp-render.py"

ALL_AGENTS=(claude cursor opencode codex copilot gemini)

usage() {
    cat <<'EOF'
Usage: ai-specs sync-agent [path] [--all | --<agent>...]

Render per-agent configs (skills + MCP + instructions) from ai-specs.toml.

Arguments:
  path             Project root (default: current directory)

Selectors (pick one or many):
  --all            All agents listed under [agents].enabled in ai-specs.toml
  --claude         Claude Code  (CLAUDE.md, .claude/skills, .mcp.json)
  --cursor         Cursor       (.cursor/mcp.json)
  --opencode       OpenCode     (opencode.json)
  --codex          Codex        (.codex/config.toml)
  --copilot        GitHub Copilot (.github/copilot-instructions.md)
  --gemini         Gemini CLI   (GEMINI.md, .gemini/skills, .gemini/settings.json)

If no selector is given, defaults to --all.
EOF
}

TARGET_PATH=""
SELECT_ALL=0
declare -a SELECTED_AGENTS=()

while [[ $# -gt 0 ]]; do
    case "$1" in
        --all)        SELECT_ALL=1; shift ;;
        --claude|--cursor|--opencode|--codex|--copilot|--gemini)
            SELECTED_AGENTS+=("${1#--}"); shift ;;
        -h|--help)    usage; exit 0 ;;
        --)           shift; break ;;
        -*)
            echo "ERROR: unknown flag: $1" >&2
            echo "Run 'ai-specs sync-agent --help' for usage." >&2
            exit 2
            ;;
        *)
            if [[ -z "$TARGET_PATH" ]]; then
                TARGET_PATH="$1"
            else
                echo "ERROR: unexpected positional argument: $1" >&2
                exit 2
            fi
            shift
            ;;
    esac
done

[[ -z "$TARGET_PATH" ]] && TARGET_PATH="$(pwd)"
TARGET_PATH="$(cd "$TARGET_PATH" && pwd)"

TOML_PATH="$TARGET_PATH/ai-specs/ai-specs.toml"
AI_SKILLS="$TARGET_PATH/ai-specs/skills"
AI_COMMANDS="$TARGET_PATH/ai-specs/commands"
AGENTS_MD="$TARGET_PATH/AGENTS.md"

if [[ ! -f "$TOML_PATH" ]]; then
    echo "ERROR: $TOML_PATH not found. Run 'ai-specs init $TARGET_PATH' first." >&2
    exit 1
fi
if [[ ! -f "$AGENTS_MD" ]]; then
    echo "ERROR: $AGENTS_MD not found. Run 'ai-specs init $TARGET_PATH' first." >&2
    exit 1
fi

# Resolve enabled agents from ai-specs.toml
ENABLED_JSON="$(python3 "$TOML_READ" "$TOML_PATH" agents)"
mapfile -t ENABLED_AGENTS < <(python3 -c "import json,sys; [print(a) for a in json.loads(sys.argv[1])]" "$ENABLED_JSON")

# Pick targets
declare -a TARGETS=()
if [[ $SELECT_ALL -eq 1 || ${#SELECTED_AGENTS[@]} -eq 0 ]]; then
    TARGETS=("${ENABLED_AGENTS[@]}")
else
    TARGETS=("${SELECTED_AGENTS[@]}")
fi

if [[ ${#TARGETS[@]} -eq 0 ]]; then
    echo "WARNING: no agents to sync. Set [agents].enabled in ai-specs.toml." >&2
    exit 0
fi

# Detect MCP presence (so we can skip mcp-render for empty cases without noise)
MCP_COUNT="$(python3 - "$TOML_PATH" <<'PY'
import sys, tomllib
with open(sys.argv[1], "rb") as f:
    print(len(tomllib.load(f).get("mcp", {}) or {}))
PY
)"

# Helpers
make_relative_symlink() {
    # ln -s <target_relative_to_link_dir> <link_path>
    local target_abs="$1"
    local link_path="$2"
    local link_dir
    link_dir="$(dirname "$link_path")"
    mkdir -p "$link_dir"

    local rel
    rel="$(python3 -c "import os,sys; print(os.path.relpath(sys.argv[1], sys.argv[2]))" \
            "$target_abs" "$link_dir")"

    if [[ -L "$link_path" ]]; then
        local existing
        existing="$(readlink "$link_path")"
        if [[ "$existing" == "$rel" ]]; then
            echo "    · symlink ok      $link_path → $rel"
            return 0
        fi
        rm "$link_path"
    elif [[ -e "$link_path" ]]; then
        echo "    ✗ refuse to overwrite non-symlink: $link_path" >&2
        return 1
    fi
    ln -s "$rel" "$link_path"
    echo "    ✓ symlink created $link_path → $rel"
}

echo ""
echo "ai-specs sync-agent"
echo "  target:  $TARGET_PATH"
echo "  agents:  ${TARGETS[*]}"
echo "  enabled: ${ENABLED_AGENTS[*]:-(none)}"
echo "  mcp:     $MCP_COUNT server(s)"
echo ""

for agent in "${TARGETS[@]}"; do
    # Validate agent
    if ! platform_get "$agent" native >/dev/null 2>&1; then
        echo "  ✗ unknown agent: $agent" >&2
        continue
    fi

    # Warn if not in enabled list (when explicitly selected)
    is_enabled=0
    for e in "${ENABLED_AGENTS[@]}"; do
        [[ "$e" == "$agent" ]] && is_enabled=1 && break
    done
    if [[ $is_enabled -eq 0 ]]; then
        echo "  ! $agent not in [agents].enabled — syncing anyway"
    fi

    echo "  ▸ $agent"

    # 1. instructions_path → symlink AGENTS.md → CLAUDE.md / GEMINI.md / etc.
    instr="$(platform_get "$agent" instructions_path)"
    if [[ -n "$instr" ]]; then
        make_relative_symlink "$AGENTS_MD" "$TARGET_PATH/$instr"
    fi

    # 2. skills_dir → symlink → ai-specs/skills
    skills="$(platform_get "$agent" skills_dir)"
    if [[ -n "$skills" ]]; then
        make_relative_symlink "$AI_SKILLS" "$TARGET_PATH/$skills"
    fi

    # 3. MCP rendering (skip if no servers or no native MCP support)
    mcp_path="$(platform_get "$agent" mcp_config_path)"
    mcp_key="$(platform_get "$agent" mcp_key)"
    if [[ -n "$mcp_path" && -n "$mcp_key" ]]; then
        if [[ "$MCP_COUNT" -gt 0 ]]; then
            python3 "$MCP_RENDER" "$TOML_PATH" "$agent" \
                "$TARGET_PATH/$mcp_path" "$mcp_key"
        else
            echo "    · mcp skipped (no [mcp.*] in manifest)"
        fi
    fi

    # 4. Slash commands — copy ai-specs/commands/*.md into the agent's
    # commands_dir (idempotent, overwrite). Source of truth is the project,
    # not the CLI. Skip agents without slash-command UX.
    cmd_dir="$(platform_get "$agent" commands_dir)"
    if [[ -n "$cmd_dir" && -d "$AI_COMMANDS" ]]; then
        dest="$TARGET_PATH/$cmd_dir"
        mkdir -p "$dest"
        copied=0
        for src in "$AI_COMMANDS"/*.md; do
            [[ -f "$src" ]] || continue
            cp "$src" "$dest/$(basename "$src")"
            copied=$((copied + 1))
        done
        if [[ $copied -gt 0 ]]; then
            echo "    ✓ commands     $cmd_dir/ ($copied file(s))"
        fi
    fi
done

echo ""
echo "✓ sync-agent complete"
