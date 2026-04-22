#!/usr/bin/env bash
# sync.sh — full reconciliation of a project's ai-specs/ from its manifest.
#
# Pipeline:
#   1. Render  <path>/ai-specs/.gitignore                   from [[deps]]
#   2. Refresh bundled skills/commands + lock file          via _internal/refresh-bundled.py
#   3. Vendor  external skills                              via _internal/vendor-skills.py
#   4. Render  <path>/AGENTS.md (skills index)              via agents-md-render.py
#   5. Refresh <path>/AGENTS.md auto-invoke table           via skill-sync/assets/sync.sh
#   6. Fan-out per-agent configs                            via sync-agent --all
#
# Usage:
#   specs-ai sync [path]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SPECS_AI_HOME="$(cd "$SCRIPT_DIR/.." && pwd)"

usage() {
    cat <<'EOF'
Usage: specs-ai sync [path]

Reconcile a project's ai-specs/ with its manifest:
  - vendor [[deps]] from ai-specs.toml
  - regenerate AGENTS.md auto-invoke table
  - fan out skills + MCP per agent

Arguments:
  path      Project root (default: current directory)
EOF
}

TARGET_PATH=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        -h|--help) usage; exit 0 ;;
        --)        shift; break ;;
        -*)
            echo "ERROR: unknown flag: $1" >&2
            echo "Run 'specs-ai sync --help' for usage." >&2
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

AI_SPECS_DIR="$TARGET_PATH/ai-specs"
TOML_PATH="$AI_SPECS_DIR/ai-specs.toml"
AI_GITIGNORE="$AI_SPECS_DIR/.gitignore"

SKILL_SYNC_DIR="$AI_SPECS_DIR/skills/skill-sync/assets"
SYNC_SH="$SKILL_SYNC_DIR/sync.sh"

VENDOR_SKILLS_PY="$SPECS_AI_HOME/lib/_internal/vendor-skills.py"
GITIGNORE_RENDER="$SPECS_AI_HOME/lib/_internal/gitignore-render.py"
AGENTS_MD_RENDER="$SPECS_AI_HOME/lib/_internal/agents-md-render.py"
REFRESH_BUNDLED_PY="$SPECS_AI_HOME/lib/_internal/refresh-bundled.py"
SYNC_AGENT_SH="$SPECS_AI_HOME/lib/sync-agent.sh"

if [[ ! -f "$TOML_PATH" ]]; then
    echo "ERROR: $TOML_PATH not found." >&2
    echo "       Run 'specs-ai init $TARGET_PATH' first." >&2
    exit 1
fi
if [[ ! -d "$SKILL_SYNC_DIR" ]]; then
    echo "ERROR: $SKILL_SYNC_DIR not found." >&2
    echo "       Run 'specs-ai init --force $TARGET_PATH' to restore bundled skills." >&2
    exit 1
fi

echo ""
echo "specs-ai sync"
echo "  target: $TARGET_PATH"
echo ""

# 1. Render ai-specs/.gitignore from [[deps]]
echo "▸ gitignore-render"
python3 "$GITIGNORE_RENDER" "$TOML_PATH" "$AI_GITIGNORE"

# 2. Refresh bundled skills & commands (auto-update clean files, drop .new for
#    customized ones). Runs before vendor so any upstream skill-sync upgrade is
#    active when we regen the AGENTS.md auto-invoke table below.
echo "▸ refresh-bundled"
python3 "$REFRESH_BUNDLED_PY" "$TARGET_PATH" "$SPECS_AI_HOME"

# 3. Vendor external skills
echo "▸ vendor-skills"
python3 "$VENDOR_SKILLS_PY" "$TARGET_PATH"

# 4. Render AGENTS.md from skills index
echo "▸ agents-md-render"
python3 "$AGENTS_MD_RENDER" "$TARGET_PATH" "$TARGET_PATH/AGENTS.md"

# 5. Refresh AGENTS.md auto-invoke table on top of the freshly-rendered file
echo "▸ skill-sync (AGENTS.md auto-invoke)"
bash "$SYNC_SH"

# 6. Fan-out per-agent configs
echo "▸ sync-agent --all"
bash "$SYNC_AGENT_SH" "$TARGET_PATH" --all

echo ""
echo "✓ specs-ai sync complete"
