#!/usr/bin/env bash
# install.sh — Install ai-specs globally.
#
# Clones (or updates) this repo to $AI_SPECS_HOME (default: ~/.ai-specs),
# then symlinks bin/ai-specs into $INSTALL_BIN (default: ~/.local/bin).
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/parada1104/ai-specs-cli/main/install.sh | bash
#   bash install.sh                       # from a local clone
#
# Env overrides:
#   AI_SPECS_HOME=~/path        Where to clone the repo (default: ~/.ai-specs)
#   INSTALL_BIN=~/path          Where to symlink the entrypoint (default: ~/.local/bin)
#   AI_SPECS_REPO=git://...     Repo URL for first install (default: github.com/parada1104/ai-specs-cli)
#   AI_SPECS_REF=tag-or-branch  Git ref to checkout (default: main)

set -e

AI_SPECS_HOME="${AI_SPECS_HOME:-$HOME/.ai-specs}"
INSTALL_BIN="${INSTALL_BIN:-$HOME/.local/bin}"
AI_SPECS_REPO="${AI_SPECS_REPO:-https://github.com/parada1104/ai-specs-cli.git}"
AI_SPECS_REF="${AI_SPECS_REF:-main}"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
RED='\033[0;31m'
BOLD='\033[1m'
NC='\033[0m'

echo -e "${BOLD}ai-specs installer${NC}"
echo "==================="
echo -e "Home: ${BOLD}$AI_SPECS_HOME${NC}"
echo -e "Bin:  ${BOLD}$INSTALL_BIN/ai-specs${NC}"
echo ""

command -v git >/dev/null 2>&1 || { echo -e "${RED}error: git is required${NC}" >&2; exit 1; }
command -v python3 >/dev/null 2>&1 || { echo -e "${RED}error: python3 is required${NC}" >&2; exit 1; }

# 1. Clone or update the repo
if [ -d "$AI_SPECS_HOME/.git" ]; then
    echo -e "${YELLOW}[1/3]${NC} Updating existing install at $AI_SPECS_HOME"

    # Detect dirty working tree before attempting any update.
    # Mode-only dirt (e.g. chmod applied to 100644 files by a previous installer)
    # is auto-remediated: restore tracked modes and continue.  Real content
    # changes still abort.
    if [ -n "$(git -C "$AI_SPECS_HOME" status --porcelain 2>/dev/null)" ]; then
        if [ -z "$(git -C "$AI_SPECS_HOME" -c core.fileMode=false status --porcelain 2>/dev/null)" ]; then
            echo "Restoring file modes altered by a previous installer..." >&2
            git -C "$AI_SPECS_HOME" checkout -- . || {
                echo -e "${RED}error: could not restore file modes at $AI_SPECS_HOME${NC}" >&2
                exit 1
            }
        else
            echo -e "${RED}error: working tree at $AI_SPECS_HOME has uncommitted changes${NC}" >&2
            echo ""
            echo -e "  ${BOLD}git -C $AI_SPECS_HOME status${NC}"
            echo ""
            echo "Resolve any modified, added, or deleted files before updating."
            echo "You can stash changes with: git -C $AI_SPECS_HOME stash"
            exit 1
        fi
    fi

    git -C "$AI_SPECS_HOME" fetch --tags origin
    git -C "$AI_SPECS_HOME" checkout "$AI_SPECS_REF"

    if ! git -C "$AI_SPECS_HOME" pull --ff-only origin "$AI_SPECS_REF"; then
        echo -e "${RED}error: git pull failed for $AI_SPECS_HOME (ref: $AI_SPECS_REF)${NC}" >&2
        exit 1
    fi

    # Post-pull verification: HEAD must match origin/<ref>
    LOCAL_HEAD="$(git -C "$AI_SPECS_HOME" rev-parse HEAD)"
    REMOTE_HEAD="$(git -C "$AI_SPECS_HOME" rev-parse "origin/$AI_SPECS_REF")"
    if [ "$LOCAL_HEAD" != "$REMOTE_HEAD" ]; then
        echo -e "${RED}error: after pull, HEAD ($(echo "$LOCAL_HEAD" | head -c 8)) does not match origin/$AI_SPECS_REF ($(echo "$REMOTE_HEAD" | head -c 8))${NC}" >&2
        echo "The repository may be in an unexpected state. Try removing and re-cloning:"
        echo -e "  ${BOLD}rm -rf $AI_SPECS_HOME && curl -fsSL https://raw.githubusercontent.com/parada1104/ai-specs-cli/main/install.sh | bash${NC}"
        exit 1
    fi
