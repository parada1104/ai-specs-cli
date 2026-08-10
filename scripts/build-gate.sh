#!/usr/bin/env bash
# build-gate.sh — reproducible multi-arch build for the worktree gate binary.
#
# Builds the zero-dependency Go module at catalog/recipes/worktree-flow/gate
# for every supported target in the release matrix (design §6.3, spec
# "Multi-arch build matrix and reproducibility"):
#
#   darwin/arm64  darwin/amd64  linux/amd64  linux/arm64
#
# Invariants (spec requirement): CGO_ENABLED=0, -trimpath, -buildvcs=false,
# and the CLI version injected at link time, so the same source and toolchain
# produce identical bytes for each target — the committed SHA256SUMS digests
# are independently verifiable by any reviewer with a Go toolchain.
#
# Usage:
#   scripts/build-gate.sh [dist_dir]
#     dist_dir   output directory; defaults to "$ROOT/dist".
#
# Reads $VERSION from the repository root VERSION file. Fails loudly if go is
# absent. Never requires network access; the module has no third-party deps.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="${1:-$ROOT/dist}"
MODULE_DIR="$ROOT/catalog/recipes/worktree-flow/gate"


if ! command -v go >/dev/null 2>&1; then
    echo "build-gate.sh: error: 'go' is required to build the worktree gate but was not found on PATH" >&2
    echo "build-gate.sh: error: a Go toolchain is a contributor prerequisite only; users never build the gate" >&2
    exit 1
fi

if [[ ! -f "$ROOT/VERSION" ]]; then
    echo "build-gate.sh: error: no VERSION file at $ROOT/VERSION" >&2
    exit 1
fi
VERSION="$(tr -d '[:space:]' < "$ROOT/VERSION")"
if [[ -z "$VERSION" ]]; then
    echo "build-gate.sh: error: VERSION file is empty" >&2
    exit 1
fi

echo "build-gate.sh: building worktree-gate $VERSION into $DIST_DIR"
mkdir -p "$DIST_DIR"

targets=(
    "darwin arm64"
    "darwin amd64"
    "linux amd64"
    "linux arm64"
)
for target in "${targets[@]}"; do
    read -r goos goarch <<< "$target"
    out="$DIST_DIR/worktree-gate-$goos-$goarch"
    echo "build-gate.sh:   $goos/$goarch -> $out"
    # The differential runners (parity, metrics, tokenizer) key off
    # dist/worktree-gate-current. Copy the native-platform build output there
    # so a stale binary can never pass as the current implementation
    # (task 1.17 / 2.16).
    if [ "$goos" = "$(uname -s | tr '[:upper:]' '[:lower:]')" ] && [ "$goarch" = "$(uname -m)" ]; then
        cp "$out" "$DIST_DIR/worktree-gate-current"
        echo "build-gate.sh:   native -> $DIST_DIR/worktree-gate-current"
    fi
    (
        cd "$MODULE_DIR"
        CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
            go build -trimpath -buildvcs=false \
            -ldflags "-s -w -X main.version=$VERSION" \
            -o "$out" .
    )
done

echo "build-gate.sh: done — 4 targets built"
