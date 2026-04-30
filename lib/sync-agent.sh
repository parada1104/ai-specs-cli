#!/usr/bin/env bash
# sync-agent.sh — fan out skills + MCP + slash commands to per-agent locations.
#
# Usage:
#   ai-specs sync-agent [path] [--all | --<agent>...]
#   ai-specs sync-agent --source-root <root> --target <path> [--all | --<agent>...]
#
# In multi-target mode the root manifest remains the source of truth, while the
# target receives a fully local derived artifact set:
#   - AGENTS.md
#   - ai-specs/.gitignore
#   - ai-specs/skills/**
#   - ai-specs/commands/**
#   - per-agent configs/symlinks (CLAUDE.md, .cursor/mcp.json, opencode.json, ...)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_SPECS_HOME="$(cd "$SCRIPT_DIR/.." && pwd)"

# shellcheck source=lib/_internal/platform.sh
source "$AI_SPECS_HOME/lib/_internal/platform.sh"

TOML_READ="$AI_SPECS_HOME/lib/_internal/toml-read.py"
MCP_RENDER="$AI_SPECS_HOME/lib/_internal/mcp-render.py"
AGENTS_MD_RENDER="$AI_SPECS_HOME/lib/_internal/agents-md-render.py"
GITIGNORE_RENDER="$AI_SPECS_HOME/lib/_internal/gitignore-render.py"
TARGET_RESOLVE_PY="$AI_SPECS_HOME/lib/_internal/target-resolve.py"
FLATTEN_SKILLS_PY="$AI_SPECS_HOME/lib/_internal/flatten-resolved-skills.py"

usage() {
    cat <<'EOF'
Usage: ai-specs sync-agent [path] [--all | --<agent>...]
       ai-specs sync-agent --source-root <root> --target <path> [--all | --<agent>...]

Render per-agent configs from the root manifest.

Arguments:
  path             Target path when using the legacy single-target form

Flags:
  --source-root    Root project that owns ai-specs/ai-specs.toml (default: target)
  --target         Target directory receiving derived local artifacts
  --all            All agents listed under [agents].enabled in ai-specs.toml
  --claude         Claude Code  (CLAUDE.md, .claude/skills, .mcp.json)
  --cursor         Cursor       (.cursor/mcp.json)
  --opencode       OpenCode     (opencode.json, .opencode/skills, .opencode/commands)
  --codex          Codex        (.codex/config.toml)
  --copilot        GitHub Copilot (.github/copilot-instructions.md)
  --gemini         Gemini CLI   (GEMINI.md, .gemini/skills, .gemini/settings.json)

If no selector is given, defaults to --all.
EOF
}

TARGET_PATH=""
SOURCE_ROOT=""
SELECT_ALL=0
EXPLICIT_SOURCE_ROOT=0
EXPLICIT_TARGET=0
declare -a SELECTED_AGENTS=()

while [[ $# -gt 0 ]]; do
    case "$1" in
        --source-root) SOURCE_ROOT="${2:-}"; EXPLICIT_SOURCE_ROOT=1; shift 2 ;;
        --target)      TARGET_PATH="${2:-}"; EXPLICIT_TARGET=1; shift 2 ;;
        --all)         SELECT_ALL=1; shift ;;
        --claude|--cursor|--opencode|--codex|--copilot|--gemini)
            SELECTED_AGENTS+=("${1#--}"); shift ;;
        -h|--help)     usage; exit 0 ;;
        --)            shift; break ;;
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
[[ -z "$SOURCE_ROOT" ]] && SOURCE_ROOT="$TARGET_PATH"
SOURCE_ROOT="$(cd "$SOURCE_ROOT" && pwd)"

