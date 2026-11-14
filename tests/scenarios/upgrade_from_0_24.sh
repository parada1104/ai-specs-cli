#!/usr/bin/env bash
# Scenario: an installed v0.24.0 upgrades to this tree and keeps working.
#
# Hermetic and backend-agnostic: plain bash, no network, and no writes outside
# the scratch directory it creates (mktemp honours the caller's temp dir).
# Any backend (this machine, CI, a Space) runs the same file with
# `bash tests/scenarios/upgrade_from_0_24.sh`.
#
# What it proves: the real FROM_TAG installer installs the old release from a
# local bare origin; the installed OLD `ai-specs upgrade` fast-forwards to this
# tree's committed content; and the FIRST verb after that upgrade still works
# because the launcher self-heals the missing dist/ai-specs (#534). Then the
# project keeps working on the new version.
#
# Env:
#   FROM_TAG  old release to install (default: v0.24.0)
#   REPO      repository cloned for the scenario (default: this script's repo)
#   TO_REF    ref whose committed tree is promoted to origin/main (default: HEAD)
#   KEEP=1    keep the scratch directory instead of deleting it
#   BREAK_SELF_HEAL=1
#             fault injection: check (d) runs the first post-upgrade verb with
#             AI_SPECS_GO_BIN pointing at a missing file, so the launcher cannot
#             self-heal and the scenario must exit 1 there. That is how this
#             test is shown to catch the #534 regression; it is never set by the
#             runner.
#
# Exit codes: 0 pass, 1 assertion failed (the FAIL line names the check and
# prints the relevant log tails), 77 skipped with a one-line reason on stderr.
#
# Prerequisites: git, curl, go, tar and python3.11+ (the old CLI is Python),
# plus a full clone that contains the FROM_TAG tag; a shallow clone skips.
#
# Checks: (a) old install + `ai-specs version`, (b) old project init+sync,
# (c) installed old `ai-specs upgrade` fast-forwards old -> new, (d) next verb
# self-heals the binary and prints exactly the new version, (e) new sync,
# (f) new doctor (only the documented tracker-ledger fail-open is tolerated),
# (g) a second verb does not fetch again.
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="${REPO:-$(cd "$SCRIPT_DIR/../.." && pwd)}"
FROM_TAG="${FROM_TAG:-v0.24.0}"
TO_REF="${TO_REF:-HEAD}"
KEEP="${KEEP:-0}"

say() { printf '%s\n' "$*"; }
pass() { printf 'PASS  %s\n' "$*"; }
skip() {
    printf 'SKIP  %s\n' "$*" >&2
    exit 77
}
# fail <check> [log...]: name the check, dump the log tails, exit 1.
fail() {
    local check="$1"
    shift
    printf 'FAIL  %s\n' "$check"
    local log
    for log in "$@"; do
        [ -f "$log" ] || continue
        printf -- '--- %s (tail) ---\n' "$log"
        tail -n 20 "$log"
    done
    exit 1
}

for tool in git curl go tar python3; do
    command -v "$tool" >/dev/null 2>&1 || skip "missing prerequisite: $tool"
done
if command -v shasum >/dev/null 2>&1; then
    sha256_file() { shasum -a 256 "$1" | awk '{print $1}'; }
elif command -v sha256sum >/dev/null 2>&1; then
    sha256_file() { sha256sum "$1" | awk '{print $1}'; }
else
    skip "missing prerequisite: shasum or sha256sum"
fi
# Pin the Go toolchain before HOME is replaced below: a shimmed `go` (mise,
# asdf) resolves its install through the caller's real HOME/GOPATH/GOCACHE, and
# building inside an isolated HOME would try to install a toolchain.
GO_BIN="$(command -v go)"
# Same reason for python3: the old CLI is Python and its scripts import
# tomllib, which a shim cannot provide once HOME points at the scratch dir.
PYTHON_BIN="$(python3 -c 'import sys; print(sys.executable)' 2>/dev/null || true)"
[ -n "$PYTHON_BIN" ] || skip "missing prerequisite: python3"
python3 -c 'import sys; raise SystemExit(0 if sys.version_info >= (3, 11) else 1)' \
    2>/dev/null || skip "python3.11+ is required to run $FROM_TAG (tomllib)"

if [ -n "${TMPDIR:-}" ]; then
    T="$(mktemp -d "$TMPDIR/ai-specs-upgrade-scenario.XXXXXX")" ||
        skip "could not create a scratch directory under $TMPDIR"
else
    T="$(mktemp -d -t ai-specs-upgrade-scenario)" ||
        skip "could not create a scratch directory with mktemp"
