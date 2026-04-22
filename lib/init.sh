#!/usr/bin/env bash
# init.sh — bootstrap the ai-specs/ standard in a project.
#
# Usage:
#   specs-ai init [path] [--name <project-name>] [--force]
#
# Flags:
#   --name <name>   Project name baked into ai-specs.toml (default: basename of path)
#   --force         Overwrite ai-specs.toml, AGENTS.md, bundled skills, and the
#                   agent-block in <path>/.gitignore. Default behavior preserves
#                   user-edited files (idempotent).
#
# Always (re)generated regardless of --force:
#   <path>/ai-specs/.gitignore   (derived from [[deps]] in ai-specs.toml)
#
# Layout produced:
#   <path>/
#   ├── AGENTS.md                       (always regenerated from ai-specs/* by `sync`)
#   ├── .gitignore                      (agent-block appended; idempotent via marker)
#   └── ai-specs/
#       ├── ai-specs.toml               (template if missing)
#       ├── .gitignore                  (always rendered from ai-specs.toml)
#       ├── agents.d/                   (user content fragments for AGENTS.md)
#       │   └── README.md
#       └── skills/
#           ├── skill-creator/          (bundled — committable)
#           └── skill-sync/             (bundled — committable)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SPECS_AI_HOME="$(cd "$SCRIPT_DIR/.." && pwd)"

usage() {
    cat <<'EOF'
Usage: specs-ai init [path] [--name <project-name>] [--force]

Bootstrap the ai-specs standard in a project (idempotent by default).

Arguments:
  path              Project root (default: current directory)

Flags:
  --name <name>     Project name in ai-specs.toml (default: basename of path)
  --force           Re-render templates and re-copy bundled skills even if present
  -h, --help        Show this help

Examples:
  specs-ai init                        # initialize cwd
  specs-ai init ~/code/my-app          # initialize specific path
  specs-ai init --name my-app          # override project name
  specs-ai init --force                # re-render templates (destructive)
EOF
}

# Defaults
TARGET_PATH=""
PROJECT_NAME=""
FORCE=0

while [[ $# -gt 0 ]]; do
    case "$1" in
        --name)        PROJECT_NAME="${2:-}"; shift 2 ;;
        --name=*)      PROJECT_NAME="${1#*=}"; shift ;;
        --force)       FORCE=1; shift ;;
        -h|--help)     usage; exit 0 ;;
        --)            shift; break ;;
        -*)
            echo "ERROR: unknown flag: $1" >&2
            echo "Run 'specs-ai init --help' for usage." >&2
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

# Resolve target path
[[ -z "$TARGET_PATH" ]] && TARGET_PATH="$(pwd)"
if [[ ! -d "$TARGET_PATH" ]]; then
    echo "ERROR: target path does not exist or is not a directory: $TARGET_PATH" >&2
    exit 1
fi
TARGET_PATH="$(cd "$TARGET_PATH" && pwd)"

# Derive project name from basename if not provided
[[ -z "$PROJECT_NAME" ]] && PROJECT_NAME="$(basename "$TARGET_PATH")"

# Paths
AI_SPECS_DIR="$TARGET_PATH/ai-specs"
SKILLS_DIR="$AI_SPECS_DIR/skills"
AGENTS_D_DIR="$AI_SPECS_DIR/agents.d"
TOML_PATH="$AI_SPECS_DIR/ai-specs.toml"
AGENTS_PATH="$TARGET_PATH/AGENTS.md"
ROOT_GITIGNORE="$TARGET_PATH/.gitignore"
AI_GITIGNORE="$AI_SPECS_DIR/.gitignore"

BUNDLED_SKILLS_DIR="$SPECS_AI_HOME/bundled-skills"
TEMPLATES_DIR="$SPECS_AI_HOME/templates"
GITIGNORE_RENDER="$SPECS_AI_HOME/lib/_internal/gitignore-render.py"
AGENTS_MD_RENDER="$SPECS_AI_HOME/lib/_internal/agents-md-render.py"

GITIGNORE_MARKER_BEGIN="# --- ai-specs: agent-generated files (managed by ai-specs sync-agent) ---"
GITIGNORE_MARKER_END="# --- end ai-specs ---"

echo ""
echo "specs-ai init"
echo "  target:  $TARGET_PATH"
echo "  name:    $PROJECT_NAME"
echo "  force:   $([ $FORCE -eq 1 ] && echo "yes" || echo "no")"
echo ""

# 1. Create directories
mkdir -p "$SKILLS_DIR" "$AGENTS_D_DIR"
echo "  ✓ ensure $AI_SPECS_DIR/skills/"
echo "  ✓ ensure $AI_SPECS_DIR/agents.d/"

