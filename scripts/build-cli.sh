#!/usr/bin/env bash
# build-cli.sh — reproducible multi-arch build for the ai-specs CLI binary.
#
# Builds cmd/ai-specs for every target in the release matrix:
#
#   darwin/arm64  darwin/amd64  linux/amd64  linux/arm64
#
# Invariants (the same ones the gate's build uses, so the committed digests are
# independently verifiable by any reviewer with a Go toolchain): CGO_ENABLED=0,
# -trimpath, -buildvcs=false. The CLI carries no link-time version: `ai-specs
# version` reads <home>/VERSION verbatim (internal/cli/version.go), so the same
# source and toolchain produce identical bytes for each target.
#
# CANONICAL TOOLCHAIN: the committed bin/SHA256SUMS is the release trust root
# that install.sh and `ai-specs upgrade` verify a download against, and Go
# compiles different stdlib bytes per Go release. Digests MUST be regenerated
# with go1.24.13 — the same version the release CI pins
# (.github/workflows/release-binaries.yml). The script warns (does not fail:
# contributors may build with any recent Go for local testing) when the active
# toolchain is not canonical, so a digest regeneration with the wrong Go version
# is caught before it is committed.
#
# There is deliberately no `ai-specs-current` copy (unlike scripts/build-gate.sh's
# `worktree-gate-current`): nothing reads one for the CLI, and a local build is
# simply `go build ./cmd/ai-specs`.
#
# Usage:
#   scripts/build-cli.sh [dist_dir]
#     dist_dir   output directory; defaults to "$ROOT/dist".
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="${1:-$ROOT/dist}"

if ! command -v go >/dev/null 2>&1; then
    echo "build-cli.sh: error: 'go' is required to build the ai-specs CLI but was not found on PATH" >&2
    echo "build-cli.sh: error: a Go toolchain is a contributor prerequisite only; install.sh falls back to it, users normally download a verified asset" >&2
    exit 1
fi

if [[ ! -f "$ROOT/VERSION" ]]; then
    echo "build-cli.sh: error: no VERSION file at $ROOT/VERSION" >&2
    exit 1
fi
VERSION="$(tr -d '[:space:]' < "$ROOT/VERSION")"
if [[ -z "$VERSION" ]]; then
    echo "build-cli.sh: error: VERSION file is empty" >&2
    exit 1
fi

GO_VERSION="$(go version | awk '{print $3}')"
if [[ "$GO_VERSION" != "go1.24.13" ]]; then
    echo "build-cli.sh: warning: active toolchain is $GO_VERSION, not the canonical go1.24.13" >&2
    echo "build-cli.sh: warning: SHA256SUMS regenerated with $GO_VERSION will NOT match the release CI or the committed trust root" >&2
fi

echo "build-cli.sh: building ai-specs $VERSION into $DIST_DIR"
mkdir -p "$DIST_DIR"

targets=(
    "darwin arm64"
    "darwin amd64"
    "linux amd64"
    "linux arm64"
)
for target in "${targets[@]}"; do
    read -r goos goarch <<< "$target"
    out="$DIST_DIR/ai-specs-$goos-$goarch"
    echo "build-cli.sh:   $goos/$goarch -> $out"
    (
        cd "$ROOT"
        CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
            go build -trimpath -buildvcs=false -o "$out" ./cmd/ai-specs
    )
done

echo "build-cli.sh: done — 4 targets built"
