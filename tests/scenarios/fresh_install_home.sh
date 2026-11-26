#!/usr/bin/env bash
# fresh_install_home.sh — a clean install must resolve its own data home.
#
# Regression for the installer defect: install.sh installs a REAL compiled
# binary at $INSTALL_BIN/ai-specs (default ~/.local/bin/ai-specs) while the data
# home lives at ~/.ai-specs. The resolver used to take BIN_DIR/.. blindly, so
# without AI_SPECS_HOME a fresh install resolved its home to ~/.local, where
# VERSION, templates/ and bin/SHA256SUMS do not exist: `ai-specs version` printed
# "unknown", init/sync could not find their templates, and generated hooks baked
# a dist/ path that never existed (fail-open).
#
# The proof runs with HOME pointed at a throwaway directory and AI_SPECS_HOME
# explicitly absent — the two conditions a real first-time user has.
#
# Usage: bash tests/scenarios/fresh_install_home.sh
# Set AI_SPECS_SOURCE to install from a different clone (default: this checkout).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SOURCE="${AI_SPECS_SOURCE:-$ROOT}"
REF="$(git -C "$SOURCE" rev-parse --abbrev-ref HEAD)"
if ! command -v go >/dev/null 2>&1; then
    echo "fresh_install_home: SKIP — no go toolchain to build the entry" >&2
    exit 0
fi
if ! command -v rsync >/dev/null 2>&1; then
    echo "fresh_install_home: SKIP — rsync is needed to snapshot the working tree" >&2
    exit 0
fi

# Scratch lives outside the repository: `mktemp -d` uses the session temp root.
T="$(mktemp -d)"
trap 'rm -rf "$T"' EXIT

fail() { echo "FAIL: $*" >&2; exit 1; }

# The throwaway HOME below must not starve the build toolchain (a mise/direnv
# shim reads its own config from $HOME). Only the resolver runs need the fake
# HOME, so `go` is reached through a wrapper that restores the original one.
REAL_HOME="$HOME"
GO_REAL="$(command -v go)"
mkdir -p "$T/go-bin"
printf '#!/bin/sh\nexec env HOME="%s" "%s" "$@"\n' "$REAL_HOME" "$GO_REAL" >"$T/go-bin/go"
chmod +x "$T/go-bin/go"

# 1. Install into a throwaway HOME, with AI_SPECS_HOME unset.
#
# The source is a scratch snapshot of the working tree rather than a clone of
# HEAD: install.sh clones a git repo, and a clone would silently install the
# last commit instead of the tree under test.
SNAP="$T/src"
mkdir -p "$SNAP"
rsync -a \
    --exclude .git --exclude dist --exclude cache \
    --exclude tmp --exclude .worktrees --exclude node_modules \
    "$SOURCE/" "$SNAP/"
git -C "$SNAP" init -q -b "$REF"
git -C "$SNAP" add -A
git -C "$SNAP" -c user.email=test@example.invalid -c user.name=test commit -qm "snapshot"

# AI_SPECS_RELEASE_BASE points at a closed port so the run takes the local
# `go build` branch: the point is to exercise the tree's resolver, never a
# previously released asset that predates the fix.
echo "fresh_install_home: installing $REF from the working-tree snapshot into $T/home"
env -u AI_SPECS_HOME \
    HOME="$T/home" \
    PATH="$T/go-bin:$PATH" \
    AI_SPECS_REPO="$SNAP" \
    AI_SPECS_REF="$REF" \
    AI_SPECS_RELEASE_BASE="http://127.0.0.1:9/offline" \
    bash "$SNAP/install.sh" >"$T/install.log" 2>&1 \
    || { sed -n '1,40p' "$T/install.log" >&2; fail "install.sh exited nonzero"; }

HOME_DIR="$T/home/.ai-specs"
ENTRY="$T/home/.local/bin/ai-specs"
[ -f "$HOME_DIR/VERSION" ] || fail "no data home at $HOME_DIR"
[ -f "$ENTRY" ] || fail "no entry installed at $ENTRY"
# The real-binary entry layout is the installer's documented decision; this
# regression is about resolving the home, not about changing the entry kind.
if [ -L "$ENTRY" ]; then
    fail "entry is a symlink; this test pins the real-file layout"
fi

WANT="$(tr -d '[:space:]' <"$HOME_DIR/VERSION")"
[ -n "$WANT" ] || fail "empty VERSION in $HOME_DIR"

# 2. The reported version comes from the data home, not from a guessed parent.
got="$(env -u AI_SPECS_HOME HOME="$T/home" "$ENTRY" version 2>"$T/version.err" | tr -d '[:space:]')" \
    || { cat "$T/version.err" >&2; fail "ai-specs version exited nonzero"; }
[ "$got" = "$WANT" ] || fail "ai-specs version = '$got', want '$WANT' (home resolved outside $HOME_DIR)"
[ "$got" != "unknown" ] || fail "ai-specs version = 'unknown': the home was not resolved"

# 3. The same binary served through the dist/ copy the hooks execute.
dist_got="$(env -u AI_SPECS_HOME HOME="$T/home" "$HOME_DIR/dist/ai-specs" version 2>/dev/null | tr -d '[:space:]')" \
    || fail "dist/ai-specs version exited nonzero"
[ "$dist_got" = "$WANT" ] || fail "dist/ai-specs version = '$dist_got', want '$WANT'"

# 4. A verb that reads the home's data, not just VERSION. Doctor grades the
#    TARGET project (an empty dir is legitimately unhealthy), so the assertion is
#    on its content: it must see the installed version and must not hit the
#    original symptom, "template not found: ~/.local/templates/...".
mkdir -p "$T/project"
env -u AI_SPECS_HOME HOME="$T/home" "$ENTRY" doctor "$T/project" >"$T/doctor.log" 2>&1 || true
if grep -n "installed unknown" "$T/doctor.log" >&2; then
    fail "doctor reports the version as unknown: the home was not resolved"
fi
grep -q "installed $WANT" "$T/doctor.log" \
    || { sed -n '1,20p' "$T/doctor.log" >&2; fail "doctor does not report the installed version $WANT"; }
if grep -nE "template.*not found" "$T/doctor.log" >&2; then
    fail "doctor cannot find the home's templates"
fi

echo "fresh_install_home: OK — $ENTRY resolves home $HOME_DIR (version $WANT)"
