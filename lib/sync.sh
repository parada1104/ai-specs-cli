#!/usr/bin/env bash
# sync.sh — full reconciliation of a project's ai-specs/ from its manifest.
#
# Pipeline:
#   0. Resolve targets from the root manifest and fail before writes if invalid
#   1. Refresh root ai-specs/.gitignore from [[deps]]
#   2. Refresh bundled skills/commands + lock file in the root workspace
#   3. Vendor external skills in the root workspace only
#   4. Render root AGENTS.md + auto-invoke table
#   5. Fan out derived local artifacts to each resolved target
#
# `.gitmodules` is advisory-only in V1.
# Failure mode is stop-on-first-failure with explicit target reporting.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AI_SPECS_HOME="$(cd "$SCRIPT_DIR/.." && pwd)"

usage() {
    cat <<'EOF'
Usage: ai-specs sync [path]

Reconcile a project's ai-specs/ with its root manifest:
  - resolve [root, ...project.subrepos]
  - vendor [[deps]] once in the root workspace
  - regenerate AGENTS.md auto-invoke table
  - fan out local derived artifacts to every resolved target

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
            echo "Run 'ai-specs sync --help' for usage." >&2
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

TARGET_RESOLVE_PY="$AI_SPECS_HOME/lib/_internal/target-resolve.py"
VENDOR_SKILLS_PY="$AI_SPECS_HOME/lib/_internal/vendor-skills.py"
GITIGNORE_RENDER="$AI_SPECS_HOME/lib/_internal/gitignore-render.py"
AGENTS_MD_RENDER="$AI_SPECS_HOME/lib/_internal/agents-md-render.py"
REFRESH_BUNDLED_PY="$AI_SPECS_HOME/lib/_internal/refresh-bundled.py"
RECIPE_MATERIALIZE_PY="$AI_SPECS_HOME/lib/_internal/recipe-materialize.py"
SYNC_AGENT_SH="$AI_SPECS_HOME/lib/sync-agent.sh"

PLAN_JSON="$(python3 "$TARGET_RESOLVE_PY" "$TARGET_PATH")" || {
    echo "ERROR: target resolution failed before any writes." >&2
    exit 1
}

ROOT_PATH="$(python3 -c 'import json,sys; print(json.loads(sys.stdin.read())["root"])' <<<"$PLAN_JSON")"
TOML_PATH="$ROOT_PATH/ai-specs/ai-specs.toml"
AI_SPECS_DIR="$ROOT_PATH/ai-specs"
AI_GITIGNORE="$AI_SPECS_DIR/.gitignore"
SKILL_SYNC_DIR="$AI_SPECS_DIR/skills/skill-sync/assets"
SYNC_SH="$SKILL_SYNC_DIR/sync.sh"

if [[ ! -f "$TOML_PATH" ]]; then
    echo "ERROR: $TOML_PATH not found." >&2
    echo "       Run 'ai-specs init $ROOT_PATH' first." >&2
    exit 1
fi
if [[ ! -d "$SKILL_SYNC_DIR" ]]; then
    echo "ERROR: $SKILL_SYNC_DIR not found." >&2
    echo "       Run 'ai-specs init --force $ROOT_PATH' to restore bundled skills." >&2
    exit 1
fi

RESOLVED_TARGETS=()
while IFS= read -r target; do
    [[ -n "$target" ]] && RESOLVED_TARGETS+=("$target")
done < <(python3 -c 'import json,sys; [print(t["path"]) for t in json.loads(sys.stdin.read())["targets"]]' <<<"$PLAN_JSON")
RESOLVED_TARGET_LABELS=()
while IFS= read -r label; do
    [[ -n "$label" ]] && RESOLVED_TARGET_LABELS+=("$label")
done < <(python3 -c 'import json,sys; [print("{}:{}".format(t["kind"], t["rel"])) for t in json.loads(sys.stdin.read())["targets"]]' <<<"$PLAN_JSON")

echo ""
echo "ai-specs sync"
echo "  root:    $ROOT_PATH"
echo "  targets: ${RESOLVED_TARGET_LABELS[*]}"
echo "  derived: AGENTS.md, ai-specs/.gitignore, ai-specs/skills/**, ai-specs/commands/**, agent-configs"
echo "  note:    .gitmodules is advisory-only in V1"
echo ""

echo "▸ gitignore-render (root)"
python3 "$GITIGNORE_RENDER" "$TOML_PATH" "$AI_GITIGNORE"

echo "▸ refresh-bundled (root)"
python3 "$REFRESH_BUNDLED_PY" "$ROOT_PATH" "$AI_SPECS_HOME"

echo "▸ vendor-skills (root only)"
python3 "$VENDOR_SKILLS_PY" "$ROOT_PATH"

echo "▸ recipe-materialize (root)"
python3 "$RECIPE_MATERIALIZE_PY" "$ROOT_PATH" "$AI_SPECS_HOME"

echo "▸ agents-md-render (root)"
python3 "$AGENTS_MD_RENDER" "$ROOT_PATH" "$ROOT_PATH/AGENTS.md"

echo "▸ skill-sync (root AGENTS.md auto-invoke)"
(cd "$ROOT_PATH" && bash "$SYNC_SH")

echo "▸ target fan-out"
for idx in "${!RESOLVED_TARGETS[@]}"; do
    target="${RESOLVED_TARGETS[$idx]}"
    label="${RESOLVED_TARGET_LABELS[$idx]}"
    echo "  ▸ $label → $target"
    if ! bash "$SYNC_AGENT_SH" --source-root "$ROOT_PATH" --target "$target" --all; then
        echo "ERROR: sync failed for target $target ($label). Stopped on first failure; previous writes are not rolled back." >&2
        exit 1
    fi
done

echo ""
echo "✓ ai-specs sync complete"
