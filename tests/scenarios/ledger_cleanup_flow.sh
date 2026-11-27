#!/usr/bin/env bash
# Scenario: the flow that actually runs records the change and cleans up,
# whatever merge strategy the human picked.
#
# Hermetic and backend-agnostic: plain bash, no network, and no writes outside
# the scratch directory it creates (mktemp honours the caller's temp dir). Any
# backend (this machine, CI, a Space) runs the same file with
# `bash tests/scenarios/ledger_cleanup_flow.sh`.
#
# What it proves (S8, S11), driven by a binary built from this tree, once per
# merge strategy in its own fresh scratch repository:
#   1. the binary serves `worktree-cleanup --help`. A capability guard, not a
#      base-versus-branch regression proof: development@75daa44e already serves
#      the verb, and it is the INSTALLED release 0.26.2 that does not, which is
#      why the generated hook's own probe (S10) exists;
#   2. `sync` publishes the per-project runtime snapshot and binds the tracker
#      witness (`state: bound`), the activation S3/S5 depend on;
#   3. the pre-tool-use gate at `apply-start` binds exactly ONE ledger row for
#      the change to the card its ODD plan's `## Tracker` names, and says so in
#      one actionable line;
#   4. the merge itself, in the shape the chosen strategy demands, with the
#      strategy's own precondition asserted so the case cannot silently degrade
#      into another one;
#   5. after that merge the cleanup verb proves it by ancestry (merge/ff) or by
#      patch/tree equivalence (rebase/squash) — the case that left six
#      worktrees alive — removes the worktree and the local branch, and closes
#      that ledger row;
#   6. a second cleanup run removes nothing further and does not double-close.
#   7. once, in its own scratch repository and with the generated post-merge
#      hook ACTIVE, a plain `git merge --no-ff` makes the hook clean up and
#      close the ledger row by itself, with no explicit cleanup call (S1).
#   8. once, in its own scratch repo with a bare `origin`, a provider-side squash
#      merge (branch deleted on the remote, landed locally by `git checkout -B`)
#      is cleaned by the generated post-checkout hook alone, with no manual call
#      (S12): the path post-merge never sees.
#   9. once, in its own scratch repo with a bare `origin`, a local squash merge
#      pushed to `origin` and landed by `git pull --ff-only` is cleaned by the
#      generated post-merge hook alone, with no manual call (S12).
#
# Env:
#   MERGE_STRATEGY  which strategy to run: merge | ff | rebase | squash. Unset
#                   runs all four (fresh scratch repo each) and prints one
#                   PASS/FAIL line per strategy plus a summary
#   SOURCE_REF      build from that committed ref instead of this working tree;
#                   the RED evidence runs it as SOURCE_REF=75daa44e (base
#                   development)
#   REPO            repository this script belongs to (default: this script's repo)
#   KEEP=1          keep the scratch directory instead of deleting it
#
# Exit codes: 0 all cases passed, 1 any case failed (the FAIL line names the
# strategy or check and the failed assertion, and prints the relevant log
# tails), 77 skipped (every case skipped, with a one-line reason on stderr).
#
# Prerequisites: git and go.
#
# Checks per strategy: (1) cleanup verb, (2) scratch repo, (3) sync snapshot +
# witness, (4) linked worktree with an ODD plan, (5) apply-start bind, (6) merge
# strategy, (7) dry-run + cleanup + ledger close, (8) second run is a no-op.
# Check (9) runs ONCE after them, in its own scratch repository, with the
# generated post-merge hook ACTIVE: the merge must trigger the cleanup by
# itself, with no explicit `worktree-cleanup` call. Checks (10) and (11) run
# once each in their own scratch repository with a bare `origin`: (10) a
# provider-side squash merge landed by `git checkout -B` must be cleaned by the
# generated post-checkout hook, (11) a local squash pushed to `origin` and
# landed by `git pull --ff-only` must be cleaned by post-merge. Neither calls
# `worktree-cleanup`.
set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="${REPO:-$(cd "$SCRIPT_DIR/../.." && pwd)}"
SOURCE_REF="${SOURCE_REF:-}"
KEEP="${KEEP:-0}"
MERGE_STRATEGY="${MERGE_STRATEGY:-}"

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

