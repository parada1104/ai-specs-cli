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
Usage: ai-specs init [path] [--name <project-name>] [--force] [--tui|--no-tui]

Bootstrap the ai-specs standard in a project (idempotent by default).

Arguments:
  path              Project root (default: current directory)

Flags:
  --name <name>     Project name in ai-specs.toml (default: basename of path)
  --force           Re-render templates and re-copy bundled skills even if present
  --tui             Force interactive onboarding (Rich prompts)
  --no-tui          Skip interactive onboarding (scriptable / CI)
  -h, --help        Show this help

Interactive onboarding (TTY):
  When stdin/stdout are a TTY, no --name/--force/--no-tui is passed, and
  ai-specs.toml does not yet exist, init launches a short wizard to choose
  project name, agents, and recipes. Use --no-tui to keep the classic path.

Examples:
  ai-specs init                        # TTY → wizard; else classic template
  ai-specs init --no-tui               # classic non-interactive bootstrap
  ai-specs init --tui --name my-app    # wizard with name prefilled
  ai-specs init ~/code/my-app          # initialize specific path
  ai-specs init --force                # re-render templates (destructive)
EOF
}

# Defaults
TARGET_PATH=""
PROJECT_NAME=""
FORCE=0
TUI_MODE="auto"   # auto | on | off
TUI_TOML=""       # staged manifest from init_tui.py when set
NAME_EXPLICIT=0   # 1 when --name was passed (disables auto-TUI)

while [[ $# -gt 0 ]]; do
    case "$1" in
        --name)        PROJECT_NAME="${2:-}"; NAME_EXPLICIT=1; shift 2 ;;
        --name=*)      PROJECT_NAME="${1#*=}"; NAME_EXPLICIT=1; shift ;;
        --force)       FORCE=1; shift ;;
        --tui)         TUI_MODE="on"; shift ;;
        --no-tui)      TUI_MODE="off"; shift ;;
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
RECIPE_MATERIALIZE_PY="$AI_SPECS_HOME/lib/_internal/recipe-materialize.py"
AGENTS_RENDER_PY="$AI_SPECS_HOME/lib/_internal/agents-render.py"
BRIEF_RENDER_POLICY_PY="$AI_SPECS_HOME/lib/_internal/brief-render-policy.py"
INIT_TUI_PY="${AI_SPECS_INIT_TUI_PY:-$AI_SPECS_HOME/lib/_internal/init_tui.py}"

GITIGNORE_MARKER_BEGIN="# --- ai-specs: agent-generated files (managed by ai-specs sync-agent) ---"
GITIGNORE_MARKER_END="# --- end ai-specs ---"

# Best-effort cleanup for staging temps (TUI + resolved-config). Registered
# whenever either temp is created; safe no-op when vars are empty.
_ai_specs_init_cleanup() {
    rm -f "${TUI_STAGING:-}" "${TUI_STAGING_TOML:-}" "${TUI_STAGING_JSON:-}" "${RESOLVED_CONFIG_TEMP:-}"
}

# Decide whether to run interactive onboarding.
SHOULD_TUI=0
case "$TUI_MODE" in
    on)  SHOULD_TUI=1 ;;
    off) SHOULD_TUI=0 ;;
    auto)
        if [[ -t 0 && -t 1 && $NAME_EXPLICIT -eq 0 && $FORCE -eq 0 && ! -f "$TOML_PATH" ]]; then
            SHOULD_TUI=1
        fi
        ;;
esac

