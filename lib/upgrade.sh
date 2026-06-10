#!/usr/bin/env bash
# upgrade.sh — safely upgrade the global ai-specs installation.
#
# Usage:
#   ai-specs upgrade [--dry-run] [--force]
#
# Flags:
#   --dry-run   Preview the upgrade without modifying the repository.
#   --force     Proceed even if the working tree is dirty.

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
Usage: ai-specs upgrade [--dry-run] [--force]

Safely upgrade the global ai-specs installation to the latest origin/main.

Flags:
  --dry-run   Show what would change without modifying the repository.
  --force     Proceed even if the working tree has uncommitted changes.
  -h, --help  Show this help.

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
for arg in "$@"; do
    case "$arg" in
        --dry-run) DRY_RUN=true ;;
        --force) FORCE=true ;;
        -h|--help) usage; exit 0 ;;
        *) echo "Unknown argument: $arg" >&2; usage >&2; exit 1 ;;
    esac
done

# --- helpers -----------------------------------------------------------------
abort() {
    echo "ai-specs upgrade: $1" >&2
    exit "${2:-1}"
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
if ! git fetch origin main; then
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

if [[ "$UP_TO_DATE" == true ]]; then
    echo "Already up to date (version $CURRENT_VERSION)."
    exit 0
fi

if ! git merge --ff-only origin/main; then
    abort "Fast-forward merge failed. The local branch may have diverged. Resolve manually or re-run install.sh." 4
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