elif [ -e "$AI_SPECS_HOME" ]; then
    echo -e "${RED}error: $AI_SPECS_HOME exists but is not a git repo${NC}" >&2
    exit 1
else
    echo -e "${YELLOW}[1/3]${NC} Cloning $AI_SPECS_REPO → $AI_SPECS_HOME"
    # Partial clone (blobs on demand) keeps the full commit graph, which
    # `ai-specs upgrade` needs for its `merge-base --is-ancestor` divergence
    # guard. A shallow clone would break that check, so it is not used here.
    # Older git has no --filter: fall back to a plain clone.
    if ! git clone --filter=blob:none --branch "$AI_SPECS_REF" \
            "$AI_SPECS_REPO" "$AI_SPECS_HOME" 2>/dev/null; then
        git clone --branch "$AI_SPECS_REF" "$AI_SPECS_REPO" "$AI_SPECS_HOME"
    fi
fi

# Drop subtrees the CLI never reads at runtime. Best effort by contract: the
# helper warns and exits 0 on any failure, leaving a usable full checkout.
if [ -f "$AI_SPECS_HOME/lib/_internal/narrow-checkout.sh" ]; then
    bash "$AI_SPECS_HOME/lib/_internal/narrow-checkout.sh" "$AI_SPECS_HOME" || true
fi

chmod +x "$AI_SPECS_HOME/bin/ai-specs"
# Bundled skill scripts (shipped to projects via init)
chmod +x "$AI_SPECS_HOME/bundled-skills/skill-sync/assets/"*.sh 2>/dev/null || true
chmod +x "$AI_SPECS_HOME/bundled-skills/skill-sync/assets/"*.py 2>/dev/null || true

# 2. Install Python dependencies (Rich + Questionary for the interactive init TUI)
VENDOR_DIR="$AI_SPECS_HOME/lib/_vendor"
_has_deps() {
    python3 -c "import rich, questionary" 2>/dev/null && return 0
    [ -d "$VENDOR_DIR" ] && python3 -c "import sys; sys.path.insert(0, '$VENDOR_DIR'); import rich, questionary" 2>/dev/null && return 0
    return 1
}
if _has_deps; then
    echo "  TUI deps (rich + questionary) already available"
else
    echo -e "${YELLOW}[2/3]${NC} Installing TUI dependencies (rich + questionary)"
    python3 -m pip install --upgrade --quiet --target "$VENDOR_DIR" 'rich>=13.0.0,<15' 'questionary>=2.0.0,<2.1' || {
        echo -e "${YELLOW}warning:${NC} could not install TUI deps; init will prompt on first use or fall back to classic mode"
    }
fi

# 3. Symlink entrypoint
echo -e "${YELLOW}[3/3]${NC} Symlinking entrypoint"
mkdir -p "$INSTALL_BIN"
ln -sf "$AI_SPECS_HOME/bin/ai-specs" "$INSTALL_BIN/ai-specs"

VERSION="$(cat "$AI_SPECS_HOME/VERSION" 2>/dev/null || echo "unknown")"

echo ""
echo -e "${GREEN}✓ Installed ai-specs $VERSION${NC}"
echo ""

if ! echo ":$PATH:" | grep -q ":$INSTALL_BIN:"; then
    echo -e "${YELLOW}note:${NC} $INSTALL_BIN is not in your PATH. Add this to your shell rc:"
    echo -e "  ${BOLD}export PATH=\"$INSTALL_BIN:\$PATH\"${NC}"
    echo ""
fi

echo -e "Test it:             ${BOLD}ai-specs version${NC}"
echo -e "Bootstrap a project: ${BOLD}cd <your-project> && ai-specs init${NC}"
echo -e "Then:                ${BOLD}ai-specs sync${NC}"
