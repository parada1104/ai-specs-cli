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
REFRESH_BUNDLED_PY="$AI_SPECS_HOME/lib/_internal/refresh-bundled.py"
RECIPE_MATERIALIZE_PY="$AI_SPECS_HOME/lib/_internal/recipe-materialize.py"
SYNC_AGENT_SH="$AI_SPECS_HOME/lib/sync-agent.sh"

PLAN_JSON="$(python3 "$TARGET_RESOLVE_PY" "$TARGET_PATH")" || {
    echo "ERROR: target resolution failed before any writes." >&2
    exit 1
}

ROOT_PATH="$(python3 -c 'import json,sys; print(json.loads(sys.stdin.read())["root"])' <<<"$PLAN_JSON")"
TOML_PATH="$ROOT_PATH/ai-specs/ai-specs.toml"
AI_GITIGNORE="$ROOT_PATH/ai-specs/.gitignore"
if [[ ! -f "$TOML_PATH" ]]; then
    echo "ERROR: $TOML_PATH not found." >&2
    echo "       Run 'ai-specs init $ROOT_PATH' first." >&2
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
RECIPE_MCP_TEMP="$(mktemp -t ai-specs-recipe-mcp-XXXXXX.json)"
python3 "$RECIPE_MATERIALIZE_PY" "$ROOT_PATH" "$AI_SPECS_HOME" --recipe-mcp-out "$RECIPE_MCP_TEMP"

# Daemon identity is the canonical git root (parent of --git-common-dir), so
# every worktree of the same repo shares one mcp-proxy daemon. Non-git roots
# fall back to ROOT_PATH; the daemon spawn downstream will fail loudly because
# shared MCPs require a git repository.
if GIT_COMMON_DIR="$(git -C "$ROOT_PATH" rev-parse --path-format=absolute --git-common-dir 2>/dev/null)"; then
    GIT_ROOT="$(dirname "$GIT_COMMON_DIR")"
else
    GIT_ROOT="$ROOT_PATH"
fi

PROXY_NAMED_CONFIG="$GIT_ROOT/.ai-specs/run/proxy.named-config.json"
if [[ -f "$PROXY_NAMED_CONFIG" ]]; then
    if command -v uvx >/dev/null 2>&1; then
        echo "▸ ensure mcp-proxy daemon"
        if ! python3 "$AI_SPECS_HOME/lib/_internal/mcp-daemon.py" ensure "$GIT_ROOT" \
                --named-config "$PROXY_NAMED_CONFIG"; then
            echo "ERROR: daemon ensure failed; aborting before fan-out step." >&2
            rm -f "$RECIPE_MCP_TEMP"
            exit 1
        fi
    else
        echo "WARN: uvx not in PATH — shared MCPs will render as stdio for this sync." >&2
        echo "      Install uv from https://docs.astral.sh/uv/ to enable the shared daemon." >&2
        # Local degradation: rewrite the per-render recipe-mcp temp so the
        # downstream mcp-render path treats every shared MCP as stdio. The
        # manifest on disk is NOT modified; degradation lasts only for this
        # invocation of sync. Strip `mode` from the merged (manifest +
        # recipe) MCP map — `mcp-render` does `{**manifest, **recipe_mcp}`,
        # so a mode-less entry in recipe_mcp wins over the manifest's
        # `mode = "shared"` and yields a stdio render.
        python3 - "$TOML_PATH" "$RECIPE_MCP_TEMP" <<'PY'
import json
import sys
import tomllib
from pathlib import Path

toml_path = Path(sys.argv[1])
temp_path = Path(sys.argv[2])

with toml_path.open("rb") as f:
    manifest_mcp = tomllib.load(f).get("mcp", {}) or {}

merged: dict = {sid: dict(cfg) for sid, cfg in manifest_mcp.items() if isinstance(cfg, dict)}
try:
    existing = json.loads(temp_path.read_text())
    if isinstance(existing, dict):
        for sid, cfg in existing.items():
            if isinstance(cfg, dict):
                merged[sid] = dict(cfg)
except (FileNotFoundError, json.JSONDecodeError):
    pass

stripped = 0
for sid, cfg in merged.items():
    if cfg.pop("mode", None) is not None:
        stripped += 1

temp_path.write_text(json.dumps(merged, indent=2) + "\n")
if stripped:
    print(f"  ⚠  stripped mode from {stripped} shared MCP(s) for stdio fallback render", file=sys.stderr)
PY
        # Remove the named-config so a future sync (with uvx restored)
        # re-triggers ensure_daemon afresh, and the daemon-running doctor
        # check does not confuse a degraded sync with a real daemon.
        rm -f "$PROXY_NAMED_CONFIG"
    fi
fi

echo "▸ target fan-out"
for idx in "${!RESOLVED_TARGETS[@]}"; do
    target="${RESOLVED_TARGETS[$idx]}"
    label="${RESOLVED_TARGET_LABELS[$idx]}"
    echo "  ▸ $label → $target"
    if ! bash "$SYNC_AGENT_SH" --source-root "$ROOT_PATH" --target "$target" --all --recipe-mcp "$RECIPE_MCP_TEMP"; then
        echo "ERROR: sync failed for target $target ($label). Stopped on first failure; previous writes are not rolled back." >&2
        rm -f "$RECIPE_MCP_TEMP"
        exit 1
    fi
done

rm -f "$RECIPE_MCP_TEMP"

echo ""
echo "✓ ai-specs sync complete"