if [[ $EXPLICIT_SOURCE_ROOT -eq 0 && $EXPLICIT_TARGET -eq 0 ]]; then
    PLAN_JSON="$(python3 "$TARGET_RESOLVE_PY" "$TARGET_PATH")" || {
        echo "ERROR: target resolution failed before any writes." >&2
        exit 1
    }

    ROOT_PATH="$(python3 -c 'import json,sys; print(json.loads(sys.stdin.read())["root"])' <<<"$PLAN_JSON")"
    RESOLVED_TARGETS=()
    while IFS= read -r target; do
        [[ -n "$target" ]] && RESOLVED_TARGETS+=("$target")
    done < <(python3 -c 'import json,sys; [print(t["path"]) for t in json.loads(sys.stdin.read())["targets"]]' <<<"$PLAN_JSON")

    if [[ ${#RESOLVED_TARGETS[@]} -gt 1 ]]; then
        echo ""
        echo "ai-specs sync-agent"
        echo "  source root: $ROOT_PATH"
        echo "  targets:     ${RESOLVED_TARGETS[*]}"
        echo "  mode:        public root fan-out"
        echo ""

        FORWARD_ARGS=()
        if [[ $SELECT_ALL -eq 1 ]]; then
            FORWARD_ARGS+=("--all")
        elif [[ ${#SELECTED_AGENTS[@]} -gt 0 ]]; then
            for agent in "${SELECTED_AGENTS[@]}"; do
                FORWARD_ARGS+=("--$agent")
            done
        fi

        for resolved_target in "${RESOLVED_TARGETS[@]}"; do
            if ! bash "$0" --source-root "$ROOT_PATH" --target "$resolved_target" "${FORWARD_ARGS[@]}"; then
                echo "ERROR: sync-agent failed for target: $resolved_target" >&2
                echo "       Stopped on first failure; no overall success reported." >&2
                exit 1
            fi
        done
        exit 0
    fi
fi

TOML_PATH="$SOURCE_ROOT/ai-specs/ai-specs.toml"
SOURCE_AI_SPECS="$SOURCE_ROOT/ai-specs"
SOURCE_AI_SKILLS="$SOURCE_AI_SPECS/skills"
SOURCE_AI_COMMANDS="$SOURCE_AI_SPECS/commands"
TARGET_AI_SPECS="$TARGET_PATH/ai-specs"
TARGET_AI_SKILLS="$TARGET_AI_SPECS/skills"
TARGET_AI_COMMANDS="$TARGET_AI_SPECS/commands"
TARGET_AGENTS_MD="$TARGET_PATH/AGENTS.md"

# Flatten resolved skills (multi-source) into a persistent dir for agent fan-out
RESOLVED_SKILLS_DIR="$SOURCE_AI_SPECS/.resolved-skills"
python3 "$FLATTEN_SKILLS_PY" "$SOURCE_ROOT" "$RESOLVED_SKILLS_DIR"

if [[ ! -f "$TOML_PATH" ]]; then
    echo "ERROR: $TOML_PATH not found. Run 'ai-specs init $SOURCE_ROOT' first." >&2
    exit 1
fi

mirror_directory() {
    local src="$1"
    local dest="$2"
    rm -rf "$dest"
    mkdir -p "$(dirname "$dest")"
    if [[ -d "$src" ]]; then
        cp -R "$src" "$dest"
    else
        mkdir -p "$dest"
    fi
}

make_relative_symlink() {
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

ensure_target_workspace() {
    if [[ "$TARGET_PATH" == "$SOURCE_ROOT" ]]; then
        [[ -f "$TARGET_AGENTS_MD" ]] || {
            echo "ERROR: $TARGET_AGENTS_MD not found. Run 'ai-specs init $TARGET_PATH' first." >&2
            exit 1
        }
        return 0
    fi

    mkdir -p "$TARGET_AI_SPECS"
    python3 "$GITIGNORE_RENDER" "$TOML_PATH" "$TARGET_AI_SPECS/.gitignore"
    mirror_directory "$RESOLVED_SKILLS_DIR" "$TARGET_AI_SKILLS"
    mirror_directory "$SOURCE_AI_COMMANDS" "$TARGET_AI_COMMANDS"

    python3 "$AGENTS_MD_RENDER" "$SOURCE_ROOT" "$TARGET_AGENTS_MD" --skills-dir "$TARGET_AI_SKILLS"

    local target_skill_sync="$TARGET_AI_SKILLS/skill-sync/assets/sync.sh"
    if [[ -f "$target_skill_sync" ]]; then
        (cd "$TARGET_PATH" && bash "$target_skill_sync")
    fi
}

# Resolve enabled agents from ai-specs.toml
ENABLED_JSON="$(python3 "$TOML_READ" "$TOML_PATH" agents)"
ENABLED_AGENTS=()
while IFS= read -r agent; do
    [[ -n "$agent" ]] && ENABLED_AGENTS+=("$agent")
done < <(python3 -c "import json,sys; [print(a) for a in json.loads(sys.argv[1]).get('enabled', [])]" "$ENABLED_JSON")

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

MCP_COUNT="$(python3 - "$TOML_PATH" <<'PY'
import sys, tomllib
with open(sys.argv[1], "rb") as f:
    print(len(tomllib.load(f).get("mcp", {}) or {}))
PY
)"

echo ""
echo "ai-specs sync-agent"
echo "  source root: $SOURCE_ROOT"
echo "  target:      $TARGET_PATH"
echo "  agents:      ${TARGETS[*]}"
echo "  enabled:     ${ENABLED_AGENTS[*]:-(none)}"
echo "  mcp:         $MCP_COUNT server(s)"
echo ""

echo "  derived artifacts: AGENTS.md, ai-specs/.gitignore, ai-specs/skills/**, ai-specs/commands/**, agent-configs"
ensure_target_workspace

# For root workspace, agents consume from resolved dir to avoid polluting ai-specs/skills/
if [[ "$TARGET_PATH" == "$SOURCE_ROOT" ]]; then
    SKILLS_SOURCE="$RESOLVED_SKILLS_DIR"
else
    SKILLS_SOURCE="$TARGET_AI_SKILLS"
fi

for agent in "${TARGETS[@]}"; do
    if ! platform_get "$agent" native >/dev/null 2>&1; then
        echo "  ✗ unknown agent: $agent" >&2
        continue
    fi

    is_enabled=0
    for e in "${ENABLED_AGENTS[@]}"; do
        [[ "$e" == "$agent" ]] && is_enabled=1 && break
    done
    if [[ $is_enabled -eq 0 ]]; then
        echo "  ! $agent not in [agents].enabled — syncing anyway"
    fi

    echo "  ▸ $agent"

    instr="$(platform_get "$agent" instructions_path)"
    if [[ -n "$instr" ]]; then
        make_relative_symlink "$TARGET_AGENTS_MD" "$TARGET_PATH/$instr"
    fi

    skills="$(platform_get "$agent" skills_dir)"
    if [[ -n "$skills" ]]; then
        if [[ "$agent" == "opencode" ]]; then
            dest="$TARGET_PATH/$skills"
            mkdir -p "$dest"
            copied=0
            for src in "$SKILLS_SOURCE"/*; do
                [[ -d "$src" ]] || continue
                base="$(basename "$src")"
                rm -rf "$dest/$base"
                cp -R "$src" "$dest/$base"
                copied=$((copied + 1))
            done
            if [[ $copied -gt 0 ]]; then
                echo "    ✓ skills       $skills/ ($copied dir(s))"
            fi
        else
            make_relative_symlink "$SKILLS_SOURCE" "$TARGET_PATH/$skills"
        fi
    fi

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

    cmd_dir="$(platform_get "$agent" commands_dir)"
    if [[ -n "$cmd_dir" && -d "$TARGET_AI_COMMANDS" ]]; then
        dest="$TARGET_PATH/$cmd_dir"
        mkdir -p "$dest"
        copied=0
        for src in "$TARGET_AI_COMMANDS"/*.md; do
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
