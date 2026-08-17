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
Usage: ai-specs sync [path] [--ignore-cli-version] [-v|--verbose]

Reconcile a project's ai-specs/ with its root manifest:
  - resolve [root, ...project.subrepos]
  - vendor [[deps]] once in the root workspace
  - regenerate AGENTS.md auto-invoke table
  - fan out local derived artifacts to every resolved target

Arguments:
  path      Project root (default: current directory)

Flags:
  --ignore-cli-version  Skip [tool] CLI version policy check (warns on stderr)
  --refresh-gates       Explicitly refresh customized gate hooks: save exact
                        pre-refresh bytes to a cache-only immutable backup,
                        then replace (never set by an ordinary sync)
  -v, --verbose         Print full per-step detail instead of compact summaries
EOF
}

TARGET_PATH=""
IGNORE_CLI_VERSION=""
REFRESH_GATES=""
VERBOSE=0
while [[ $# -gt 0 ]]; do
    case "$1" in
        -h|--help) usage; exit 0 ;;
        --ignore-cli-version) IGNORE_CLI_VERSION="--ignore-cli-version"; shift ;;
        --refresh-gates) REFRESH_GATES="--refresh-gates"; shift ;;
        -v|--verbose) VERBOSE=1; shift ;;
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
GITIGNORE_ROOT_REFRESH="$AI_SPECS_HOME/lib/_internal/gitignore-root-refresh.py"
REFRESH_BUNDLED_PY="$AI_SPECS_HOME/lib/_internal/refresh-bundled.py"
CLI_VERSION_PY="$AI_SPECS_HOME/lib/_internal/cli_version.py"
RECIPE_MATERIALIZE_PY="$AI_SPECS_HOME/lib/_internal/recipe-materialize.py"
AGENTS_RENDER_PY="$AI_SPECS_HOME/lib/_internal/agents-render.py"
BRIEF_RENDER_POLICY_PY="$AI_SPECS_HOME/lib/_internal/brief-render-policy.py"
SYNC_AGENT_SH="$AI_SPECS_HOME/lib/sync-agent.sh"

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

    # A mktemp failure (unwritable or full TMPDIR) must name itself instead of
    # surfacing later as whatever abort message the wrapped command produces.
    #
    # Compact mode cannot apply here: filtering happens on captured files, and
    # there are none. The step runs straight through, so its detail lines reach
    # the terminal. The warning says so rather than leaving raw output
    # unexplained.
    if ! out_file="$(mktemp 2>/dev/null)" || ! err_file="$(mktemp 2>/dev/null)"; then
        echo "  ! cannot create temporary files (check TMPDIR); running this step with unfiltered output" >&2
        rm -f "${out_file:-}" 2>/dev/null || true
        set +e
        "$@"
        rc=$?
        set -e
        return $rc
    fi

    set +e
    "$@" >"$out_file" 2>"$err_file"
    rc=$?
    # errexit stays OFF until this helper has finished its own cleanup.
    #
    # `[[ -s f ]] && cat f` is exempt from errexit when `[[` fails (a non-final
    # command in an && list), but NOT when `cat` itself fails — reachable via
    # SIGPIPE on an early-closed stdout, or a full disk. Restoring errexit
    # before these lines meant such a failure aborted the script from inside
    # run_step: both temp files leaked and the wrapped command's status was
    # replaced by cat's. Only bare call sites are affected, which is 5 of the 6
    # here, so this is the common path rather than the exotic one.
    #
    # The restore itself is mandatory: `set` options are shell-global, not
    # function-local, so dropping it would silently disable errexit for the
    # remainder of the script.
    if [[ $rc -ne 0 ]]; then
        [[ -s "$out_file" ]] && cat "$out_file"
        [[ -s "$err_file" ]] && cat "$err_file" >&2
        rm -f "$out_file" "$err_file"
        set -e
        return $rc
    fi
    print_step_output "$out_file"
    print_step_output "$err_file" >&2
    rm -f "$out_file" "$err_file"
    set -e
    return 0
}

PLAN_JSON="$(python3 "$TARGET_RESOLVE_PY" "$TARGET_PATH")" || {
    echo "ERROR: target resolution failed before any writes." >&2
    exit 1
}

ROOT_PATH="$(python3 -c 'import json,sys; print(json.loads(sys.stdin.read())["root"])' <<<"$PLAN_JSON")"
IFS=$'\t' read -r PLANNING_ROOT TOPOLOGY_RESOLVED TOPOLOGY_VIA <<< "$(python3 -c 'import json,sys; d=json.loads(sys.stdin.read()); print("\t".join([d["planning_root"], d["topology"]["resolved"], d["topology"]["via"]]))' <<<"$PLAN_JSON")"
TOML_PATH="$ROOT_PATH/ai-specs/ai-specs.toml"
AI_GITIGNORE="$ROOT_PATH/ai-specs/.gitignore"
if [[ ! -f "$TOML_PATH" ]]; then
    echo "ERROR: $TOML_PATH not found." >&2
    echo "       Run 'ai-specs init $ROOT_PATH' first." >&2
    exit 1
fi