case "$MERGE_STRATEGY" in
    "") RUN="merge ff rebase squash" ;;
    merge | ff | rebase | squash) RUN="$MERGE_STRATEGY" ;;
    *)
        fail "MERGE_STRATEGY=$MERGE_STRATEGY is not one of merge, ff, rebase or squash" \
            >&2
        ;;
esac

for tool in git go; do
    command -v "$tool" >/dev/null 2>&1 || skip "missing prerequisite: $tool"
done
# Pin the toolchain before HOME/GIT_CONFIG_GLOBAL change: a shimmed `go` (mise,
# asdf) resolves its install through the caller's real environment.
GO_BIN="$(command -v go)"

if [ -n "${TMPDIR:-}" ]; then
    T="$(mktemp -d "$TMPDIR/ai-specs-ledger-flow.XXXXXX")" ||
        skip "could not create a scratch directory under $TMPDIR"
else
    T="$(mktemp -d -t ai-specs-ledger-flow)" ||
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

# (a) The tree the binary is built from: this working tree, or SOURCE_REF.
SRC="$REPO"
if [ -n "$SOURCE_REF" ]; then
    SRC="$T/src"
    mkdir -p "$SRC"
    git -C "$REPO" archive "$SOURCE_REF" | tar -x -C "$SRC" ||
        fail "(a) could not export $SOURCE_REF from $REPO"
fi

say "== (1) build the CLI from ${SOURCE_REF:-this working tree}"
BIN="$T/dist/ai-specs"
mkdir -p "$T/dist" "$T/home-base/dist"
(cd "$SRC" && "$GO_BIN" build -trimpath -buildvcs=false \
    -o "$BIN" ./cmd/ai-specs) >"$T/build.log" 2>&1 ||
    fail "(1) could not build ./cmd/ai-specs with $GO_BIN" "$T/build.log"
"$BIN" worktree-cleanup --help >"$T/help.out" 2>"$T/help.err" ||
    fail "(1) the built binary does not serve 'worktree-cleanup --help' (T1/S1)" \
        "$T/help.err" "$T/build.log"
grep -q 'worktree-cleanup' "$T/help.out" ||
    fail "(1) 'worktree-cleanup --help' did not print its usage" "$T/help.out"
pass "(1) the built binary serves the worktree-cleanup verb"

# (b) An isolated CLI home shaped like an install: the assets sync reads, and
# the binary where the generated post-merge hook resolves it. Prepared once,
# copied per strategy so no strategy can see another's cache.
for asset in catalog templates bundled-skills bundled-commands; do
    [ -d "$SRC/$asset" ] ||
        fail "(1) the source tree has no $asset directory to build the CLI home from"
    cp -R "$SRC/$asset" "$T/home-base/$asset"
done
[ -f "$SRC/VERSION" ] && cp "$SRC/VERSION" "$T/home-base/VERSION"

# run_strategy <merge|ff|rebase|squash>
# Runs inside a subshell with its own scratch repository, CLI home and config;
# logs go to $T/<strategy>/, so a failure never leaks into the next strategy.
# prep_case <scratch dir>: isolated CLI home and git config, so no case can see
# another's cache, branches or ledger. Sets $S.
prep_case() {
    S="$1"
    mkdir -p "$S" || skip "could not create the scratch directory $S"
    cp -R "$T/home-base" "$S/home" || fail "(2) could not copy the CLI home"
    cp "$BIN" "$S/home/dist/ai-specs" || fail "(2) could not copy the CLI binary"
    export AI_SPECS_HOME="$S/home"
    export GIT_CONFIG_GLOBAL="$S/gitconfig" GIT_CONFIG_NOSYSTEM=1
    git config --global user.email "scenario@example.invalid"
    git config --global user.name "scenario"
    git config --global init.defaultBranch main
    git config --global --add safe.directory '*'
    unset TRACKER_LEDGER_MODE TRACKER_CARD_GATE_MODE TRACKER_CARD_GATE_PATHS
}

