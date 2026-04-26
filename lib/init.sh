#!/usr/bin/env bash
# init.sh — bootstrap the ai-specs/ standard in a project.
#
# Usage:
#   ai-specs init [path] [--name <project-name>] [--force]
#
# Flags:
#   --name <name>   Project name baked into ai-specs.toml (default: basename of path)
#   --force         Re-copy bundled skills & commands, regenerate AGENTS.md,
#                   and refresh the agent-block in <path>/.gitignore. Default
#                   behavior preserves user-edited files (idempotent).
#
# NEVER overwritten (user-owned, source of truth):
#   <path>/ai-specs/ai-specs.toml   — mutated only by `add-dep` or by the user.
#
# Always (re)generated regardless of --force:
#   <path>/ai-specs/.gitignore      (derived from [[deps]] in ai-specs.toml)
#   <path>/AGENTS.md                (generated artifact)
#   <path>/ai-specs/.ai-specs.lock  (bundled-file SHA baseline)
#
# Layout produced:
#   <path>/
#   ├── AGENTS.md                       (always regenerated from ai-specs/* by `sync`)
#   ├── .gitignore                      (agent-block appended; idempotent via marker)
#   └── ai-specs/
#       ├── ai-specs.toml               (template if missing; source of truth only at root)
#       ├── .gitignore                  (always rendered from ai-specs.toml)
#       ├── skills/
#       │   ├── skill-creator/          (bundled — committable)
#       │   └── skill-sync/             (bundled — committable)
#       │   (optional: vendor policy skills from catalog/ — see ai-specs-cli catalog/README.md)
#       └── commands/
#           └── skills-as-rules.md      (bundled — committable, fan-out to agents)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_SPECS_HOME="$(cd "$SCRIPT_DIR/.." && pwd)"

usage() {
    cat <<'EOF'
Usage: ai-specs init [path] [--name <project-name>] [--force]

Bootstrap the ai-specs standard in a project (idempotent by default).

Arguments:
  path              Project root (default: current directory)

Flags:
  --name <name>     Project name in ai-specs.toml (default: basename of path)
  --force           Re-render templates and re-copy bundled skills even if present
  -h, --help        Show this help

Examples:
  ai-specs init                        # initialize cwd
  ai-specs init ~/code/my-app          # initialize specific path
  ai-specs init --name my-app          # override project name
  ai-specs init --force                # re-render templates (destructive)
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
            echo "Run 'ai-specs init --help' for usage." >&2
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
COMMANDS_DIR="$AI_SPECS_DIR/commands"
TOML_PATH="$AI_SPECS_DIR/ai-specs.toml"
AGENTS_PATH="$TARGET_PATH/AGENTS.md"
ROOT_GITIGNORE="$TARGET_PATH/.gitignore"
AI_GITIGNORE="$AI_SPECS_DIR/.gitignore"

BUNDLED_SKILLS_DIR="$AI_SPECS_HOME/bundled-skills"
BUNDLED_COMMANDS_DIR="$AI_SPECS_HOME/bundled-commands"
TEMPLATES_DIR="$AI_SPECS_HOME/templates"
GITIGNORE_RENDER="$AI_SPECS_HOME/lib/_internal/gitignore-render.py"
AGENTS_MD_RENDER="$AI_SPECS_HOME/lib/_internal/agents-md-render.py"

GITIGNORE_MARKER_BEGIN="# --- ai-specs: agent-generated files (managed by ai-specs sync-agent) ---"
GITIGNORE_MARKER_END="# --- end ai-specs ---"

echo ""
echo "ai-specs init"
echo "  target:  $TARGET_PATH"
echo "  name:    $PROJECT_NAME"
echo "  force:   $([ $FORCE -eq 1 ] && echo "yes" || echo "no")"
echo ""

# 1. Create directories
mkdir -p "$SKILLS_DIR" "$COMMANDS_DIR"
echo "  ✓ ensure $AI_SPECS_DIR/skills/"
echo "  ✓ ensure $AI_SPECS_DIR/commands/"

# 2. Copy bundled skills (contracts only: skill-creator + skill-sync).
#    Optional policy skills → [[deps]] from ai-specs-cli catalog/ (see catalog/README.md).
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

# 2b. Copy bundled commands (same pattern as skills: idempotent, --force overwrites)
if [[ -d "$BUNDLED_COMMANDS_DIR" ]]; then
    for src in "$BUNDLED_COMMANDS_DIR"/*.md; do
        [[ -f "$src" ]] || continue
        base="$(basename "$src")"
        dst="$COMMANDS_DIR/$base"
        if [[ -f "$dst" && $FORCE -eq 0 ]]; then
            echo "  ✓ keep   commands/$base (use --force to overwrite)"
        else
            cp "$src" "$dst"
            echo "  ✓ copy   commands/$base"
        fi
    done
fi

# 3. Render ai-specs.toml from template (ONLY if missing — never overwritten).
#    The TOML is user-owned source of truth: [agents].enabled, [[deps]], [mcp.*]
#    are all hand-edited. `--force` does NOT touch it; mutations go through
#    `add-dep` or by hand.
if [[ -f "$TOML_PATH" ]]; then
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

# 7. Establish the bundled-file SHA baseline (ai-specs/.ai-specs.lock).
#    --init mode: never writes .new sidecars; just records CLI shas so future
#    `refresh-bundled` can diff against them.
echo "▸ refresh-bundled --init"
python3 "$AI_SPECS_HOME/lib/_internal/refresh-bundled.py" \
    "$TARGET_PATH" "$AI_SPECS_HOME" --init

# 8. Next steps
cat <<EOF

✓ ai-specs initialized at $TARGET_PATH

Next steps:
  1. Edit  $AI_SPECS_DIR/ai-specs.toml
       - set [agents].enabled       (claude, cursor, opencode, codex, copilot, gemini)
       - add [[deps]]               (vendored skills from git)
       - add [mcp.*] sections       (MCP servers)
  2. Optional: add recommended policy skills via [[deps]] (see catalog/README.md in the ai-specs-cli repo)
  3. Run   ai-specs sync             (vendor deps + regenerate AGENTS.md + fan-out per agent)
  4. Commit:
       - ai-specs/ai-specs.toml
       - ai-specs/skills/ (skill-creator + skill-sync + vendored + your locals)
       - ai-specs/commands/<your-local-commands>.md
       - AGENTS.md   .gitignore   ai-specs/.ai-specs.lock

Generated agent files (.claude/, .cursor/, opencode.json, .mcp.json, CLAUDE.md, ...) are
gitignored — they are regenerated by 'ai-specs sync-agent' on every clone.
EOF
