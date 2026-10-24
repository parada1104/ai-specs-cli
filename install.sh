#!/usr/bin/env bash
# install.sh — Install ai-specs globally.
#
# Clones (or updates) this repo to $AI_SPECS_HOME (default: ~/.ai-specs) — the
# data home, the trust anchor (the committed digests) and where the still-shell
# verbs live — then installs the compiled ai-specs binary as the entrypoint in
# $INSTALL_BIN (default: ~/.local/bin), verifying a release asset against the
# committed digests when one exists and building locally otherwise.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/parada1104/ai-specs-cli/main/install.sh | bash
#   bash install.sh                       # from a local clone
#
# Env overrides:
#   AI_SPECS_HOME=~/path        Where to clone the repo (default: ~/.ai-specs)
#   INSTALL_BIN=~/path          Where to install the entrypoint (default: ~/.local/bin)
#   AI_SPECS_REPO=git://...     Repo URL for first install (default: github.com/parada1104/ai-specs-cli)
#   AI_SPECS_REF=tag-or-branch  Git ref to checkout (default: main)
#   AI_SPECS_RELEASE_BASE=url   Release download base (default: this repo's releases for VERSION)
#   AI_SPECS_TRUST_ROOT_FILE=p  Committed digests to verify against (default: <home>/bin/SHA256SUMS)
#   AI_SPECS_SKIP_TUI_DEPS=1    Skip the still-Python TUI dependency step (tests)

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
    #
    # git removes the destination itself when a clone fails cleanly (verified
    # for a bad ref and an unreachable remote), but an interrupted transfer can
    # leave residue behind — and then the retry would die with "destination
    # path already exists" instead of falling back. Clear it first, and only
    # when this script is the one that created it.
    if ! git clone --filter=blob:none --branch "$AI_SPECS_REF" \
            "$AI_SPECS_REPO" "$AI_SPECS_HOME" 2>/dev/null; then
        # Only reachable from the branch where $AI_SPECS_HOME did not exist
        # beforehand, so any residue here was created by the failed clone. The
        # non-empty guard is defense against a future refactor moving this.
        if [ -n "$AI_SPECS_HOME" ] && [ -e "$AI_SPECS_HOME" ]; then
            rm -rf "$AI_SPECS_HOME"
        fi
        git clone --branch "$AI_SPECS_REF" "$AI_SPECS_REPO" "$AI_SPECS_HOME"
    fi
fi

# Drop subtrees the CLI never reads at runtime. Best effort by contract: the
# helper warns and exits 0 on any failure, leaving a usable full checkout.
if [ -f "$AI_SPECS_HOME/lib/_internal/narrow-checkout.sh" ]; then
    bash "$AI_SPECS_HOME/lib/_internal/narrow-checkout.sh" "$AI_SPECS_HOME" || true
fi

# Nothing in lib/ is chmod-ed, and neither is bin/ai-specs any more: those files
# are committed 100644 and touching their modes makes git report mode-only dirt,
# which breaks later update runs. The entry is the installed binary below, and it
# gets its executable bit when it is installed.
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
if [ -n "${AI_SPECS_SKIP_TUI_DEPS:-}" ]; then
    echo "  skipping TUI deps (AI_SPECS_SKIP_TUI_DEPS is set)"
elif _has_deps; then
    echo "  TUI deps (rich + questionary) already available"
else
    echo -e "${YELLOW}[2/3]${NC} Installing TUI dependencies (rich + questionary)"
    python3 -m pip install --upgrade --quiet --target "$VENDOR_DIR" 'rich>=13.0.0,<15' 'questionary>=2.0.0,<2.1' || {
        echo -e "${YELLOW}warning:${NC} could not install TUI deps; init will prompt on first use or fall back to classic mode"
    }
fi

# 3. Install the entrypoint as the compiled binary (GO-20 U1b-2b).
#
#    The checkout above stays the data home: the trust anchor (the committed
#    digests), the catalogue and the still-shell verbs. What changes is the
#    ENTRY — a real compiled file at $INSTALL_BIN/ai-specs instead of a symlink
#    into the checkout. A verified release asset wins; a local `go build` is the
#    fallback; either way the bytes land atomically, so a failure never leaves a
#    broken or unverified entry behind.
VERSION="$(cat "$AI_SPECS_HOME/VERSION" 2>/dev/null || echo "unknown")"
echo -e "${YELLOW}[3/3]${NC} Installing the entrypoint"

