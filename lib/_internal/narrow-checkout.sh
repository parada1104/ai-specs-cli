#!/usr/bin/env bash
# narrow-checkout.sh — drop subtrees the CLI never reads from a global install.
#
# Usage:
#   narrow-checkout.sh <repo-dir>
#
# `~/.ai-specs` is a full clone, but nothing under lib/ or bin/ reads
# openspec/, tests/, .github/ or tmp/ at runtime. Those four are roughly half
# the tracked files. Excluding them from the working tree shrinks the install
# and keeps `ai-specs upgrade` diffs readable.
#
# Contract: this is an optimization, never a precondition. Every failure path
# warns and exits 0, leaving a full — and completely usable — checkout behind.
# The caller must never treat narrowing as a reason to fail an install or an
# upgrade.
#
# Deliberately NOT a shallow clone: `ai-specs upgrade` calls
# `git merge-base --is-ancestor HEAD origin/main` as its divergence guard
# (lib/upgrade.sh), and truncated history makes that check unreliable. Sparse
# checkout narrows the working tree while leaving the commit graph whole.

set -uo pipefail

# Cone-mode sparse checkout is an allowlist, so name what stays rather than
# what goes. A path absent from this list is simply not materialized, which
# means a new top-level runtime directory must be added here.
KEEP_DIRS=(
    lib
    bin
    catalog
    bundled-skills
    bundled-commands
    templates
    scripts
    docs
)

EXCLUDED_LABEL="openspec/, tests/, .github/, tmp/"

warn() {
    echo "  ! narrow-checkout: $1" >&2
}

TARGET="${1:-}"
if [[ -z "$TARGET" ]]; then
    warn "no target directory given; skipping"
    exit 0
fi

if [[ ! -d "$TARGET" ]]; then
    warn "target $TARGET does not exist; skipping"
    exit 0
fi

if ! git -C "$TARGET" rev-parse --git-dir >/dev/null 2>&1; then
    warn "target $TARGET is not a git repository; skipping"
    exit 0
fi

# Reconcile against the CURRENT allowlist rather than short-circuiting on
# "is it sparse at all".
#
# Checking only `core.sparseCheckout == true` meant an install narrowed by an
# older release kept that release's allowlist forever: a top-level runtime
# directory added in a later version would never be materialized, silently, on
# every machine that had already upgraded once (JD C2).
DESIRED="$(printf '%s\n' "${KEEP_DIRS[@]}" | LC_ALL=C sort)"
CURRENT="$(git -C "$TARGET" sparse-checkout list 2>/dev/null | LC_ALL=C sort || true)"

if [[ -n "$CURRENT" && "$CURRENT" == "$DESIRED" ]]; then
    echo "  checkout already narrowed ($EXCLUDED_LABEL excluded)"
    exit 0
fi

# Probe for sparse-checkout support.
#
# `list` is not a usable probe on its own: it also fails on a supported git
# whose worktree is simply not sparse yet ("fatal: this worktree is not
# sparse"), which is exactly the state we are about to fix.
#
# `-h` is used rather than `--help` deliberately. `--help` routes through `git
# help`, which honors `help.format`; a user with `help.format = web` would have
# a browser launched by a capability check. `-h` prints short usage and never
# reaches man or a browser.
#
# Exit codes are not comparable across versions here (129 for a known
# subcommand, 1 for an unknown one), so match on the message git prints when a
# subcommand does not exist.
PROBE="$(git -C "$TARGET" sparse-checkout -h 2>&1 || true)"
if [[ "$PROBE" == *"is not a git command"* || -z "$PROBE" ]]; then
    warn "this git has no sparse-checkout support; keeping the full checkout"
    exit 0
fi

# Uncommitted work inside an excluded subtree would be discarded by the
# checkout that follows. Refuse rather than destroy it — the user keeps a full
# checkout, which costs disk and nothing else.
DIRTY="$(git -C "$TARGET" status --porcelain 2>/dev/null || true)"
if [[ -n "$DIRTY" ]]; then
    warn "working tree has uncommitted changes; keeping the full checkout"
    exit 0
fi

# Apply the allowlist in ONE call.
#
# The previous `init --cone` + `set` split had a destructive window: `init`
# immediately prunes the working tree to root-level files (measured: 20
# top-level entries down to 8, with lib/ and bin/ gone), and only the later
# `set` puts the kept directories back. A failure between them left the install
# without its own CLI (JD C1). `set --cone` initializes and applies together,
# so a failure leaves the tree untouched instead of half-pruned.
if ! git -C "$TARGET" sparse-checkout set --cone "${KEEP_DIRS[@]}" >/dev/null 2>&1; then
    warn "could not apply the sparse checkout; keeping the full checkout"
    exit 0
fi

# Defense in depth: a partial failure inside `set` could still leave the tree
# short of a directory the CLI needs. Verify before claiming success, and never
# report a recovery that did not happen.
MISSING=""
for dir in "${KEEP_DIRS[@]}"; do
    # Only require directories that actually exist at HEAD.
    if git -C "$TARGET" cat-file -e "HEAD:$dir" 2>/dev/null; then
        [[ -d "$TARGET/$dir" ]] || MISSING="$MISSING $dir"
    fi
done

if [[ -n "$MISSING" ]]; then
    warn "sparse checkout left these directories missing:$MISSING"
    if git -C "$TARGET" sparse-checkout disable >/dev/null 2>&1; then
        warn "restored the full checkout"
    else
        # Do not claim a recovery that failed. The install is unusable and the
        # user needs the exact command, not a reassuring message.
        warn "COULD NOT RESTORE the checkout — the installation is incomplete."
        warn "run: git -C $TARGET sparse-checkout disable"
    fi
    exit 0
fi

echo "  narrowed checkout ($EXCLUDED_LABEL excluded)"
exit 0