# 1b. Drop a README inside agents.d/ so users know what to put there.
AGENTS_D_README="$AGENTS_D_DIR/README.md"
if [[ -f "$AGENTS_D_README" && $FORCE -eq 0 ]]; then
    echo "  ✓ keep   ai-specs/agents.d/README.md"
else
    cp "$TEMPLATES_DIR/agents.d-readme.md" "$AGENTS_D_README"
    echo "  ✓ wrote  ai-specs/agents.d/README.md"
fi

# 2. Copy bundled skills
for skill in skill-creator skill-sync; do
    src="$BUNDLED_SKILLS_DIR/$skill"
    dst="$SKILLS_DIR/$skill"
    if [[ ! -d "$src" ]]; then
        echo "  ✗ missing bundled skill at $src — corrupt install?" >&2
        exit 1
    fi
    if [[ -d "$dst" && $FORCE -eq 0 ]]; then
        echo "  ✓ keep   skills/$skill (use --force to overwrite)"
    else
        rm -rf "$dst"
        cp -R "$src" "$dst"
        echo "  ✓ copy   skills/$skill"
    fi
done

# 3. Render ai-specs.toml from template
if [[ -f "$TOML_PATH" && $FORCE -eq 0 ]]; then
    echo "  ✓ keep   ai-specs/ai-specs.toml"
else
    sed "s/{{PROJECT_NAME}}/$PROJECT_NAME/g" \
        "$TEMPLATES_DIR/ai-specs.toml.tmpl" > "$TOML_PATH"
    echo "  ✓ wrote  ai-specs/ai-specs.toml"
fi

# 4. AGENTS.md is fully generated from ai-specs/* — render it now.
python3 "$AGENTS_MD_RENDER" "$TARGET_PATH" "$AGENTS_PATH"

# 5. Append agent-block to root .gitignore (idempotent via marker)
append_block() {
    [[ -f "$ROOT_GITIGNORE" ]] || touch "$ROOT_GITIGNORE"
    if [[ -s "$ROOT_GITIGNORE" ]]; then
        local last_byte
        last_byte="$(tail -c 1 "$ROOT_GITIGNORE" | od -An -c | tr -d ' ')"
        [[ "$last_byte" != "\\n" ]] && printf '\n' >> "$ROOT_GITIGNORE"
    fi
    cat "$TEMPLATES_DIR/gitignore-root.tmpl" >> "$ROOT_GITIGNORE"
}

strip_block() {
    awk -v begin="$GITIGNORE_MARKER_BEGIN" -v end="$GITIGNORE_MARKER_END" '
        $0 == begin { in_block = 1; next }
        in_block && $0 == end { in_block = 0; next }
        !in_block { print }
    ' "$ROOT_GITIGNORE" > "$ROOT_GITIGNORE.tmp" && mv "$ROOT_GITIGNORE.tmp" "$ROOT_GITIGNORE"
}

if [[ -f "$ROOT_GITIGNORE" ]] && grep -qxF "$GITIGNORE_MARKER_BEGIN" "$ROOT_GITIGNORE"; then
    if [[ $FORCE -eq 1 ]]; then
        strip_block
        append_block
        echo "  ✓ refresh .gitignore (agent block)"
    else
        echo "  ✓ keep   .gitignore (agent block present)"
    fi
else
    append_block
    echo "  ✓ append .gitignore (agent block)"
fi

# 6. Generate ai-specs/.gitignore (always, derived)
python3 "$GITIGNORE_RENDER" "$TOML_PATH" "$AI_GITIGNORE"

# 7. Next steps
cat <<EOF

✓ specs-ai initialized at $TARGET_PATH

Next steps:
  1. Edit  $AI_SPECS_DIR/ai-specs.toml
       - set [agents].enabled       (claude, cursor, opencode, codex, copilot, gemini)
       - add [[deps]]               (vendored skills from git)
       - add [mcp.*] sections       (MCP servers)
  2. Run   specs-ai sync             (vendor deps + regenerate AGENTS.md + fan-out per agent)
  3. Commit:
       - ai-specs/ai-specs.toml
       - ai-specs/skills/skill-creator/  ai-specs/skills/skill-sync/
       - ai-specs/skills/<your-local-skills>/
       - AGENTS.md   .gitignore

Generated agent files (.claude/, .cursor/, opencode.json, .mcp.json, CLAUDE.md, ...) are
gitignored — they are regenerated by 'specs-ai sync-agent' on every clone.
EOF
