#!/usr/bin/env bash
# install.sh — Install specs-ai globally.
#
# Clones (or updates) this repo to $SPECS_AI_HOME (default: ~/.specs-ai),
# then symlinks bin/specs-ai into $INSTALL_BIN (default: ~/.local/bin).
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/nnodes/specs-ai-cli/main/install.sh | bash
#   bash install.sh                       # from a local clone
#
# Env overrides:
#   SPECS_AI_HOME=~/path        Where to clone the repo (default: ~/.specs-ai)
#   INSTALL_BIN=~/path          Where to symlink the entrypoint (default: ~/.local/bin)
#   SPECS_AI_REPO=git://...     Repo URL for first install (default: github.com/nnodes/specs-ai-cli)
#   SPECS_AI_REF=tag-or-branch  Git ref to checkout (default: main)

set -e

SPECS_AI_HOME="${SPECS_AI_HOME:-$HOME/.specs-ai}"
INSTALL_BIN="${INSTALL_BIN:-$HOME/.local/bin}"
SPECS_AI_REPO="${SPECS_AI_REPO:-https://github.com/nnodes/specs-ai-cli.git}"
SPECS_AI_REF="${SPECS_AI_REF:-main}"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
RED='\033[0;31m'
BOLD='\033[1m'
NC='\033[0m'

echo -e "${BOLD}specs-ai installer${NC}"
echo "==================="
echo -e "Home: ${BOLD}$SPECS_AI_HOME${NC}"
echo -e "Bin:  ${BOLD}$INSTALL_BIN/specs-ai${NC}"
echo ""

command -v git >/dev/null 2>&1 || { echo -e "${RED}error: git is required${NC}" >&2; exit 1; }
command -v python3 >/dev/null 2>&1 || { echo -e "${RED}error: python3 is required${NC}" >&2; exit 1; }

# 1. Clone or update the repo
if [ -d "$SPECS_AI_HOME/.git" ]; then
    echo -e "${YELLOW}[1/2]${NC} Updating existing install"
    git -C "$SPECS_AI_HOME" fetch --tags origin
    git -C "$SPECS_AI_HOME" checkout "$SPECS_AI_REF"
    git -C "$SPECS_AI_HOME" pull --ff-only origin "$SPECS_AI_REF" 2>/dev/null || true
elif [ -e "$SPECS_AI_HOME" ]; then
    echo -e "${RED}error: $SPECS_AI_HOME exists but is not a git repo${NC}" >&2
    exit 1
else
    echo -e "${YELLOW}[1/2]${NC} Cloning $SPECS_AI_REPO → $SPECS_AI_HOME"
    git clone --branch "$SPECS_AI_REF" "$SPECS_AI_REPO" "$SPECS_AI_HOME"
fi

chmod +x "$SPECS_AI_HOME/bin/specs-ai" "$SPECS_AI_HOME/lib/"*.sh "$SPECS_AI_HOME/payload/cli/ai-specs" "$SPECS_AI_HOME/payload/cli/"*.sh

# 2. Symlink entrypoint
echo -e "${YELLOW}[2/2]${NC} Symlinking entrypoint"
mkdir -p "$INSTALL_BIN"
ln -sf "$SPECS_AI_HOME/bin/specs-ai" "$INSTALL_BIN/specs-ai"

VERSION="$(cat "$SPECS_AI_HOME/VERSION" 2>/dev/null || echo "unknown")"

echo ""
echo -e "${GREEN}✓ Installed specs-ai $VERSION${NC}"
echo ""

if ! echo ":$PATH:" | grep -q ":$INSTALL_BIN:"; then
    echo -e "${YELLOW}note:${NC} $INSTALL_BIN is not in your PATH. Add this to your shell rc:"
    echo -e "  ${BOLD}export PATH=\"$INSTALL_BIN:\$PATH\"${NC}"
    echo ""
fi

echo -e "Test it: ${BOLD}specs-ai version${NC}"
echo -e "Bootstrap a project: ${BOLD}cd <your-project> && specs-ai init-project${NC}"
