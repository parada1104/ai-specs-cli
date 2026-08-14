#!/usr/bin/env bash
# sync-agent.sh — fan out skills + MCP + slash commands to per-agent locations.
#
# Usage:
#   ai-specs sync-agent [path] [--all | --<agent>...] [-v|--verbose]
#   ai-specs sync-agent --source-root <root> --target <path> [--all | --<agent>...] [-v|--verbose]
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
GITIGNORE_RENDER="$AI_SPECS_HOME/lib/_internal/gitignore-render.py"
TARGET_RESOLVE_PY="$AI_SPECS_HOME/lib/_internal/target-resolve.py"
FLATTEN_SKILLS_PY="$AI_SPECS_HOME/lib/_internal/flatten-resolved-skills.py"
PROJECT_CACHE_PY="$AI_SPECS_HOME/lib/_internal/project-cache.py"
RECIPE_MATERIALIZE_PY="$AI_SPECS_HOME/lib/_internal/recipe-materialize.py"
AGENTS_RENDER_PY="$AI_SPECS_HOME/lib/_internal/agents-render.py"
BRIEF_RENDER_POLICY_PY="$AI_SPECS_HOME/lib/_internal/brief-render-policy.py"
HOOKS_RENDER_PY="$AI_SPECS_HOME/lib/_internal/hooks-render.py"
usage() {
    cat <<'EOF'
Usage: ai-specs sync-agent [path] [--all | --<agent>...] [-v|--verbose]
       ai-specs sync-agent --source-root <root> --target <path> [--all | --<agent>...] [-v|--verbose]

Render per-agent configs from the root manifest.

Arguments:
  path             Target path when using the legacy single-target form

Flags:
  --source-root    Root project that owns ai-specs/ai-specs.toml (default: target)
  --target         Target directory receiving derived local artifacts
  --resolved-hooks Pre-resolved runtime-hooks JSON (from recipe-materialize)
  --all            All agents listed under [agents].enabled in ai-specs.toml
  --claude         Claude Code  (CLAUDE.md, .claude/skills, .mcp.json)
  --cursor         Cursor       (.cursor/mcp.json)
  --opencode       OpenCode     (opencode.json, .opencode/skills, .opencode/commands)
  --codex          Codex        (.codex/config.toml)
  --copilot        GitHub Copilot (.github/copilot-instructions.md)
  --gemini         Gemini CLI   (GEMINI.md, .gemini/skills, .gemini/settings.json)
  --pi             Pi (pi.dev)  (.pi/skills, .mcp.json)
  --omp            Oh My Pi     (.omp/skills, .omp/mcp.json, .omp/commands)
  -v, --verbose    Print full per-step detail instead of compact summaries

If no selector is given, defaults to --all.
EOF
}

TARGET_PATH=""
SOURCE_ROOT=""
SELECT_ALL=0
EXPLICIT_SOURCE_ROOT=0
EXPLICIT_TARGET=0
RECIPE_MCP_JSON=""
RESOLVED_CONFIG_JSON=""
RESOLVED_HOOKS_JSON=""
VERBOSE=0
declare -a SELECTED_AGENTS=()