_platform_asset() {
    case "$(uname -s)" in
        Darwin) _asset_os="darwin" ;;
        Linux)  _asset_os="linux" ;;
        *)      return 1 ;;
    esac
    case "$(uname -m)" in
        arm64|aarch64) _asset_arch="arm64" ;;
        x86_64|amd64)  _asset_arch="amd64" ;;
        *)             return 1 ;;
    esac
    printf 'ai-specs-%s-%s' "$_asset_os" "$_asset_arch"
}

_release_base() {
    if [ -n "${AI_SPECS_RELEASE_BASE:-}" ]; then
        printf '%s' "$AI_SPECS_RELEASE_BASE"
    else
        printf 'https://github.com/parada1104/ai-specs-cli/releases/download/v%s' "$VERSION"
    fi
}

_sha256_of() {
    if command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$1" | awk '{print $1}'
    elif command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{print $1}'
    else
        return 1
    fi
}

mkdir -p "$INSTALL_BIN"
ENTRY_TMP="$(mktemp "$INSTALL_BIN/.ai-specs.install.XXXXXX")"
ENTRY_READY=0

# Verify BEFORE installing: the network supplies only bytes, the checkout
# supplies the expected digest. No committed digest means no download is used.
ASSET="$(_platform_asset || true)"
TRUST_ROOT="${AI_SPECS_TRUST_ROOT_FILE:-$AI_SPECS_HOME/bin/SHA256SUMS}"
if [ -n "$ASSET" ] && [ -f "$TRUST_ROOT" ] && command -v curl >/dev/null 2>&1; then
    EXPECTED="$(awk -v asset="$ASSET" '$2 == asset { print $1 }' "$TRUST_ROOT" | head -n 1)"
    if [ -n "$EXPECTED" ] && curl -fsSL "$(_release_base)/$ASSET" -o "$ENTRY_TMP"; then
        GOT="$(_sha256_of "$ENTRY_TMP" || true)"
        if [ -n "$GOT" ] && [ "$GOT" = "$EXPECTED" ]; then
            ENTRY_READY=1
            echo "  verified release asset: $ASSET"
        else
            echo -e "${YELLOW}warning:${NC} $ASSET did not match the committed digest; discarding the download" >&2
        fi
    fi
fi

if [ "$ENTRY_READY" -ne 1 ] && command -v go >/dev/null 2>&1; then
    echo "  no verified asset for this platform; building locally with go"
    if (cd "$AI_SPECS_HOME" && go build -trimpath -buildvcs=false -o "$ENTRY_TMP" ./cmd/ai-specs); then
        ENTRY_READY=1
    fi
fi

if [ "$ENTRY_READY" -ne 1 ]; then
    rm -f "$ENTRY_TMP"
    echo -e "${RED}error: could not install the ai-specs binary${NC}" >&2
    echo "  No verifiable release asset for this platform and no usable Go toolchain." >&2
    echo "  Install Go (https://go.dev/dl) and re-run, or point AI_SPECS_RELEASE_BASE at a mirror." >&2
    exit 1
fi

chmod 0755 "$ENTRY_TMP"
mv -f "$ENTRY_TMP" "$INSTALL_BIN/ai-specs"

# GO-20 U2e: the repo launcher bin/ai-specs execs $AI_SPECS_HOME/dist/ai-specs;
# this is an addition to the U1b-2b install plan. The source is the SAME verified
# file that just became the entry (never a rebuild), and the copy is loud: a
# failed write names the path and aborts the install.
# Canonicalize the HOME DIRECTORY (D-U2e-1), never the binary, so a symlinked
# home still receives dist/ under its real directory.
DIST_HOME="$(cd -P "$AI_SPECS_HOME" 2>/dev/null && pwd -P)" || DIST_HOME="$AI_SPECS_HOME"
DIST_BIN="$DIST_HOME/dist/ai-specs"
if ! mkdir -p "$DIST_HOME/dist"; then
    echo -e "${RED}error: could not create $DIST_HOME/dist${NC}" >&2
    exit 1
fi
if ! cp "$INSTALL_BIN/ai-specs" "$DIST_BIN"; then
    echo -e "${RED}error: could not copy the installed binary to $DIST_BIN${NC}" >&2
    exit 1
fi
if ! chmod 0755 "$DIST_BIN"; then
    echo -e "${RED}error: could not make $DIST_BIN executable${NC}" >&2
    exit 1
fi

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