# setup_flow: checks (2)-(5) — scratch repo, sync snapshot + witness, linked
# worktree with an ODD plan, and the apply-start bind. Sets $P, $W and $LEDGER;
# each caller adds its own merge and its own outcome assertions.
setup_flow() {
    say "== (2) scratch repository with worktree-flow and the tracker recipe"
    P="$S/proj"
    git init -q "$P" || fail "(2) git init failed"
    mkdir -p "$P/ai-specs" "$P/src"
    CARD_ID="47ab3c9e13e2d38ecd89a567"
    cat >"$P/ai-specs/ai-specs.toml" <<'EOF'
[project]
name = "ledger-flow-scratch"

[recipes.worktree-flow]
enabled = true

[recipes.worktree-flow.config]
worktrees_dir = ".worktrees"
integration_branch = "main"

[recipes.trello-mcp-workflow]
enabled = true

[recipes.trello-mcp-workflow.config]
board_id = "69ec097f13e2d38ecd89a557"
gate_mode = "warn"
EOF
    printf 'package main\n\nfunc main() {}\n' >"$P/src/app.go"
    (cd "$P" && git add -A && git commit -q -m "scratch base") ||
        fail "(2) could not commit the scratch repository"
    pass "(2) scratch repository committed on $(git -C "$P" symbolic-ref --short HEAD)"

    say "== (3) sync: runtime snapshot + tracker witness"
    (cd "$P" && "$BIN" sync) >"$S/sync.log" 2>&1 ||
        fail "(3) ai-specs sync failed" "$S/sync.log"
    set -- "$S/home"/cache/projects/*/runtime.toml
    [ -f "$1" ] ||
        fail "(3) sync wrote no per-project runtime snapshot under $S/home/cache/projects" \
            "$S/sync.log"
    grep -q '^integration_branch = "main"$' "$1" ||
        fail "(3) the runtime snapshot does not carry integration_branch = main" "$1"
    WITNESS="$P/.git/ai-specs/ledger/witness.json"
    [ -f "$WITNESS" ] ||
        fail "(3) sync wrote no tracker witness at $WITNESS (the ledger stays dormant)" \
            "$S/sync.log"
    grep -q '"state": "bound"' "$WITNESS" ||
        fail "(3) the tracker witness is not bound" "$WITNESS"
    pass "(3) sync published the snapshot and a bound tracker witness"
    # Commit what sync wrote, so a later cleanliness assertion is about cleanup.
    (cd "$P" && git add -A && git commit -q -m "sync") ||
        fail "(3) could not commit the sync output" "$S/sync.log"

    say "== (4) linked worktree with a production write and an ODD plan"
    (cd "$P" && git worktree add -q .worktrees/feat -b change/feat) ||
        fail "(4) git worktree add failed"
    W="$P/.worktrees/feat"
    mkdir -p "$W/odd/tasks" || fail "(4) could not create $W/odd/tasks"
    cat >"$W/odd/tasks/feat.md" <<EOF
# feat (ODD)

## Tracker

- **card_id**: \`$CARD_ID\`
- **url**: https://trello.com/c/47ab3c9e
- **list**: En curso
EOF
    printf 'package main\n\nfunc feat() {}\n' >"$W/src/app.go"
    (cd "$W" && git add -A && git commit -q -m "feat") ||
        fail "(4) could not commit the feature worktree"
    pass "(4) linked worktree .worktrees/feat created on change/feat"

    say "== (5) apply-start: the pre-tool-use gate binds the ledger row"
    printf '%s' "{\"tool_name\":\"Write\",\"tool_input\":{\"file_path\":\"$W/src/app.go\"}}" |
        "$BIN" tracker-card-gate >"$S/apply.out" 2>"$S/apply.err"
    rc=$?
    [ "$rc" -eq 0 ] ||
        fail "(5) tracker-card-gate exited $rc; the Tracker gate is advisory and must exit 0" \
            "$S/apply.err" "$S/apply.out"
    # The store file is an implementation detail, never a contract (S14): read
    # whichever file the binary under test wrote, so this scenario holds for the
    # split layout and for an older binary that still writes the pre-split one. A
    # CLI read verb (`changes status`) replaces this seam when it lands (T7).
    LEDGER="$P/.git/ai-specs/ledgers/tracker.json"
    [ -f "$LEDGER" ] || LEDGER="$P/.git/ai-specs/ledger/state.json"
    [ -f "$LEDGER" ] ||
        fail "(5) apply-start wrote no ledger store, so the change was never registered" \
            "$S/apply.err" "$S/apply.out"
    [ "$(grep -c . "$S/apply.err")" = 1 ] ||
        fail "(5) apply-start did not report exactly one actionable line" "$S/apply.err"
    grep -q "bound card $CARD_ID at apply-start" "$S/apply.err" ||
        fail "(5) apply-start did not report binding card $CARD_ID" "$S/apply.err"
    grep -q "\"item_id\": \"$CARD_ID\"" "$LEDGER" ||
        fail "(5) the ledger row is not bound to card $CARD_ID" "$LEDGER"
    grep -q '"branch": "change/feat"' "$LEDGER" ||
        fail "(5) the ledger row is not keyed to branch change/feat" "$LEDGER"
    grep -q '"status": "open"' "$LEDGER" ||
        fail "(5) the ledger row is not open" "$LEDGER"
    [ "$(grep -c '"kind": "open"' "$LEDGER")" = 1 ] ||
        fail "(5) apply-start did not open exactly one ledger item" "$LEDGER"
    pass "(5) apply-start bound one open ledger row to $CARD_ID"
}

# run_strategy <name>: checks (2)-(8) for one merge strategy.
run_strategy() {
    local strategy="$1"
    prep_case "$T/$strategy"
    setup_flow

    # (6) The merge, in the shape this strategy demands. Each strategy asserts
    # its OWN precondition first, so a case can never silently degrade into
    # another: merge/ff leave the branch tip an ancestor of main; rebase/squash
    # do not, and rebase must additionally show rewritten commits.
    # Sync installs the managed post-merge hook, the real trigger (S1). On a
    # merge commit it cleans up immediately, and this scenario has to drive the
    # cleanup verb itself to observe its dry-run and its idempotency, so hooks
    # are disabled for the merge commands only.
    g() { (cd "$P" && git -c core.hooksPath=/dev/null "$@"); }
    local orig_tip=""
    say "== (6) simulated merge: $strategy"
    case "$strategy" in
    merge)
        g merge --no-ff -q -m "merge change/feat" change/feat \
            >"$S/merge.log" 2>&1 ||
            fail "(6) git merge --no-ff failed" "$S/merge.log"
        (cd "$P" && git merge-base --is-ancestor change/feat main) ||
            fail "(6-merge) change/feat is not an ancestor of main, so this is not a --no-ff merge commit" \
                "$S/merge.log"
        ;;
    ff)
        g merge --ff-only -q change/feat >"$S/merge.log" 2>&1 ||
            fail "(6) git merge --ff-only failed" "$S/merge.log"
        (cd "$P" && git merge-base --is-ancestor change/feat main) ||
            fail "(6-ff) change/feat is not an ancestor of main, so this was not a fast-forward" \
                "$S/merge.log"
        [ "$(cd "$P" && git rev-parse change/feat)" = "$(cd "$P" && git rev-parse main)" ] ||
            fail "(6-ff) the branch tip is not the integration tip, so this was not a fast-forward" \
                "$S/merge.log"
        ;;
    rebase)
        # main moves on first, otherwise "rebase onto main" would be a no-op and
        # no commit would actually be rewritten.
        orig_tip="$(cd "$P" && git rev-parse change/feat)"
        printf 'package main\n\nfunc other() {}\n' >"$P/src/other.go"
        # Only the new file: `git add -A` here would record the live linked
        # worktree itself as an embedded-repo gitlink.
        (g add src/other.go && g commit -q -m "main moves ahead") \
            >"$S/merge.log" 2>&1 ||
            fail "(6) could not advance main before the rebase" "$S/merge.log"
        # Rebase a copy of the branch onto main and fast-forward main onto it;
        # change/feat keeps the ORIGINAL commits, as a contributor's local
        # branch does when only the rebased copy was merged.
        (g checkout -q -b rebase-work change/feat &&
            g rebase -q main && g checkout -q main &&
            g merge --ff-only -q rebase-work && g branch -d -q rebase-work) \
            >>"$S/merge.log" 2>&1 ||
            fail "(6) git rebase + merge failed" "$S/merge.log"
        if (cd "$P" && git merge-base --is-ancestor change/feat main); then
            fail "(6-rebase) change/feat is an ancestor of main; the commits were not rewritten" \
                "$S/merge.log"
        fi
        (cd "$P" && git merge-base --is-ancestor "$orig_tip" main) &&
            fail "(6-rebase) the original tip $orig_tip is on main; nothing was rebased" \
                "$S/merge.log"
        [ -z "$(cd "$P" && git rev-list main | grep -qx "$orig_tip" && echo present)" ] ||
            fail "(6-rebase) the original tip $orig_tip is still reachable from main" \
                "$S/merge.log"
        (cd "$P" && git log --format=%s main | grep -qx 'feat') ||
            fail "(6-rebase) the rebased 'feat' commit is not on main" "$S/merge.log"
        ;;
    squash)
        g merge --squash change/feat >"$S/merge.log" 2>&1 ||
            fail "(6) git merge --squash failed" "$S/merge.log"
        g commit -q -m "squash change/feat" >>"$S/merge-commit.log" 2>&1 ||
            fail "(6) committing the squashed merge failed" "$S/merge-commit.log"
        if (cd "$P" && git merge-base --is-ancestor change/feat main); then
            fail "(6-squash) change/feat is an ancestor of main; this is not the squash case" \
                "$S/merge.log"
        fi
        (cd "$P" && git log --format=%s main | grep -qx 'squash change/feat') ||
            fail "(6-squash) main carries no squash commit" "$S/merge.log"
        ;;
    esac
    [ -d "$W" ] ||
        fail "(6) the worktree was already gone before the explicit cleanup ran" \
            "$S/merge.log"
    pass "(6) $strategy merge detected: branch tip $(cd "$P" && git rev-parse --short change/feat), main $(cd "$P" && git rev-parse --short main)"

    say "== (7) worktree-cleanup: dry-run, then the real pass"
    (cd "$P" && "$BIN" worktree-cleanup --dry-run) >"$S/dry.log" 2>&1 ||
        fail "(7) worktree-cleanup --dry-run failed" "$S/dry.log"
    grep -q '^would remove feat$' "$S/dry.log" ||
        fail "(7) the dry-run did not report the worktree as removable" "$S/dry.log"
    [ -d "$W" ] || fail "(7) the dry-run removed the worktree" "$S/dry.log"
    grep -q '"status": "open"' "$LEDGER" ||
        fail "(7) the dry-run mutated the ledger row" "$LEDGER"
    (cd "$P" && "$BIN" worktree-cleanup) >"$S/cleanup.log" 2>&1 ||
        fail "(7) worktree-cleanup failed" "$S/cleanup.log"
    [ -e "$W" ] && fail "(7) the worktree .worktrees/feat still exists" "$S/cleanup.log"
    [ -n "$(cd "$P" && git branch --list change/feat)" ] &&
        fail "(7) the local branch change/feat still exists" "$S/cleanup.log"
    grep -q '"status": "closed"' "$LEDGER" ||
        fail "(7) the ledger row was not closed" "$LEDGER" "$S/cleanup.log"
    grep -q '"kind": "close"' "$LEDGER" ||
        fail "(7) the ledger row carries no close decision" "$LEDGER"
    grep -q '"checkpoint": "archive-close"' "$LEDGER" ||
        fail "(7) the close decision is not recorded at archive-close" "$LEDGER"
    grep -q '"branch": "change/feat"' "$LEDGER" ||
        fail "(7) the closed row is no longer keyed to change/feat" "$LEDGER"
    (cd "$P" && git show main:src/app.go | grep -q 'func feat') ||
        fail "(7) main does not contain the change's content" "$S/cleanup.log"
    [ -z "$(cd "$P" && git status --porcelain)" ] ||
        fail "(7) the repository is dirty after cleanup" "$S/cleanup.log"
    pass "(7) cleanup removed the worktree and branch and closed the ledger row"

    say "== (8) a second cleanup removes nothing and does not double-close"
    local closes_before
    closes_before="$(grep -c '"kind": "close"' "$LEDGER")"
    (cd "$P" && "$BIN" worktree-cleanup) >"$S/cleanup2.log" 2>"$S/cleanup2.err"
    rc=$?
    [ "$rc" -eq 0 ] ||
        fail "(8) the second worktree-cleanup exited $rc instead of 0" \
            "$S/cleanup2.err" "$S/cleanup2.log"
    if grep -q 'closed ledger item' "$S/cleanup2.log"; then
        fail "(8) the second cleanup closed a ledger item again" "$S/cleanup2.log"
    fi
    [ "$(grep -c '"kind": "close"' "$LEDGER")" = "$closes_before" ] ||
        fail "(8) the second cleanup appended another close decision" "$LEDGER"
    [ -z "$(cd "$P" && git status --porcelain)" ] ||
        fail "(8) the second cleanup left the repository dirty" "$S/cleanup2.log"
    pass "(8) the second cleanup was a no-op"
}

# Check (9), run ONCE: the automatic path. Sync installs the generated
# post-merge hook, so here it stays ACTIVE — a plain `git merge --no-ff` must
# clean up and close the ledger row by itself, with no explicit cleanup call.
run_hook_flow() {
    say "== (9) merge with the generated post-merge hook active"
    prep_case "$T/hook"
    setup_flow
    # No core.hooksPath override: the hook must fire on its own.
    (cd "$P" && git merge --no-ff -m "merge change/feat" change/feat) \
        >"$S/hook.log" 2>&1
    rc=$?
    [ "$rc" -eq 0 ] ||
        fail "(9) git merge --no-ff exited $rc" \
            "$S/hook.log" "$S/apply.err" "$S/apply.out"
    # The hook's own output is the proof that the trigger fired at all: without
    # it the assertions below could pass vacuously.
    if ! grep -q 'closed ledger item for change/feat' "$S/hook.log" ||
        ! grep -q 'removed feat' "$S/hook.log"; then
        fail "(9) the automatic post-merge trigger did not fire: the hook printed no cleanup lines (no 'closed ledger item for change/feat' and 'removed feat')" \
            "$S/hook.log"
    fi
    [ -e "$W" ] &&
        fail "(9) the hook did not remove the worktree .worktrees/feat" "$S/hook.log"
    [ -n "$(cd "$P" && git branch --list change/feat)" ] &&
        fail "(9) the hook did not remove the local branch change/feat" "$S/hook.log"
    grep -q '"status": "closed"' "$LEDGER" ||
        fail "(9) the hook did not close the ledger row" "$LEDGER" "$S/hook.log"
    grep -q '"kind": "close"' "$LEDGER" ||
        fail "(9) the closed ledger row carries no close decision" "$LEDGER"
    grep -q '"checkpoint": "archive-close"' "$LEDGER" ||
        fail "(9) the close decision is not recorded at archive-close" "$LEDGER"
    [ "$(grep -c '"kind": "close"' "$LEDGER")" = 1 ] ||
        fail "(9) the hook recorded something other than exactly one close decision" "$LEDGER"
    (cd "$P" && git rev-parse -q --verify main^2 >/dev/null) ||
        fail "(9) main's tip is not a --no-ff merge commit" "$S/hook.log"
    (cd "$P" && git show main:src/app.go | grep -q 'func feat') ||
        fail "(9) main does not contain the change's content" "$S/hook.log"
    [ -z "$(cd "$P" && git status --porcelain)" ] ||
        fail "(9) the repository is dirty after the hook ran" "$S/hook.log"
    pass "(9) the post-merge hook cleaned up and closed the ledger row with no manual step"
}

# assert_hook_cleanup <label> <hook log>: the hook's OWN cleanup lines prove it
# fired (a silent hook cannot pass vacuously); then the outcomes (10)/(11)
# require the same closed row and removed worktree as the explicit cases.
assert_hook_cleanup() {
    local label="$1" log="$2"
    if ! grep -q 'closed ledger item for change/feat' "$log" ||
        ! grep -q 'removed feat' "$log"; then
        fail "$label did not fire: the hook printed no cleanup lines (no 'closed ledger item for change/feat' and 'removed feat')" \
            "$log"
    fi
    [ -e "$W" ] && fail "$label did not remove the worktree .worktrees/feat" "$log"
    [ -n "$(cd "$P" && git branch --list change/feat)" ] &&
        fail "$label did not remove the local branch change/feat" "$log"
    grep -q '"status": "closed"' "$LEDGER" ||
        fail "$label did not close the ledger row" "$LEDGER" "$log"
    grep -q '"kind": "close"' "$LEDGER" ||
        fail "$label closed row carries no close decision" "$LEDGER"
    grep -q '"checkpoint": "archive-close"' "$LEDGER" ||
        fail "$label close decision is not recorded at archive-close" "$LEDGER"
    [ "$(grep -c '"kind": "close"' "$LEDGER")" = 1 ] ||
        fail "$label recorded something other than exactly one close decision" "$LEDGER"
    (cd "$P" && git show main:src/app.go | grep -q 'func feat') ||
        fail "$label main does not contain the change's content" "$log"
    [ -z "$(cd "$P" && git status --porcelain)" ] ||
        fail "$label the repository is dirty after the hook ran" "$log"
}

# Check (10): a provider-side squash merge. The bare `origin` receives the
# feature branch, a second clone does what GitHub's squash merge plus "delete
# branch" leaves behind, and the first repo lands it with a CHECKOUT (not a
# merge, not a pull). Only the generated post-checkout hook can clean that.
run_provider_squash_flow() {
    prep_case "$T/provider"
    setup_flow
    say "== (10) simulated provider-side squash merge, landed by a checkout"
    local origin="$S/origin.git" clone="$S/clone"
    git init -q --bare "$origin" || fail "(10) could not create the bare origin"
    (cd "$P" && git remote add origin "$origin" &&
        git push -q origin main && git push -q origin change/feat) \
        >"$S/provider-push.log" 2>&1 ||
        fail "(10) could not push main and change/feat to origin" "$S/provider-push.log"
    git clone -q "$origin" "$clone" >"$S/provider-clone.log" 2>&1 ||
        fail "(10) could not clone origin" "$S/provider-clone.log"
    (cd "$clone" && git fetch -q origin change/feat &&
        git merge --squash FETCH_HEAD && git commit -q -m "squash change/feat" &&
        git push -q origin main && git push -q origin --delete change/feat) \
        >"$S/provider-merge.log" 2>&1 ||
        fail "(10) the provider-side squash merge failed" "$S/provider-merge.log"
    [ -z "$(cd "$P" && git ls-remote --heads origin change/feat)" ] ||
        fail "(10) the remote feature branch still exists; the provider deletes it" \
            "$S/provider-merge.log"
    (cd "$P" && git fetch -q origin) >"$S/provider-fetch.log" 2>&1 ||
        fail "(10) git fetch origin failed" "$S/provider-fetch.log"
    # No core.hooksPath override: the checkout must trigger the cleanup itself.
    (cd "$P" && git checkout -B main origin/main) >"$S/hook10.log" 2>&1
    rc=$?
    [ "$rc" -eq 0 ] ||
        fail "(10) git checkout -B main origin/main exited $rc" "$S/hook10.log"
    assert_hook_cleanup "(10) post-checkout" "$S/hook10.log"
    pass "(10) the post-checkout hook cleaned up after a provider-side squash merge with no manual step"
}

# Check (11): a local squash merge (made in a throwaway linked worktree) pushed
# to `origin` while this checkout's main stays behind, so the following
# `git pull --ff-only` is a real fast-forward and fires post-merge. No manual
# `worktree-cleanup`.
run_pull_ff_flow() {
    prep_case "$T/pull"
    setup_flow
    say "== (11) local squash merge, landed by a fast-forward pull"
    local origin="$S/origin.git"
    git init -q --bare "$origin" || fail "(11) could not create the bare origin"
    (cd "$P" && git remote add origin "$origin" && git push -q origin main) \
        >"$S/pull-push.log" 2>&1 ||
        fail "(11) could not push the pre-merge main to origin" "$S/pull-push.log"
    # Squash locally in a throwaway linked worktree and push from there: pushing
    # main itself, or checking main out after the push, would advance the primary
    # checkout before the pull (post-checkout already syncs the base branch), so
    # the pull would be a no-op and post-merge would never fire.
    (cd "$P" && git worktree add -q .worktrees/squash -b squash-work main &&
        (cd .worktrees/squash && git merge --squash change/feat &&
            git commit -q -m "squash change/feat" &&
            git push -q origin squash-work:main) &&
        git worktree remove .worktrees/squash && git branch -D -q squash-work) \
        >"$S/pull-squash.log" 2>&1 ||
        fail "(11) the local squash merge failed" "$S/pull-squash.log"
    # The primary checkout's main is still behind origin/main, so this pull is a
    # real fast-forward and fires post-merge.
    (cd "$P" && git pull --ff-only origin main) >"$S/hook11.log" 2>&1
    rc=$?
    [ "$rc" -eq 0 ] ||
        fail "(11) git pull --ff-only exited $rc" "$S/hook11.log"
    assert_hook_cleanup "(11) post-merge" "$S/hook11.log"
    pass "(11) the post-merge hook cleaned up after a fast-forward pull with no manual step"
}

SUMMARY=""
ran=0
skipped=0
failed=0

# run_case <label> <log> <runner...>: run one case in a subshell, echo its log,
# record PASS/FAIL/SKIP and, on failure, the failed assertion.
run_case() {
    local label="$1" log="$2" why
    shift 2
    ( "$@" ) >"$log" 2>&1
    rc=$?
    cat "$log"
    if [ "$rc" -eq 0 ]; then
        ran=$((ran + 1))
        SUMMARY="$SUMMARY
PASS  $label"
    elif [ "$rc" -eq 77 ]; then
        skipped=$((skipped + 1))
        SUMMARY="$SUMMARY
SKIP  $label"
    else
        failed=$((failed + 1))
        why="$(grep -m1 '^FAIL  ' "$log" || printf 'exited %s' "$rc")"
        printf -- '--- %s (tail) ---\n' "$log"
        tail -n 20 "$log"
        printf 'FAIL  %s: %s\n' "$label" "${why#FAIL  }"
        SUMMARY="$SUMMARY
FAIL  $label: ${why#FAIL  }"
    fi
}

for strategy in $RUN; do
    say ""
    say "===== MERGE_STRATEGY=$strategy"
    run_case "strategy $strategy" "$T/$strategy.log" run_strategy "$strategy"
done

say ""
say "===== (9) automatic post-merge hook (once, hook enabled)"
run_case "check (9) automatic post-merge hook" "$T/hook.log" run_hook_flow

say ""
say "===== (10) provider-side squash merge, cleaned by post-checkout (once)"
run_case "check (10) provider-side squash merge via post-checkout" \
    "$T/provider.log" run_provider_squash_flow

say ""
say "===== (11) local squash then fast-forward pull, cleaned by post-merge (once)"
run_case "check (11) local squash then pull via post-merge" \
    "$T/pull.log" run_pull_ff_flow

say ""
say "== summary"
printf '%s\n' "${SUMMARY#?}"
total=$((ran + skipped + failed))
if [ "$failed" -gt 0 ]; then
    printf 'FAIL  %s/%s flow cases passed: 4 merge strategies + the automatic post-merge hook + provider-squash/post-checkout + local-squash/pull -> apply, merge, one ledger row bound and closed, worktree removed\n' \
        "$ran" "$total"
    exit 1
fi
if [ "$ran" -eq 0 ]; then
    printf 'SKIP  all %s flow cases skipped\n' "$total" >&2
    exit 77
fi
printf 'PASS  %s/%s flow cases: 4 merge strategies + the automatic post-merge hook + provider-squash/post-checkout + local-squash/pull -> apply, merge, one ledger row bound and closed, worktree removed\n' \
    "$ran" "$total"
exit 0