while [[ $# -gt 0 ]]; do
    case "$1" in
        --source-root)      SOURCE_ROOT="${2:-}"; EXPLICIT_SOURCE_ROOT=1; shift 2 ;;
        --target)           TARGET_PATH="${2:-}"; EXPLICIT_TARGET=1; shift 2 ;;
        --recipe-mcp)       RECIPE_MCP_JSON="${2:-}"; shift 2 ;;
        --resolved-config)  RESOLVED_CONFIG_JSON="${2:-}"; shift 2 ;;
        --resolved-hooks)   RESOLVED_HOOKS_JSON="${2:-}"; shift 2 ;;
        --all)              SELECT_ALL=1; shift ;;
        --claude|--cursor|--opencode|--codex|--copilot|--gemini|--pi|--omp)
            SELECTED_AGENTS+=("${1#--}"); shift ;;
        -v|--verbose)  VERBOSE=1; shift ;;
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

        # Generate resolved-config for subrepo AGENTS.md enrichment.
        # Use --resolved-config-only so no skills are copied, no hooks run,
        # no lock is written, and no recipe-mcp temp file is created.
        STANDALONE_RESOLVED_CONFIG_TEMP="$(mktemp -t ai-specs-resolved-config-XXXXXX.json)"
        trap 'rm -f "$STANDALONE_RESOLVED_CONFIG_TEMP"' EXIT
        if ! python3 "$RECIPE_MATERIALIZE_PY" "$ROOT_PATH" "$AI_SPECS_HOME" \
            --resolved-config-out "$STANDALONE_RESOLVED_CONFIG_TEMP" \
            --resolved-config-only 2>&1; then
            echo "WARNING: resolved-config generation failed; subrepo AGENTS.md will be rendered without structured fields." >&2
        fi

        FORWARD_ARGS=()
        if [[ $SELECT_ALL -eq 1 ]]; then
            FORWARD_ARGS+=("--all")
        elif [[ ${#SELECTED_AGENTS[@]} -gt 0 ]]; then
            for agent in "${SELECTED_AGENTS[@]}"; do
                FORWARD_ARGS+=("--$agent")
            done
        fi
        [[ $VERBOSE -eq 1 ]] && FORWARD_ARGS+=("--verbose")
        # Forward resolved-config so subrepo AGENTS.md gets structured fields
        if [[ -f "$STANDALONE_RESOLVED_CONFIG_TEMP" ]]; then
            FORWARD_ARGS+=("--resolved-config" "$STANDALONE_RESOLVED_CONFIG_TEMP")
        fi

        # Nest only the children — parent keeps framing (header above + footer below).
        for resolved_target in "${RESOLVED_TARGETS[@]}"; do
            echo "  syncing $resolved_target"
            if ! AI_SPECS_SYNC_NESTED=1 bash "$0" --source-root "$ROOT_PATH" --target "$resolved_target" "${FORWARD_ARGS[@]}"; then
                echo "ERROR: sync-agent failed for target: $resolved_target" >&2
                echo "       Stopped on first failure; no overall success reported." >&2
                exit 1
            fi
        done
        echo ""
        echo "✓ sync-agent complete"
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

# Guard: project must be initialized before any recipe/flatten work.
# This runs before materialize so an uninitialized project gets a helpful
# "Run ai-specs init first" message instead of a "recipe materialize failed" error.
if [[ ! -f "$TOML_PATH" ]]; then
    echo "ERROR: $TOML_PATH not found. Run 'ai-specs init $SOURCE_ROOT' first." >&2
    exit 1
fi

# Materialize recipes into cache FIRST so flatten/merge-commands see up-to-date content.
# When --recipe-mcp is not passed, run materialize now and capture the temp MCP path.
if [[ -z "$RECIPE_MCP_JSON" ]]; then
    _MATERIALIZE_OUT="$(mktemp -t ai-specs-materialize-XXXXXX)"
    trap 'rm -f "$_MATERIALIZE_OUT"' EXIT
    if ! python3 "$RECIPE_MATERIALIZE_PY" "$SOURCE_ROOT" "$AI_SPECS_HOME" >"$_MATERIALIZE_OUT" 2>&1; then
        echo "ERROR: recipe materialize failed" >&2
        grep -v '^RECIPE_MCP_TEMP:' "$_MATERIALIZE_OUT" >&2 || true
        exit 1
    fi
    RECIPE_MCP_JSON="$(grep '^RECIPE_MCP_TEMP:' "$_MATERIALIZE_OUT" | cut -d: -f2- || true)"
fi

if [[ -n "$(shopt -p inherit_errexit 2>/dev/null)" ]]; then
    shopt -s inherit_errexit
fi

# print_step_output FILE — print a step's captured stdout or stderr file.
# Verbose mode: cat the file bytes as-is (byte-identical, including trailing
# blank lines). Compact mode: drop lines whose first non-whitespace char is
# one of the success/detail-noise markers (✓ · ⇢ ▸), keeping every other
# non-blank line (warnings/notices: !, ✗, ℹ, ...) intact.
# Takes a file path (not a string) so command substitution cannot strip
# trailing newlines from the replayed output.
print_step_output() {
    local file="$1"
    [[ -f "$file" ]] || return 0
    [[ -s "$file" ]] || return 0
    if [[ $VERBOSE -eq 1 ]]; then
        cat "$file"
        return 0
    fi
    local line stripped
    while IFS= read -r line || [[ -n "$line" ]]; do
        stripped="${line#"${line%%[![:space:]]*}"}"
        [[ -z "$stripped" ]] && continue
        case "$stripped" in
            '✓'*|'·'*|'⇢'*|'▸'*) continue ;;
        esac
        printf '%s\n' "$line"
    done < "$file"
}

# run_step LABEL CMD [ARGS...] — print "  syncing LABEL", run CMD capturing
# its stdout and stderr separately (preserving which stream each line came
# from), then print each through print_step_output on its original stream.
# On failure, print the FULL unfiltered stdout/stderr before returning the
# command's exit status so the caller's existing error handling still runs.
run_step() {
    local label="$1"; shift
    echo "  syncing $label"
    local out_file err_file rc=0
    out_file="$(mktemp)"
    err_file="$(mktemp)"
    set +e
    "$@" >"$out_file" 2>"$err_file"
    rc=$?
    set -e
    if [[ $rc -ne 0 ]]; then
        [[ -s "$out_file" ]] && cat "$out_file"
        [[ -s "$err_file" ]] && cat "$err_file" >&2
        rm -f "$out_file" "$err_file"
        return $rc
    fi
    print_step_output "$out_file"
    print_step_output "$err_file" >&2
    rm -f "$out_file" "$err_file"
    return 0
}

# Flatten resolved skills into the per-project CLI cache; merge commands (local wins).
# Both steps go through run_step so their ✓ detail lines respect compact/verbose.
RESOLVED_SKILLS_DIR="$(python3 "$PROJECT_CACHE_PY" "$SOURCE_ROOT" path resolved-skills)"
run_step "flatten resolved skills" python3 "$FLATTEN_SKILLS_PY" "$SOURCE_ROOT" "$RESOLVED_SKILLS_DIR"
MERGED_COMMANDS_DIR="$(python3 "$PROJECT_CACHE_PY" "$SOURCE_ROOT" path root)/merged-commands"
run_step "merge commands" python3 "$PROJECT_CACHE_PY" "$SOURCE_ROOT" merge-commands "$MERGED_COMMANDS_DIR"

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

make_skills_symlink() {
    # Absolute symlink for cache-backed resolved-skills (out-of-tree).
    local target_abs="$1"
    local link_path="$2"
    local link_dir
    link_dir="$(dirname "$link_path")"
    mkdir -p "$link_dir"
    local abs
    abs="$(python3 -c "import os,sys; print(os.path.realpath(sys.argv[1]))" "$target_abs")"

    if [[ -L "$link_path" ]]; then
        local existing
        existing="$(readlink "$link_path")"
        if [[ "$existing" == "$abs" ]]; then
            # Noise (keep ·): idempotent success detail; filtered in compact mode.
            echo "    · symlink ok      $link_path → $abs"
            return 0
        fi
        rm "$link_path"
    elif [[ -e "$link_path" ]]; then
        echo "    ✗ refuse to overwrite non-symlink: $link_path" >&2
        return 1
    fi
    ln -s "$abs" "$link_path"
    echo "    ✓ symlink created $link_path → $abs"
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
            # Noise (keep ·): idempotent success detail; filtered in compact mode.
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
    # Subrepo/fan-out side-output: filter ✓ "wrote ..." via run_step like other steps.
    run_step "ai-specs/.gitignore" python3 "$GITIGNORE_RENDER" "$TOML_PATH" "$TARGET_AI_SPECS/.gitignore"
    mirror_directory "$RESOLVED_SKILLS_DIR" "$TARGET_AI_SKILLS"
    mirror_directory "$MERGED_COMMANDS_DIR" "$TARGET_AI_COMMANDS"
    if [[ "$(python3 "$BRIEF_RENDER_POLICY_PY" "$TOML_PATH")" == "true" ]]; then
        local render_args=("$TOML_PATH" "$TARGET_AGENTS_MD")
        if [[ -n "$RESOLVED_CONFIG_JSON" && -f "$RESOLVED_CONFIG_JSON" ]]; then
            render_args+=("--resolved-config" "$RESOLVED_CONFIG_JSON")
        fi
        python3 "$AGENTS_RENDER_PY" "${render_args[@]}"
    else
        [[ -f "$TARGET_AGENTS_MD" ]] || {
            echo "ERROR: $TARGET_AGENTS_MD not found and brief.render = false." >&2
            echo "       Create AGENTS.md manually or set [brief].render = true." >&2
            exit 1
        }
        echo "    ℹ skipped AGENTS.md (brief.render = false)"
    fi
}

# Resolve enabled agents from ai-specs.toml
ENABLED_JSON="$(python3 "$TOML_READ" "$TOML_PATH" agents)"
declare -a ENABLED_AGENTS=()
while IFS= read -r agent; do
    [[ -n "$agent" ]] && ENABLED_AGENTS+=("$agent")
done < <(python3 -c "import json,sys; [print(a) for a in json.loads(sys.argv[1]).get('enabled', [])]" "$ENABLED_JSON")

# Pick targets; guard before per-agent fan-out (flatten already ran — cache is populated).
declare -a TARGETS=()
if [[ $SELECT_ALL -eq 1 || ${#SELECTED_AGENTS[@]} -eq 0 ]]; then
    TARGETS=(${ENABLED_AGENTS[@]+"${ENABLED_AGENTS[@]}"})
else
    TARGETS=("${SELECTED_AGENTS[@]}")
fi

if [[ ${#TARGETS[@]} -eq 0 ]]; then
    echo "WARNING: no agents to sync. Set [agents].enabled in ai-specs.toml." >&2
    exit 0
fi

MCP_COUNT="$(python3 - "$TOML_PATH" "$RECIPE_MCP_JSON" <<'PY'
import sys, tomllib, json
with open(sys.argv[1], "rb") as f:
    manifest_mcp = tomllib.load(f).get("mcp", {}) or {}
recipe_mcp = {}
try:
    with open(sys.argv[2]) as f:
        recipe_mcp = json.load(f)
except (FileNotFoundError, json.JSONDecodeError):
    pass
print(len(manifest_mcp) + len(recipe_mcp))
PY
)"

if [[ "${AI_SPECS_SYNC_NESTED:-0}" != "1" ]]; then
    echo ""
    echo "ai-specs sync-agent"
    echo "  source root: $SOURCE_ROOT"
    echo "  target:      $TARGET_PATH"
    echo "  agents:      ${TARGETS[*]}"
    echo "  enabled:     ${ENABLED_AGENTS[*]:-(none)}"
    echo "  mcp:         $MCP_COUNT server(s)"
    echo ""
    echo "  derived artifacts: AGENTS.md, ai-specs/.gitignore, ai-specs/skills/**, ai-specs/commands/**, agent-configs"
fi
ensure_target_workspace

# For root workspace, agents consume from cache flatten + merged commands
if [[ "$TARGET_PATH" == "$SOURCE_ROOT" ]]; then
    SKILLS_SOURCE="$RESOLVED_SKILLS_DIR"
    COMMANDS_SOURCE="$MERGED_COMMANDS_DIR"
else
    SKILLS_SOURCE="$TARGET_AI_SKILLS"
    COMMANDS_SOURCE="$TARGET_AI_COMMANDS"
fi

sync_one_agent() {
    local agent="$1"
    if ! platform_get "$agent" native >/dev/null 2>&1; then
        echo "  ✗ unknown agent: $agent" >&2
        return 0
    fi

    local is_enabled=0 e
    for e in "${ENABLED_AGENTS[@]}"; do
        [[ "$e" == "$agent" ]] && is_enabled=1 && break
    done
    if [[ $is_enabled -eq 0 ]]; then
        echo "  ! $agent not in [agents].enabled — syncing anyway"
    fi

    local instr
    instr="$(platform_get "$agent" instructions_path)" || return $?
    if [[ -n "$instr" ]]; then
        make_relative_symlink "$TARGET_AGENTS_MD" "$TARGET_PATH/$instr" || return $?
    fi

    local skills skills_link
    skills="$(platform_get "$agent" skills_dir)" || return $?
    if [[ -n "$skills" ]]; then
        skills_link="$TARGET_PATH/$skills"
        if [[ -e "$skills_link" && ! -L "$skills_link" ]]; then
            # Legacy installs may have a real skills directory; replace so
            # fan-out can reconcile to the canonical symlink.
            rm -rf "$skills_link" || return $?
        fi
        make_skills_symlink "$SKILLS_SOURCE" "$skills_link" || return $?
    fi

    local mcp_path mcp_key
    mcp_path="$(platform_get "$agent" mcp_config_path)" || return $?
    mcp_key="$(platform_get "$agent" mcp_key)" || return $?
    if [[ -n "$mcp_path" && -n "$mcp_key" ]]; then
        if [[ "$MCP_COUNT" -gt 0 ]]; then
            python3 "$MCP_RENDER" "$TOML_PATH" "$agent" \
                "$TARGET_PATH/$mcp_path" "$mcp_key" \
                --recipe-mcp "$RECIPE_MCP_JSON" || return $?
        else
            # Notice (not noise): must survive compact mode — same class as
            # "skipped AGENTS.md", which already uses ℹ.
            echo "    ℹ mcp skipped (no [mcp.*] in manifest)"
        fi
    fi

    local cmd_dir dest copied src
    cmd_dir="$(platform_get "$agent" commands_dir)" || return $?
    if [[ -n "$cmd_dir" && -d "$COMMANDS_SOURCE" ]]; then
        dest="$TARGET_PATH/$cmd_dir"
        rm -rf "$dest" || return $?
        mkdir -p "$dest" || return $?
        copied=0
        for src in "$COMMANDS_SOURCE"/*.md; do
            [[ -f "$src" ]] || continue
            cp "$src" "$dest/$(basename "$src")" || return $?
            copied=$((copied + 1))
        done
        if [[ $copied -gt 0 ]]; then
            echo "    ✓ commands     $cmd_dir/ ($copied file(s))"
        fi
    fi

    local hooks_target
    hooks_target="$(platform_get "$agent" runtime_hooks_target)" || return $?
    if [[ -n "$hooks_target" && -n "$RESOLVED_HOOKS_JSON" && -f "$RESOLVED_HOOKS_JSON" ]]; then
        if python3 "$HOOKS_RENDER_PY" "$RESOLVED_HOOKS_JSON" "$agent" "$TARGET_PATH"; then
            echo "    ✓ runtime hooks $hooks_target"
        fi
    fi
}

for agent in "${TARGETS[@]}"; do
    run_step "$agent" sync_one_agent "$agent"
done

if [[ "${AI_SPECS_SYNC_NESTED:-0}" != "1" ]]; then
    echo ""
    echo "✓ sync-agent complete"
fi