fi
cleanup() {
    if [ "$KEEP" = 1 ]; then
        printf 'kept scratch dir: %s\n' "$T"
    else
        rm -rf "$T"
    fi
}
trap cleanup EXIT

O="$T/origin.git"
say "== origin: bare clone of $REPO"
git clone -q --bare "$REPO" "$O" >"$T/clone.log" 2>&1 ||
    fail "could not clone $REPO into the scratch dir" "$T/clone.log"

FROM_SHA="$(git --git-dir="$O" rev-parse --verify --quiet "$FROM_TAG^{commit}" || true)"
[ -n "$FROM_SHA" ] ||
    skip "tag $FROM_TAG is not in the clone of $REPO (shallow clone?)"
TO_SHA="$(git -C "$REPO" rev-parse --verify --quiet "$TO_REF^{commit}" || true)"
[ -n "$TO_SHA" ] || skip "ref $TO_REF does not resolve in $REPO"
git --git-dir="$O" cat-file -e "$TO_SHA^{commit}" 2>/dev/null ||
    skip "commit for $TO_REF is not in the clone of $REPO (shallow clone?)"

OLD_VERSION="$(git --git-dir="$O" show "$FROM_SHA:VERSION")"
NEW_VERSION="$(git --git-dir="$O" show "$TO_SHA:VERSION")"
[ -n "$OLD_VERSION" ] && [ -n "$NEW_VERSION" ] ||
    fail "could not read VERSION from the $FROM_TAG and $TO_REF trees"

# A local release mirror for the self-heal check, built from TO_REF's committed
# tree with the caller's real Go environment (see GO_BIN above).
ASSET="ai-specs-$(uname -s | tr '[:upper:]' '[:lower:]')-$(uname -m |
    sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/')"
mkdir -p "$T/src" "$T/mirror"
git --git-dir="$O" archive "$TO_SHA" | tar -x -C "$T/src" ||
    fail "could not export the $TO_REF tree"
(cd "$T/src" && "$GO_BIN" build -trimpath -buildvcs=false \
    -o "$T/mirror/$ASSET" ./cmd/ai-specs) >"$T/build.log" 2>&1 ||
    fail "could not build the release mirror with $GO_BIN" "$T/build.log"
printf '%s  %s\n' "$(sha256_file "$T/mirror/$ASSET")" "$ASSET" \
    > "$T/mirror/SHA256SUMS"

say "== isolated home (old=$OLD_VERSION, new=$NEW_VERSION)"
mkdir -p "$T/home" "$T/proj"
export HOME="$T/home"
export AI_SPECS_HOME="$HOME/.ai-specs"
export GIT_CONFIG_GLOBAL="$T/gitconfig" GIT_CONFIG_NOSYSTEM=1
git config --global user.email "scenario@example.invalid"
git config --global user.name "scenario"
git config --global init.defaultBranch main
git config --global --add safe.directory '*'
export PATH="$HOME/.local/bin:$(dirname "$PYTHON_BIN"):$PATH"
# No network: the old installer's optional pip step must fail fast, not hang.
export PIP_NO_INPUT=1 PIP_DISABLE_PIP_VERSION_CHECK=1 PIP_NO_INDEX=1
unset AI_SPECS_RELEASE_BASE AI_SPECS_TRUST_ROOT_FILE AI_SPECS_GO_BIN
export AI_SPECS_REPO="file://$O" AI_SPECS_REF=main
git --git-dir="$O" symbolic-ref HEAD refs/heads/main
git --git-dir="$O" update-ref refs/heads/main "$FROM_SHA"
AIBIN="$HOME/.local/bin/ai-specs"

say "== (a) install $FROM_TAG with its own install.sh"
git --git-dir="$O" show "$FROM_SHA:install.sh" >"$T/install-old.sh" ||
    fail "(a) could not read $FROM_TAG:install.sh"
bash "$T/install-old.sh" >"$T/install-old.log" 2>&1 ||
    fail "(a) $FROM_TAG install.sh failed" "$T/install-old.log"
"$AIBIN" version >"$T/version-old.out" 2>"$T/version-old.err" ||
    fail "(a) installed $FROM_TAG ai-specs version failed" \
        "$T/version-old.err" "$T/install-old.log"
[ "$(cat "$T/version-old.out")" = "$OLD_VERSION" ] ||
    fail "(a) old version printed '$(cat "$T/version-old.out")', expected $OLD_VERSION" \
        "$T/version-old.out" "$T/version-old.err"
pass "(a) installed $FROM_TAG and its ai-specs prints $OLD_VERSION"

say "== (b) project init+sync on $OLD_VERSION"
cd "$T/proj" || fail "(b) could not enter the scenario project"
git init -q . || fail "(b) git init failed"
"$AIBIN" init </dev/null >"$T/init-old.log" 2>&1 ||
    fail "(b) old ai-specs init failed" "$T/init-old.log"