if [[ $SHOULD_TUI -eq 1 ]]; then
    if [[ -f "$TOML_PATH" ]]; then
        echo "  · skip TUI — ai-specs.toml already exists (never overwritten by init)"
    elif [[ ! -f "$INIT_TUI_PY" ]]; then
        echo "  · skip TUI — init_tui.py missing from this install" >&2
    else
        TUI_STAGING="$(mktemp -t ai-specs-init-tui-XXXXXX)"
        TUI_STAGING_TOML="${TUI_STAGING}.toml"
        TUI_STAGING_JSON="${TUI_STAGING}.json"
        trap '_ai_specs_init_cleanup' EXIT
        # init_tui writes --out as given; json sidecar uses Path.with_suffix('.json')
        set +e
        python3 "$INIT_TUI_PY"             --target "$TARGET_PATH"             --name "$PROJECT_NAME"             --out "$TUI_STAGING_TOML"
        tui_rc=$?
        set -e
        if [[ $tui_rc -eq 0 ]]; then
            TUI_TOML="$TUI_STAGING_TOML"
            if [[ -f "$TUI_STAGING_JSON" ]]; then
                # Prefer wizard name when present
                tui_name="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("name",""))' "$TUI_STAGING_JSON" 2>/dev/null || true)"
                [[ -n "$tui_name" ]] && PROJECT_NAME="$tui_name"
            fi
        elif [[ $tui_rc -eq 1 ]]; then
            echo "ai-specs init: cancelled" >&2
            rm -f "$TUI_STAGING" "$TUI_STAGING_TOML" "$TUI_STAGING_JSON"
            exit 1
        elif [[ $tui_rc -eq 3 ]]; then
            echo "  · TUI unavailable — falling back to classic init" >&2
            rm -f "$TUI_STAGING" "$TUI_STAGING_TOML" "$TUI_STAGING_JSON"
        else
            echo "ERROR: interactive init failed (exit $tui_rc)" >&2
            rm -f "$TUI_STAGING" "$TUI_STAGING_TOML" "$TUI_STAGING_JSON"
            exit "$tui_rc"
        fi
    fi
fi

echo ""
echo "ai-specs init"
echo "  target:  $TARGET_PATH"
echo "  name:    $PROJECT_NAME"
echo "  force:   $([ $FORCE -eq 1 ] && echo "yes" || echo "no")"
echo "  tui:     $([ -n "$TUI_TOML" ] && echo "yes" || echo "no")"
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
elif [[ -n "$TUI_TOML" && -f "$TUI_TOML" ]]; then
    cp "$TUI_TOML" "$TOML_PATH"
    echo "  ✓ wrote  ai-specs/ai-specs.toml (from TUI)"
    rm -f "$TUI_TOML" "${TUI_TOML%.toml}.json" "${TUI_TOML%.*}" 2>/dev/null || true
else
    sed "s/{{PROJECT_NAME}}/$PROJECT_NAME/g" \
        "$TEMPLATES_DIR/ai-specs.toml.tmpl" > "$TOML_PATH"
    echo "  ✓ wrote  ai-specs/ai-specs.toml"
fi

# 3b. Render a baseline AGENTS.md from the freshly written manifest.
#     Best-effort: any failure falls back to the placeholder written below.
#     The if-guard consumes the exit code so set -e cannot abort init.
#     Mirrors sync.sh agents-render block for byte-stability.
if [[ "$(python3 "$BRIEF_RENDER_POLICY_PY" "$TOML_PATH")" == "true" ]]; then
    RESOLVED_CONFIG_TEMP="$(mktemp -t ai-specs-resolved-config-XXXXXX.json)"
    trap '_ai_specs_init_cleanup' EXIT
    if python3 "$RECIPE_MATERIALIZE_PY" "$TARGET_PATH" "$AI_SPECS_HOME" \
           --resolved-config-out "$RESOLVED_CONFIG_TEMP" \
       && python3 "$AGENTS_RENDER_PY" "$TOML_PATH" "$AGENTS_PATH" \
           --preserve-if-runtime-brief --resolved-config "$RESOLVED_CONFIG_TEMP"; then
        echo "  ✓ render AGENTS.md (baseline brief)"
    else
        # Fallback: if render failed, write a one-line placeholder.
        [[ -f "$AGENTS_PATH" ]] || echo "# AGENTS.md - Runtime context" > "$AGENTS_PATH"
        echo "  ! render skipped — fallback placeholder written" >&2
    fi
else
    if [[ -f "$AGENTS_PATH" ]]; then
        echo "  · skipped AGENTS.md (brief.render = false)"
    else
        echo "# AGENTS.md - Runtime context" > "$AGENTS_PATH"
        echo "  · skipped AGENTS.md render (brief.render = false)" >&2
        echo "  ! created placeholder — replace with your manual brief" >&2
    fi
fi

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