python3 "$CLI_VERSION_PY" check-sync "$ROOT_PATH" "$AI_SPECS_HOME" $IGNORE_CLI_VERSION || exit 1

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
echo "  planning: $PLANNING_ROOT"
echo "  topology: $TOPOLOGY_RESOLVED (via $TOPOLOGY_VIA)"
echo "  targets: ${RESOLVED_TARGET_LABELS[*]}"
echo "  fan-out: declared-only (project.subrepos; .gitmodules is advisory-only)"
echo "  derived: AGENTS.md, ai-specs/.gitignore, ai-specs/skills/**, ai-specs/commands/**, agent-configs"
echo ""

run_step "ai-specs/.gitignore" python3 "$GITIGNORE_RENDER" "$TOML_PATH" "$AI_GITIGNORE"

run_step "root .gitignore (agent block)" python3 "$GITIGNORE_ROOT_REFRESH" "$ROOT_PATH" "$AI_SPECS_HOME/templates/gitignore-root.tmpl"

run_step "bundled skills + commands" python3 "$REFRESH_BUNDLED_PY" "$ROOT_PATH" "$AI_SPECS_HOME"

run_step "vendored skills" python3 "$VENDOR_SKILLS_PY" "$ROOT_PATH"

RECIPE_MCP_TEMP="$(mktemp -t ai-specs-recipe-mcp-XXXXXX.json)"
RESOLVED_CONFIG_TEMP="$(mktemp -t ai-specs-resolved-config-XXXXXX.json)"
RESOLVED_HOOKS_TEMP="$(mktemp -t ai-specs-resolved-hooks-XXXXXX.json)"
trap 'rm -f "$RECIPE_MCP_TEMP" "$RESOLVED_CONFIG_TEMP" "$RESOLVED_HOOKS_TEMP"' EXIT
RECIPE_OUT_FILE="$(mktemp)"
RECIPE_ERR_FILE="$(mktemp)"
set +e
python3 "$RECIPE_MATERIALIZE_PY" "$ROOT_PATH" "$AI_SPECS_HOME" --recipe-mcp-out "$RECIPE_MCP_TEMP" --resolved-config-out "$RESOLVED_CONFIG_TEMP" --resolved-hooks-out "$RESOLVED_HOOKS_TEMP" $REFRESH_GATES >"$RECIPE_OUT_FILE" 2>"$RECIPE_ERR_FILE"
RECIPE_RC=$?
set -e
RECIPE_NAMES="$( { grep -oE '▸ recipe [^ ]+' "$RECIPE_OUT_FILE" | sed -E 's/.*recipe //' | paste -sd, - ; } 2>/dev/null || true)"
if [[ -n "$RECIPE_NAMES" ]]; then
    echo "  syncing recipes → ${RECIPE_NAMES//,/, }"
else
    echo "  syncing recipes"
fi
if [[ $RECIPE_RC -ne 0 ]]; then
    [[ -s "$RECIPE_OUT_FILE" ]] && cat "$RECIPE_OUT_FILE"
    [[ -s "$RECIPE_ERR_FILE" ]] && cat "$RECIPE_ERR_FILE" >&2
    rm -f "$RECIPE_OUT_FILE" "$RECIPE_ERR_FILE"
    exit $RECIPE_RC
fi
print_step_output "$RECIPE_OUT_FILE"
print_step_output "$RECIPE_ERR_FILE" >&2
rm -f "$RECIPE_OUT_FILE" "$RECIPE_ERR_FILE"

sync_agents_render() {
    if [[ "$(python3 "$BRIEF_RENDER_POLICY_PY" "$TOML_PATH")" == "true" ]]; then
        python3 "$AGENTS_RENDER_PY" "$TOML_PATH" "$ROOT_PATH/AGENTS.md" --preserve-if-runtime-brief --resolved-config "$RESOLVED_CONFIG_TEMP"
    else
        echo "  ℹ skipped AGENTS.md (brief.render = false)"
    fi
}
run_step "AGENTS.md" sync_agents_render

export AI_SPECS_SYNC_NESTED=1
for idx in "${!RESOLVED_TARGETS[@]}"; do
    target="${RESOLVED_TARGETS[$idx]}"
    label="${RESOLVED_TARGET_LABELS[$idx]}"
    SYNC_AGENT_ARGS=(--source-root "$ROOT_PATH" --target "$target" --all --recipe-mcp "$RECIPE_MCP_TEMP" --resolved-config "$RESOLVED_CONFIG_TEMP" --resolved-hooks "$RESOLVED_HOOKS_TEMP")
    [[ $VERBOSE -eq 1 ]] && SYNC_AGENT_ARGS+=(--verbose)
    if ! run_step "$label → $target" bash "$SYNC_AGENT_SH" "${SYNC_AGENT_ARGS[@]}"; then
        echo "ERROR: sync failed for target $target ($label). Stopped on first failure; previous writes are not rolled back." >&2
        exit 1
    fi
done

python3 "$CLI_VERSION_PY" stamp-meta "$ROOT_PATH" "$AI_SPECS_HOME"

echo ""
echo "✓ ai-specs sync complete"
