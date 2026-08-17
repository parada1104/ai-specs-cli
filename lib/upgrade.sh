#!/usr/bin/env bash
# upgrade.sh — safely upgrade the global ai-specs installation.
#
# Usage:
#   ai-specs upgrade [--dry-run] [--force] [-v|--verbose]
#
# Flags:
#   --dry-run   Preview the upgrade without modifying the repository.
#   --force     Proceed even if the working tree is dirty.
#   --verbose   Show the full git output instead of one line per step.

set -euo pipefail

SCRIPT_SOURCE="${BASH_SOURCE[0]}"
while [[ -L "$SCRIPT_SOURCE" ]]; do
    DIR="$(cd "$(dirname "$SCRIPT_SOURCE")" && pwd)"
    SCRIPT_SOURCE="$(readlink "$SCRIPT_SOURCE")"
    [[ "$SCRIPT_SOURCE" != /* ]] && SCRIPT_SOURCE="$DIR/$SCRIPT_SOURCE"
done
SCRIPT_DIR="$(cd "$(dirname "$SCRIPT_SOURCE")" && pwd)"
RESOLVED_HOME="$(cd "$SCRIPT_DIR/.." && pwd)"

usage() {
    cat <<'EOF'
Usage: ai-specs upgrade [--dry-run] [--force] [-v|--verbose]

Safely upgrade the global ai-specs installation to the latest origin/main.

Flags:
  --dry-run       Show what would change without modifying the repository.
  --force         Proceed even if the working tree has uncommitted changes.
  -v, --verbose   Show the full git output instead of one line per step.
  -h, --help      Show this help.

Exit codes:
  0   Success or dry-run completed.
  1   Broken or missing installation.
  2   Dev / non-standard checkout.
  3   Pre-flight check failed (dirty tree, non-fast-forward, etc.).
  4   Git fetch or merge failed.
  5   Post-upgrade verification failed (symlink broken).
EOF
}

# --- argument parsing --------------------------------------------------------
DRY_RUN=false
FORCE=false
VERBOSE=0
for arg in "$@"; do
    case "$arg" in
        --dry-run) DRY_RUN=true ;;
        --force) FORCE=true ;;
        -v|--verbose) VERBOSE=1 ;;
        -h|--help) usage; exit 0 ;;
        *) echo "Unknown argument: $arg" >&2; usage >&2; exit 1 ;;
    esac
done

# --- helpers -----------------------------------------------------------------
abort() {
    echo "ai-specs upgrade: $1" >&2
    exit "${2:-1}"
}

# run_step LABEL CMD [ARGS...] — print one progress line, run CMD with its
# output captured, and surface that output only when it matters.
#
# Mirrors the contract established for sync in lib/sync.sh: compact by default,
# full detail under --verbose, and a failing step always prints everything it
# produced so a diagnosis is never hidden. The caller keeps ownership of the
# exit code, so every existing abort path and its code survive unchanged.
run_step() {
    local label="$1"; shift
    echo "  $label"
    local out_file err_file rc=0
    out_file="$(mktemp)"
    err_file="$(mktemp)"
    set +e
    "$@" >"$out_file" 2>"$err_file"
    rc=$?
    set -e
    if [[ $rc -ne 0 || $VERBOSE -eq 1 ]]; then
        [[ -s "$out_file" ]] && cat "$out_file"
        [[ -s "$err_file" ]] && cat "$err_file" >&2
    fi
    rm -f "$out_file" "$err_file"
    return $rc
}

# print_release_report — summarize the crossed versions and replay their
# upgrade notices.
#
# Everything here is best-effort. The fast-forward has already landed by the
# time this runs, so a missing or malformed CHANGELOG.md must degrade to the
# plain "Upgraded: X -> Y" line rather than turn a successful upgrade into a
# failed one.
#
# Notices are displayed, never evaluated or executed: `upgrade` operates on the
# global installation and has no consumer project in scope, so it cannot judge
# project-dependent conditions. Anything conditional belongs to `ai-specs
# doctor`, which has that state.
print_release_report() {
    local changelog="$AI_SPECS_HOME/CHANGELOG.md"
    local parser="$AI_SPECS_HOME/lib/_internal/changelog.py"
    [[ -f "$changelog" && -f "$parser" ]] || return 0

    local summary notices
    summary="$(python3 "$parser" "$changelog" "$CURRENT_VERSION" "$NEW_VERSION" 2>/dev/null || true)"
    if [[ -n "$summary" ]]; then
        echo ""
        echo "What changed"
        printf '%s\n' "$summary"
    fi

    notices="$(python3 "$parser" "$changelog" "$CURRENT_VERSION" "$NEW_VERSION" --notices 2>/dev/null || true)"
    if [[ -n "$notices" ]]; then
        echo ""
        echo "Action required"
        printf '%s\n' "$notices"
    fi

    # Separate the report from whatever the caller prints next.
    if [[ -n "$summary" || -n "$notices" ]]; then
        echo ""
    fi
}

# Resolve the real path of the running ai-specs binary by walking symlinks.
# This is used to verify the install channel.
resolve_binary() {
    local source="${BASH_SOURCE[0]}"
    while [[ -L "$source" ]]; do
        local dir
        dir="$(cd "$(dirname "$source")" && pwd)"
        source="$(readlink "$source")"
        [[ "$source" != /* ]] && source="$dir/$source"
    done
    echo "$(cd "$(dirname "$source")" && pwd)"/"$(basename "$source")"
}

# --- install detection -------------------------------------------------------
DETECT_HOME="${HOME:-$(eval echo ~)}"
EXPECTED_HOME="$DETECT_HOME/.ai-specs"
LOCAL_BIN="$DETECT_HOME/.local/bin/ai-specs"

BINARY_PATH="$(resolve_binary)"

# 1. Check AI_SPECS_HOME is set.
if [[ -z "${AI_SPECS_HOME:-}" ]]; then
    abort "AI_SPECS_HOME is not set. The installation appears broken. Re-run install.sh to repair." 1
fi

# 2. Check the resolved script lives inside ~/.ai-specs.
#    This also serves as the dev-channel guard.
if [[ "$BINARY_PATH" != "$EXPECTED_HOME"/* ]]; then
    abort "This checkout is not the standard global installation (resolved path: $BINARY_PATH). Use 'git pull' manually in the correct directory." 2
fi

# 3. Check AI_SPECS_HOME matches the expected path.
if [[ "$AI_SPECS_HOME" != "$EXPECTED_HOME" ]]; then
    abort "AI_SPECS_HOME ($AI_SPECS_HOME) does not match the expected global path ($EXPECTED_HOME). Re-run install.sh to repair." 1
fi

# 4. Check ~/.ai-specs/.git exists.
if [[ ! -d "$AI_SPECS_HOME/.git" ]]; then
    abort "The installation at $AI_SPECS_HOME is missing its .git directory. Re-run install.sh to repair." 1
fi

# 5. Check ~/.local/bin/ai-specs is a symlink resolving into ~/.ai-specs.
if [[ ! -L "$LOCAL_BIN" ]]; then
    abort "~/.local/bin/ai-specs is missing or not a symlink. Re-run install.sh to repair." 1
fi

SYMLINK_TARGET=""
if command -v readlink &>/dev/null; then
    SYMLINK_TARGET="$(readlink "$LOCAL_BIN")"
fi
if [[ -z "${SYMLINK_TARGET:-}" ]]; then
    # Fallback: walk the symlink ourselves
    local_source="$LOCAL_BIN"
    while [[ -L "$local_source" ]]; do
        local_dir="$(cd "$(dirname "$local_source")" && pwd)"
        local_source="$(readlink "$local_source")"
        [[ "$local_source" != /* ]] && local_source="$local_dir/$local_source"
    done
    SYMLINK_TARGET="$local_source"
fi

# Normalize to absolute path for comparison
SYMLINK_DIR="$(cd "$(dirname "$SYMLINK_TARGET")" && pwd 2>/dev/null || true)"
if [[ -z "${SYMLINK_DIR:-}" ]]; then
    abort "~/.local/bin/ai-specs symlink appears broken. Re-run install.sh to repair." 1
fi

# The symlink must resolve to something inside ~/.ai-specs
if [[ "$SYMLINK_DIR" != "$EXPECTED_HOME"/* ]]; then
    abort "~/.local/bin/ai-specs resolves outside ~/.ai-specs. Re-run install.sh to repair." 1
fi

# --- pre-flight checks -------------------------------------------------------
cd "$AI_SPECS_HOME"

# Check we are on a branch that can fast-forward to origin/main.
# First, ensure origin/main exists (either locally or we can fetch it).
if ! git rev-parse --verify origin/main &>/dev/null; then
    # If origin/main is missing, try a lightweight fetch to establish it.
    # In dry-run we skip this, but pre-flight still needs to know if origin is reachable.
    if [[ "$DRY_RUN" == true ]]; then
        abort "origin/main is not available locally. A real upgrade would fetch it first." 3
    fi
fi

# Verify fast-forward is possible.
# HEAD must be an ancestor of origin/main.
if git rev-parse --verify origin/main &>/dev/null; then
    if ! git merge-base --is-ancestor HEAD origin/main; then
        abort "Local branch has diverged from origin/main. Resolve manually or re-run install.sh." 3
    fi

    # Also verify origin/main is strictly ahead (or equal) to HEAD.
    HEAD_SHA="$(git rev-parse HEAD)"
    ORIGIN_SHA="$(git rev-parse origin/main)"
    if [[ "$HEAD_SHA" == "$ORIGIN_SHA" ]]; then
        UP_TO_DATE=true
    else
        UP_TO_DATE=false
    fi
else
    # origin/main not available yet
    UP_TO_DATE=false
fi

# Working tree cleanliness.
# Mode-only dirt (chmod applied to 100644 files by a previous installer) is
# auto-remediated: if the tree looks dirty under core.fileMode=true but clean
# under core.fileMode=false, the diff is purely mode-bits — restore and continue.
# Real content changes still abort (or proceed with --force).
DIRTY_FILES="$(git status --porcelain)"
if [[ -n "$DIRTY_FILES" ]]; then
    MODE_ONLY_DIRTY="$(git -c core.fileMode=false status --porcelain)"
    if [[ -z "$MODE_ONLY_DIRTY" ]]; then
        echo "Restoring file modes altered by a previous installer..." >&2
        git checkout -- .
    elif [[ "$FORCE" == false ]]; then
        abort "Working tree is dirty. Stash changes, clean the tree, or use --force.\n$DIRTY_FILES" 3
    else
        echo "Warning: working tree is dirty. Proceeding because --force was given." >&2
    fi
fi

# Read current version.
CURRENT_VERSION=""
if [[ -f "$AI_SPECS_HOME/VERSION" ]]; then
    CURRENT_VERSION="$(cat "$AI_SPECS_HOME/VERSION" | tr -d '[:space:]')"
fi

# --- dry-run -----------------------------------------------------------------
if [[ "$DRY_RUN" == true ]]; then
    TARGET_VERSION="unknown"
    if git rev-parse --verify origin/main &>/dev/null; then
        TARGET_VERSION="$(git show origin/main:VERSION 2>/dev/null | tr -d '[:space:]' || echo "unknown")"
    fi
    echo "Dry-run: no changes will be made."
    echo "Current version: $CURRENT_VERSION"
    echo "Target version:  $TARGET_VERSION"
    if [[ "$UP_TO_DATE" == true ]]; then
        echo "Already up to date."
    fi
    exit 0
fi

# --- fetch & merge -----------------------------------------------------------
if ! run_step "fetching origin/main" git fetch origin main; then
    abort "Failed to fetch from origin. Check your network connection." 4
fi

# Re-evaluate after fetch
if ! git merge-base --is-ancestor HEAD origin/main; then
    abort "Local branch has diverged from origin/main after fetch. Resolve manually or re-run install.sh." 3
fi

HEAD_SHA="$(git rev-parse HEAD)"
ORIGIN_SHA="$(git rev-parse origin/main)"
if [[ "$HEAD_SHA" == "$ORIGIN_SHA" ]]; then
    UP_TO_DATE=true
else
    UP_TO_DATE=false
fi

if [[ "$UP_TO_DATE" == false ]]; then
    TARGET_VERSION="$(git show origin/main:VERSION 2>/dev/null | tr -d '[:space:]' || true)"
    MERGE_LABEL="fast-forwarding to origin/main"
    if [[ -n "$CURRENT_VERSION" && -n "$TARGET_VERSION" ]]; then
        MERGE_LABEL="fast-forwarding $CURRENT_VERSION -> $TARGET_VERSION"
    fi
    if ! run_step "$MERGE_LABEL" git merge --ff-only origin/main; then
        abort "Fast-forward merge failed. The local branch may have diverged. Resolve manually or re-run install.sh." 4
    fi
fi

# --- refresh TUI deps (rich + questionary) -----------------------------
VENDOR_DIR="$AI_SPECS_HOME/lib/_vendor"
_tui_deps_ok() {
    python3 -c "import rich, questionary" 2>/dev/null && return 0
    [[ -d "$VENDOR_DIR" ]] && python3 -c "import sys; sys.path.insert(0, '$VENDOR_DIR'); import rich, questionary" 2>/dev/null && return 0
    return 1
}
if ! _tui_deps_ok; then
    run_step "installing TUI dependencies (rich + questionary)" \
        python3 -m pip install --upgrade --quiet --target "$VENDOR_DIR" \
        'rich>=13.0.0,<15' 'questionary>=2.0.0,<2.1' || {
        echo "warning: could not install TUI deps; init will prompt on first use" >&2
    }
elif [[ $VERBOSE -eq 1 ]]; then
    echo "  TUI deps (rich + questionary) already available"
fi

# --- post-upgrade verification -----------------------------------------------
NEW_VERSION=""
if [[ -f "$AI_SPECS_HOME/VERSION" ]]; then
    NEW_VERSION="$(cat "$AI_SPECS_HOME/VERSION" | tr -d '[:space:]')"
fi

if [[ "$CURRENT_VERSION" == "$NEW_VERSION" ]]; then
    echo "Already up to date (version $CURRENT_VERSION)."
else
    echo "Upgraded: $CURRENT_VERSION -> $NEW_VERSION"
    print_release_report
fi

# Symlink integrity check.
if [[ ! -L "$LOCAL_BIN" ]]; then
    abort "Post-upgrade: ~/.local/bin/ai-specs is no longer a symlink. Re-run install.sh to repair." 5
fi

POST_SYMLINK_TARGET=""
if command -v readlink &>/dev/null; then
    POST_SYMLINK_TARGET="$(readlink "$LOCAL_BIN")"
fi
if [[ -z "${POST_SYMLINK_TARGET:-}" ]]; then
    local_source="$LOCAL_BIN"
    while [[ -L "$local_source" ]]; do
        local_dir="$(cd "$(dirname "$local_source")" && pwd)"
        local_source="$(readlink "$local_source")"
        [[ "$local_source" != /* ]] && local_source="$local_dir/$local_source"
    done
    POST_SYMLINK_TARGET="$local_source"
fi

POST_SYMLINK_DIR="$(cd "$(dirname "$POST_SYMLINK_TARGET")" && pwd 2>/dev/null || true)"
if [[ -z "${POST_SYMLINK_DIR:-}" || "$POST_SYMLINK_DIR" != "$EXPECTED_HOME"/* ]]; then
    abort "Post-upgrade: ~/.local/bin/ai-specs symlink is broken or points outside ~/.ai-specs. Re-run install.sh to repair." 5
fi

echo "Symlink integrity verified: $LOCAL_BIN -> $POST_SYMLINK_TARGET"
exit 0
