---
name: worktree-candidate-cleanup
description: >
  Conservative post-merge sweep for stale native review candidate views under
  .git/gentle-ai/candidate-views. Trigger: after merged worktree cleanup, or
  when the candidate-view store accumulates orphaned owners whose process is
  dead and whose changes are already in the integration branch.
license: MIT
metadata:
  author: ai-specs
  version: "1.0"
  scope: [root]
  auto_invoke:
    - "After merged worktree cleanup to sweep stale native review candidate views"
---

# Worktree Candidate Cleanup

## Purpose

Native review materializes a frozen candidate tree as a detached Git worktree
under the shared git dir, plus a sibling controller owner marker:

```text
<common-dir>/gentle-ai/candidate-views/<uuid>/            # frozen read-only tree
<common-dir>/gentle-ai/candidate-views/<uuid>.owner.json  # owner marker
```

`<common-dir>` is `git rev-parse --git-common-dir` (the main worktree's `.git`).

These views and markers are **owned by the native review controller**. The
standard merged-worktree cleanup (`worktree-cleanup.sh` / `/worktree-clean`)
only reclaims `.worktrees/` and never touches this store. This skill defines the
extra, strictly conservative manual sweep for the residue the controller can no
longer own.

## Ownership boundary

- Candidate views and `.owner.json` markers are controller-owned state.
- Never delete the store, and never mutate review authority, transactions,
  locks, or receipts.
- The native controller reaps dead-owner views on its own startup. This skill is
  for the residue that survives, and it is **more** conservative than that
  sweep: it additionally requires the candidate's changes to be merged.
- Process one candidate at a time. If any proof cannot be produced, preserve the
  entry unchanged.

## Eligibility (all proofs must hold)

A candidate view is eligible for removal only when every proof below passes.
A missing, malformed, ambiguous, unreadable, or unreproducible proof means
**preserve**.

1. **Registered candidate worktree.** `<uuid>` appears exactly once in
   `git worktree list --porcelain`, that row has no `locked`/`prunable` field,
   and `<view>/.git` points to `<common-dir>/worktrees/<uuid>`.
2. **Valid owner marker.** `<uuid>.owner.json` is a regular non-symlink file
   whose JSON keys are exactly `commonDir,host,pid,root,token,uuid,version`,
   with `version: 1`, `uuid == basename(view)`, `root == view`,
   `commonDir == <common-dir>`, a UUID v4 `token`, a null-or-string `host`,
   and integer `pid > 0`. Unknown, legacy, or malformed markers are preserved
   untouched.
3. **Owner process proven dead.** The marker `host` is non-null and equals the
   local host identity, the PID is not this process, and `kill(pid, 0)` raises
   `ESRCH`. A live PID, an `EPERM`/permission error, a null host, or a host
   mismatch all mean **preserve**: pid death cannot be proven across hosts or
   reboots, and a recycled PID is indistinguishable from a live owner.
4. **No review reference.** No file under
   `<common-dir>/gentle-ai/review-transactions/**/*.json` mentions the
   candidate's tree hash, UUID, or path, and no `<uuid>.reaper-lock` exists.
   A contended or unreadable transaction store means **preserve**.
5. **Changes already in the integration branch.** The candidate is contained in
   the integration branch by ancestry, by tree identity, or by patch-id
   equivalence (`git cherry`, which covers squash/rebase merges), and the
   candidate tree is clean (`git status --porcelain` is empty). Never infer the
   integration branch: it is declared in recipe config, not guessed.

## Dry-run first (read-only)

Run from the main worktree, never from inside a candidate view or a worktree
being removed. This audit prints one verdict per candidate and removes nothing.

```bash
cd "$(git rev-parse --show-toplevel)"          # main worktree root
# `--git-common-dir` may print a relative `.git` from the main worktree; make it absolute.
common_dir="$(CDPATH= cd "$(git rev-parse --git-common-dir)" && pwd)" || {
  echo "unable to resolve git common dir" >&2; exit 1; }
views_dir="$common_dir/gentle-ai/candidate-views"
integration_branch="${INTEGRATION_BRANCH:-development}"
git rev-parse --verify --quiet "$integration_branch" >/dev/null || {
  echo "unknown integration branch: $integration_branch" >&2; exit 1; }

# Read-only. Prints one verdict line per candidate; removes nothing.
candidate_audit() {
python3 - "$common_dir" "$integration_branch" <<'PY'
import json, os, platform, re, subprocess, sys, glob

UUID4 = re.compile(r"^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$")

common_dir, base = sys.argv[1], sys.argv[2]
views_dir = os.path.join(common_dir, "gentle-ai", "candidate-views")

def git(*args):
    return subprocess.run(["git", *args], capture_output=True, text=True).stdout.strip()

def local_host():
    try:
        if platform.system() == "Darwin":
            boot = subprocess.run(["/usr/sbin/sysctl", "-n", "kern.bootsessionuuid"],
                                  capture_output=True, text=True, timeout=2).stdout.strip().lower()
            return f"darwin:{boot}" if boot else None
        if platform.system() == "Linux":
            with open("/proc/sys/kernel/random/boot_id") as fh:
                boot = fh.read().strip()
            return f"linux:{boot}:{os.readlink('/proc/self/ns/pid')}"
    except Exception:
        return None
    return None

def owner_dead(owner, host):
    if not host or owner.get("host") != host:
        return False
    pid = owner.get("pid")
    if not isinstance(pid, int) or pid <= 1 or pid == os.getpid():
        return False
    try:
        os.kill(pid, 0)
        return False
    except ProcessLookupError:
        return True
    except PermissionError:
        return False

def valid_marker(view):
    path = view + ".owner.json"
    try:
        if os.path.islink(path) or not os.path.isfile(path):
            return None
        owner = json.load(open(path))
    except Exception:
        return None
    if set(owner) != {"commonDir", "host", "pid", "root", "token", "uuid", "version"}:
        return None
    if owner["version"] != 1 or owner["uuid"] != os.path.basename(view):
        return None
    if owner["root"] != view or owner["commonDir"] != common_dir:
        return None
    if not isinstance(owner["pid"], int) or owner["pid"] <= 0:
        return None
    if not isinstance(owner["token"], str) or not UUID4.match(owner["token"]):
        return None
    if not (owner["host"] is None or isinstance(owner["host"], str)):
        return None
    return owner

def registered(view):
    rows = git("worktree", "list", "--porcelain", "-z").split("\0\0")
    hits = [r for r in rows if r.startswith("worktree ") and r[9:].split("\0")[0] == view]
    if len(hits) != 1:
        return False
    fields = hits[0].split("\0")
    if any(f == "locked" or f.startswith("locked ") or
           f == "prunable" or f.startswith("prunable ") for f in fields):
        return False
    try:
        pointer = open(os.path.join(view, ".git")).read()
    except Exception:
        return False
    return (pointer.startswith("gitdir: ") and
            os.path.dirname(pointer[8:].strip()) == os.path.join(common_dir, "worktrees"))

def referenced(tree_hash, view):
    pattern = os.path.join(common_dir, "gentle-ai", "review-transactions", "**", "*.json")
    for path in glob.glob(pattern, recursive=True):
        try:
            blob = open(path).read()
        except Exception:
            return True
        if (tree_hash and tree_hash in blob) or os.path.basename(view) in blob or view in blob:
            return True
    return False

def merged(view):
    head = git("-C", view, "rev-parse", "HEAD")
    if not head:
        return False
    if subprocess.run(["git", "merge-base", "--is-ancestor", head, base], capture_output=True).returncode == 0:
        return True
    if subprocess.run(["git", "diff", "--quiet", base, head], capture_output=True).returncode == 0:
        return True
    cherry = subprocess.run(["git", "cherry", base, head], capture_output=True, text=True).stdout
    lines = [line for line in cherry.splitlines() if line.strip()]
    return bool(lines) and all(line.startswith("-") for line in lines)

host = local_host()
if not os.path.isdir(views_dir):
    print(f"no candidate store at {views_dir}")
    sys.exit(0)

for entry in sorted(os.listdir(views_dir)):
    view = os.path.join(views_dir, entry)
    if entry.endswith(".owner.json") or not os.path.isdir(view) or os.path.islink(view):
        continue
    reasons = []
    if not registered(view):
        reasons.append("not a registered candidate worktree")
    owner = valid_marker(view)
    if owner is None:
        reasons.append("missing or malformed owner marker")
    elif not owner_dead(owner, host):
        reasons.append("owner process not proven dead")
    tree = git("-C", view, "rev-parse", "HEAD^{tree}")
    if referenced(tree, view):
        reasons.append("referenced by a review transaction")
    if os.path.exists(view + ".reaper-lock"):
        reasons.append("reaper lock present")
    if not merged(view):
        reasons.append(f"changes not provably merged into {base}")
    dirty = subprocess.run(["git", "--no-optional-locks", "-C", view, "status", "--porcelain"],
                           capture_output=True, text=True).stdout.strip()
    if dirty:
        reasons.append("candidate worktree is dirty")
    verdict = "ELIGIBLE" if not reasons else "PRESERVE"
    print(f"{entry}: {verdict} - {'; '.join(reasons) or 'all proofs hold'}")
PY
}

candidate_audit
```

## Removal (only after an ELIGIBLE verdict)

Run from the main worktree, in the same shell that defined `candidate_audit`
(see the dry-run section), one candidate at a time. `chmod -R u+w` mirrors the
controller's cleanup unlocking; the frozen views are materialized read-only.
The owner marker is fingerprinted and the read-only audit is re-run immediately
before removal so a candidate that changes or is re-owned mid-sweep is
preserved instead of reaped.

```bash
cd "$(git rev-parse --show-toplevel)"
# `--git-common-dir` may print a relative `.git` from the main worktree; make it absolute.
common_dir="$(CDPATH= cd "$(git rev-parse --git-common-dir)" && pwd)" || {
  echo "unable to resolve git common dir" >&2; exit 1; }
views_dir="$common_dir/gentle-ai/candidate-views"
integration_branch="${INTEGRATION_BRANCH:-development}"
uuid="<uuid>"                                  # exact ELIGIBLE candidate
view="$views_dir/$uuid"
marker="$view.owner.json"

sha256_of() { if command -v shasum >/dev/null 2>&1; then shasum -a 256 "$1" | awk '{print $1}'; else sha256sum "$1" | awk '{print $1}'; fi; }
type candidate_audit >/dev/null 2>&1 || { echo "define candidate_audit first (dry-run section)" >&2; exit 1; }
if [ ! -d "$view" ]; then echo "no candidate view at $view" >&2; exit 1; fi

before="$(sha256_of "$marker")"                # fingerprint the owner marker
if ! candidate_audit | grep -qx "$uuid: ELIGIBLE - all proofs hold"; then
  echo "eligibility not re-proven for $uuid; preserving" >&2; exit 1
fi

chmod -R u+w "$view"                           # unfreeze the read-only view
git worktree remove --force "$view"            # --force only after every proof

if [ -e "$view" ]; then echo "incomplete removal; preserving marker" >&2; exit 1; fi
if git worktree list --porcelain | grep -qF "worktree $view"; then
  echo "still registered; preserving marker" >&2; exit 1
fi
if [ "$(sha256_of "$marker")" != "$before" ]; then
  echo "owner marker changed during removal; preserving" >&2; exit 1
fi

rm -f "$marker"                                # remove ONLY the matching marker
```

If any step after `chmod` fails, stop and preserve the marker and any partial
view for conservative recovery; never fall back to recursive deletion.

## Always preserve

- missing, unknown, legacy, or malformed `.owner.json`
- a live, unprovable, cross-host, or cross-boot owner
- a present `<uuid>.reaper-lock` (contended)
- a candidate referenced by any review transaction
- a candidate whose changes are not provably merged
- non-candidate entries (other worktrees, the main worktree, `.worktrees/`,
  any non-UUID directory in the store)

## After the standard cleanup

The merged-worktree pass and this sweep are separate stores and separate steps.
Once the standard cleanup has reclaimed `.worktrees/`:

```bash
bash ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh \
  --dir .worktrees --base development --dry-run   # preview
bash ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh \
  --dir .worktrees --base development
```

Then invoke this skill explicitly to audit and, when every proof holds, reap
stale candidate views:

```text
Run the worktree-candidate-cleanup skill: audit the native review candidate
store and reap only views that pass every eligibility proof.
```

## Non-goals

- No automatic mutation of native review authority or transaction state.
- No recursive deletion and no staging or committing inside this workflow.
- No change to the Go cleanup implementation or to the native controller.
