#!/usr/bin/env bash
# verify-gate-sums.sh — canonical digest comparison for the worktree-gate
# release (CI checksum gate, task 0.6 / spec "Binary acquisition, verification
# and cache layout").
#
# The CI release workflow (.github/workflows/release-worktree-gate.yml) emits
#
#   sha256sum worktree-gate-* > SHA256SUMS
#
# from the freshly built artifacts and must fail the release whenever a built
# digest differs from the committed trust root
# catalog/recipes/worktree-flow/bin/SHA256SUMS. A byte-level diff of the two
# files would ALSO fail on incidental differences that are not part of the
# digest contract: the committed file carries a documentation header and a
# hand-maintained line order, while `sha256sum` output is bare and
# lexicographic. This script compares the CANONICAL form of both files — one
# `<sha256>  <name>` line per digest entry, comments/blank lines dropped,
# sorted by asset name — so the release gate answers exactly one question:
# do the built bytes match the committed digests?
#
# Usage:
#   scripts/verify-gate-sums.sh <generated-sums> <committed-sums>
#
# Exit 0 when every digest entry matches; exit 1 on any mismatch (the error
# message tells the operator to regenerate and commit SHA256SUMS).
set -euo pipefail

if [[ $# -ne 2 ]]; then
    echo "usage: $0 <generated-sums> <committed-sums>" >&2
    exit 2
fi

canonicalize() {
    # <64-hex>  worktree-gate-<goos>-<goarch>, one per line, sorted by name.
    grep -E '^[0-9a-f]{64}[[:space:]]+worktree-gate-' "$1" \
        | awk '{print $1 "  " $2}' \
        | sort -k2
}

generated="$(mktemp)"
committed="$(mktemp)"
trap 'rm -f "$generated" "$committed"' EXIT

canonicalize "$1" > "$generated"
canonicalize "$2" > "$committed"

if ! diff -u "$committed" "$generated"; then
    echo "error: built digests differ from the committed SHA256SUMS;" \
         "regenerate catalog/recipes/worktree-flow/bin/SHA256SUMS and commit it" >&2
    exit 1
fi

echo "verify-gate-sums.sh: ok — $(wc -l < "$committed" | tr -d ' ') digest entries match the committed trust root"