"$AIBIN" sync </dev/null >"$T/sync-old.log" 2>&1 ||
    fail "(b) old ai-specs sync failed" "$T/sync-old.log"
pass "(b) old project init and sync succeeded"

say "== (c) promote $TO_REF to origin/main, run the INSTALLED old upgrade"
PROMO="$(git --git-dir="$O" commit-tree "$TO_SHA^{tree}" -p refs/heads/main \
    -m "promote $TO_REF (squash)" 2>"$T/promote.log")" ||
    fail "(c) could not create the promotion commit in the bare clone" "$T/promote.log"
git --git-dir="$O" update-ref refs/heads/main "$PROMO" ||
    fail "(c) could not move origin/main to the promotion commit"
"$AIBIN" upgrade </dev/null >"$T/upgrade.log" 2>&1 ||
    fail "(c) installed $FROM_TAG ai-specs upgrade failed" "$T/upgrade.log"
grep -q "Upgraded: $OLD_VERSION -> $NEW_VERSION" "$T/upgrade.log" ||
    fail "(c) upgrade did not report 'Upgraded: $OLD_VERSION -> $NEW_VERSION'" \
        "$T/upgrade.log"
pass "(c) old upgrade reported Upgraded: $OLD_VERSION -> $NEW_VERSION"

# (d) the first verb after the upgrade must self-heal the missing binary.
export AI_SPECS_RELEASE_BASE="file://$T/mirror"
export AI_SPECS_TRUST_ROOT_FILE="$T/mirror/SHA256SUMS"
heal_env=()
if [ "${BREAK_SELF_HEAL:-0}" = 1 ]; then
    heal_env=(AI_SPECS_GO_BIN="$T/missing-ai-specs")
    say "== (d) BREAK_SELF_HEAL=1: the launcher cannot acquire a binary"
fi
say "== (d) first verb after the upgrade: ai-specs version"
env "${heal_env[@]}" "$AIBIN" version \
    >"$T/version-new.out" 2>"$T/version-new.err" ||
    fail "(d) ai-specs version after the upgrade failed" \
        "$T/version-new.err" "$T/version-new.out" "$T/upgrade.log"
[ "$(cat "$T/version-new.out")" = "$NEW_VERSION" ] ||
    fail "(d) stdout is not exactly the new version $NEW_VERSION" "$T/version-new.out"
grep -qE 'fetching|installed' "$T/version-new.err" ||
    fail "(d) the launcher did not report fetching/installing the native binary" \
        "$T/version-new.err"
[ -x "$AI_SPECS_HOME/dist/ai-specs" ] ||
    fail "(d) $AI_SPECS_HOME/dist/ai-specs is missing or not executable" \
        "$T/version-new.err"
pass "(d) the launcher self-healed: dist/ai-specs installed, version $NEW_VERSION"

say "== (e) ai-specs sync on the upgraded project"
"$AIBIN" sync </dev/null >"$T/sync-new.log" 2>&1 ||
    fail "(e) ai-specs sync after the upgrade failed" "$T/sync-new.log"
pass "(e) sync after the upgrade succeeded"

say "== (f) ai-specs doctor"
doctor_rc=0
"$AIBIN" doctor </dev/null >"$T/doctor.log" 2>&1 || doctor_rc=$?
error_lines="$(grep -E '^[[:space:]]*ERROR[[:space:]]' "$T/doctor.log" || true)"
error_count="$(printf '%s' "$error_lines" | grep -c . || true)"
if [ "$doctor_rc" -eq 0 ]; then
    pass "(f) doctor exited 0"
elif [ "$error_count" = 1 ] &&
    printf '%s' "$error_lines" | grep -q 'tracker-ledger.*failing open'; then
    pass "(f) doctor's only ERROR is the documented tracker-ledger fail-open"
else
    fail "(f) doctor exited $doctor_rc with unexpected errors" "$T/doctor.log"
fi

say "== (g) second verb: no fetch again"
"$AIBIN" version >"$T/version-again.out" 2>"$T/version-again.err" ||
    fail "(g) the second ai-specs version failed" \
        "$T/version-again.err" "$T/version-again.out"
[ "$(cat "$T/version-again.out")" = "$NEW_VERSION" ] ||
    fail "(g) the second run's stdout is not exactly $NEW_VERSION" "$T/version-again.out"
if grep -q 'fetching' "$T/version-again.err"; then
    fail "(g) the second verb fetched the binary again" "$T/version-again.err"
fi
pass "(g) the second verb reused dist/ai-specs without fetching"

say "PASS  upgrade $OLD_VERSION -> $NEW_VERSION survived"
exit 0
