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
    echo -e "${YELLOW}[1/2]${NC} Updating existing install"
    git -C "$AI_SPECS_HOME" fetch --tags origin
    git -C "$AI_SPECS_HOME" checkout "$AI_SPECS_REF"
    git -C "$AI_SPECS_HOME" pull --ff-only origin "$AI_SPECS_REF" 2>/dev/null || true
elif [ -e "$AI_SPECS_HOME" ]; then
    echo -e "${RED}error: $AI_SPECS_HOME exists but is not a git repo${NC}" >&2
    exit 1
else
    echo -e "${YELLOW}[1/2]${NC} Cloning $AI_SPECS_REPO → $AI_SPECS_HOME"
    git clone --branch "$AI_SPECS_REF" "$AI_SPECS_REPO" "$AI_SPECS_HOME"
fi

chmod +x \
    "$AI_SPECS_HOME/bin/ai-specs" \
    "$AI_SPECS_HOME/lib/"*.sh \
    "$AI_SPECS_HOME/lib/_internal/"*.py \
    "$AI_SPECS_HOME/lib/_internal/"*.sh 2>/dev/null || true
# Bundled skill scripts (shipped to projects via init)
chmod +x "$AI_SPECS_HOME/bundled-skills/skill-sync/assets/"*.sh 2>/dev/null || true
chmod +x "$AI_SPECS_HOME/bundled-skills/skill-sync/assets/"*.py 2>/dev/null || true

# 2. Symlink entrypoint
echo -e "${YELLOW}[2/2]${NC} Symlinking entrypoint"
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
