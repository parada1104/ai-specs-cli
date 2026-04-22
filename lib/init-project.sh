#!/usr/bin/env bash
# init-project.sh — Bootstrap ai-specs/ standard into a target project.
#
# Creates ai-specs/{ai-specs.toml, cli/, skills/{skill-creator,skill-sync}},
# scaffolds AGENTS.md if missing, appends ai-specs block to .gitignore.
#
# Usage:
#   specs-ai init-project [path] [--name <project>] [--force]

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
PAYLOAD="$REPO_ROOT/payload"
TEMPLATES="$REPO_ROOT/templates"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
RED='\033[0;31m'
BOLD='\033[1m'
NC='\033[0m'

TARGET=""
NAME=""
FORCE=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --name)  NAME="$2"; shift 2 ;;
        --force) FORCE=true; shift ;;
        --help|-h)
            cat <<EOF
Usage: specs-ai init-project [PATH] [OPTIONS]

Bootstrap the ai-specs/ standard in a target project.

Arguments:
  PATH         Target directory (default: current working directory)

Options:
  --name <n>   Project name (default: basename of target path)
  --force      Overwrite existing ai-specs/ directory
  --help       Show this help

What gets created:
  <PATH>/ai-specs/ai-specs.toml          (rendered from template)
  <PATH>/ai-specs/cli/                   (payload — copied from specs-ai-cli)
  <PATH>/ai-specs/skills/skill-creator/  (payload)
  <PATH>/ai-specs/skills/skill-sync/     (payload)
  <PATH>/AGENTS.md                       (only if missing — rendered from template)
  <PATH>/.gitignore                      (ai-specs block appended if missing)
EOF
            exit 0 ;;
        -*)
            echo -e "${RED}Unknown option: $1${NC}" >&2; exit 1 ;;
        *)
            if [ -z "$TARGET" ]; then TARGET="$1"
            else echo -e "${RED}Unexpected arg: $1${NC}" >&2; exit 1; fi
            shift ;;
    esac
done

TARGET="${TARGET:-$(pwd)}"
TARGET="$(cd "$TARGET" 2>/dev/null && pwd)" || {
    echo -e "${RED}error: target path does not exist: $TARGET${NC}" >&2
    exit 1
}
NAME="${NAME:-$(basename "$TARGET")}"

echo -e "${BOLD}specs-ai init-project${NC}"
echo "======================="
echo -e "Target:  ${BOLD}$TARGET${NC}"
echo -e "Project: ${BOLD}$NAME${NC}"
echo ""

AI_SPECS_DIR="$TARGET/ai-specs"

if [ -d "$AI_SPECS_DIR" ] && ! $FORCE; then
    echo -e "${RED}error: $AI_SPECS_DIR already exists${NC}" >&2
    echo -e "Use --force to overwrite, or run 'specs-ai upgrade $TARGET' to refresh payload only." >&2
    exit 1
fi

# 1. Copy payload (cli/ + skills/{skill-creator,skill-sync})
echo -e "${YELLOW}[1/4]${NC} Copying payload (cli + skill-creator + skill-sync)"
mkdir -p "$AI_SPECS_DIR/cli/lib" "$AI_SPECS_DIR/skills"
cp "$PAYLOAD/cli/ai-specs" "$AI_SPECS_DIR/cli/"
cp "$PAYLOAD/cli/init.sh" "$PAYLOAD/cli/sync-agent.sh" "$PAYLOAD/cli/add-skill.sh" "$PAYLOAD/cli/add-dep.sh" "$AI_SPECS_DIR/cli/"
cp "$PAYLOAD/cli/lib/"*.py "$PAYLOAD/cli/lib/"*.sh "$AI_SPECS_DIR/cli/lib/"
cp -R "$PAYLOAD/skills/skill-creator" "$PAYLOAD/skills/skill-sync" "$AI_SPECS_DIR/skills/"
chmod +x "$AI_SPECS_DIR/cli/ai-specs" "$AI_SPECS_DIR/cli/"*.sh

# 2. Render ai-specs.toml from template
echo -e "${YELLOW}[2/4]${NC} Rendering ai-specs/ai-specs.toml"
TOML_DEST="$AI_SPECS_DIR/ai-specs.toml"
if [ -f "$TOML_DEST" ] && ! $FORCE; then
    echo -e "${BLUE}  (preserved existing $TOML_DEST)${NC}"
else
    sed "s/{{PROJECT_NAME}}/$NAME/g" "$TEMPLATES/ai-specs.toml.tmpl" > "$TOML_DEST"
fi

# 3. Render AGENTS.md if missing (never overwrite — user content)
echo -e "${YELLOW}[3/4]${NC} Scaffolding AGENTS.md (if missing)"
AGENTS_DEST="$TARGET/AGENTS.md"
if [ -f "$AGENTS_DEST" ]; then
    echo -e "${BLUE}  (preserved existing $AGENTS_DEST)${NC}"
else
    sed "s/{{PROJECT_NAME}}/$NAME/g" "$TEMPLATES/AGENTS.md.tmpl" > "$AGENTS_DEST"
fi

# 4. Append .gitignore block (idempotent — skip if marker present)
echo -e "${YELLOW}[4/4]${NC} Appending ai-specs block to .gitignore"
GITIGNORE="$TARGET/.gitignore"
MARKER="# --- ai-specs: agent-generated files"
if [ -f "$GITIGNORE" ] && grep -qF "$MARKER" "$GITIGNORE"; then
    echo -e "${BLUE}  (ai-specs block already present)${NC}"
else
    [ -f "$GITIGNORE" ] && echo "" >> "$GITIGNORE"
    cat "$TEMPLATES/gitignore.tmpl" >> "$GITIGNORE"
fi

echo ""
echo -e "${GREEN}✓ ai-specs initialized at $AI_SPECS_DIR${NC}"
echo ""
echo -e "${BOLD}Next steps:${NC}"
echo -e "  1. Edit ${BOLD}$TOML_DEST${NC} — adjust [agents].enabled, add [mcp.*] servers."
echo -e "  2. Run: ${BOLD}cd $TARGET && ./ai-specs/cli/ai-specs sync${NC}"
echo -e "     (vendors [[deps]], regenerates AGENTS.md, distributes to enabled agents)"
