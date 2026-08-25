#!/usr/bin/env bash
# worktree-gate.sh — pre-tool-use guard distributed by the worktree-flow recipe.
#
# Enforces the worktree-flow discipline: "exploration ends at the first write;
# create a dedicated worktree before writing." Blocks Edit/Write/MultiEdit/
# NotebookEdit calls that target the canonical MAIN worktree while it is on a
# protected branch (default: main, development). Also best-effort blocks shell
# commands (Bash/Shell/Execute/Terminal) that high-confidence write into that
# same protected main worktree. Edits inside a linked worktree (under
# .worktrees/) are always allowed.
#
# Dual-input contract (one script, every harness):
#   PATH mode stdin = JSON { "event", "tool_name",
#     "tool_input": {file_path|notebook_path}, "cwd" }
#   SHELL mode stdin = JSON with tool_input.command (or script/cmd) OR Cursor
#     native top-level { "command", "cwd", … }
#   exit 0 → allow.   exit 2 → block (stderr is surfaced to the agent).
# Fail-open: any parse/lookup error or ambiguous heuristic allows the action
# (a buggy guard must never wedge all editing). Override protected branches via
# WORKTREE_GATE_PROTECTED. gate_mode off disables both path and shell gating.
# gate_scope is stamped by sync and may be overridden per invocation.
#
# Known internal harness URIs (xd://, skill://, artifact://, ...) are tool
# interfaces, not Git destinations: they bypass filesystem classification in
# PATH mode only, and never when they mask a traversal into the repo. Relative
# candidates resolve against the event cwd when it is an absolute existing
# directory; the process $PWD is fallback only.

stamped_gate_mode="__WORKTREE_GATE_MODE__"
stamped_gate_scope="__WORKTREE_GATE_SCOPE__"
stamped_repo_topology="__WORKTREE_REPO_TOPOLOGY__"
protected="${WORKTREE_GATE_PROTECTED:-main development}"

# Resolve gate mode: env override beats stamped sync value; invalid values warn and fall back.
_resolve_gate_mode() {
  local candidate="${WORKTREE_GATE_MODE:-$stamped_gate_mode}"
  case "$candidate" in always|ask|off) echo "$candidate" ; return ;;
  esac
  if [ -n "${WORKTREE_GATE_MODE:-}" ]; then
    echo "worktree-gate: ignoring invalid WORKTREE_GATE_MODE='${WORKTREE_GATE_MODE}'; falling back to stamped mode." >&2
  elif [ "$stamped_gate_mode" != always ] && [ "$stamped_gate_mode" != ask ] && [ "$stamped_gate_mode" != off ]; then
    echo "worktree-gate: invalid stamped gate_mode='${stamped_gate_mode}'; falling back to always." >&2
  fi
  case "$stamped_gate_mode" in always|ask|off) echo "$stamped_gate_mode" ;;
  *) echo always ;;
  esac
}
gate_mode="$(_resolve_gate_mode)"

_resolve_gate_scope() {
  local override="${WORKTREE_GATE_SCOPE:-}"
  if [ -n "$override" ]; then
    case "$override" in
      auto|superrepo|subrepo) echo "$override"; return ;;
      *) echo "worktree-gate: invalid WORKTREE_GATE_SCOPE='$override'; falling back to stamped scope." >&2 ;;
    esac
  fi
  case "$stamped_gate_scope" in
    auto|superrepo|subrepo) echo "$stamped_gate_scope" ;;
    *) echo "worktree-gate: missing or invalid stamped gate_scope='$stamped_gate_scope'; falling back to auto." >&2; echo auto ;;
  esac
}

# off → disable the gate entirely, before scope/topology evaluation.
[ "$gate_mode" = off ] && exit 0
gate_scope="$(_resolve_gate_scope)"
_resolve_repo_topology() {
  case "$stamped_repo_topology" in
    auto|standalone|monorepo-apps|monorepo-submodules) echo "$stamped_repo_topology" ;;
    *) echo "worktree-gate: missing or invalid stamped repo_topology='$stamped_repo_topology'; falling back to auto." >&2; echo auto ;;
  esac
}

repo_topology="$(_resolve_repo_topology)"

input="$(cat)"

# Extract mode + tool + candidate write paths. python3 is a project prerequisite.
# Protocol on stdout:
#   line 1: <mode>\t<tool_name>   mode ∈ {path, shell}
#   line 2..N: one candidate path per line
# NOTE: the embedded script is fed via a quote-delimited heredoc (not `-c
# '...'`) because the regexes below contain literal single quotes; bash
# cannot escape a `'` inside a single-quoted `-c` string, which previously
# broke the parser. The event JSON is passed as argv[1] (not stdin) since the
# heredoc itself occupies stdin.
parsed="$(python3 - "$input" <<'PYEOF' 2>/dev/null
import json, re, shlex, sys

def scrub(path):
    if path is None:
        return None
    p = path.strip()
    if not p or p in (".", "-"):
        return None
    if p.startswith("&"):
        return None
    if p in ("/dev/null", "/dev/stdout", "/dev/stderr") or p.startswith("/dev/fd/"):
        return None
    return p

def dedupe(paths):
    out, seen = [], set()
    for p in paths:
        s = scrub(p)
        if s is None or s in seen:
            continue
        seen.add(s)
        out.append(s)
    return out

WRAPPERS = {
    "sudo", "env", "nice", "time", "nohup", "xargs", "command",
}

def command_word(seg):
    i = 0
    while i < len(seg):
        t = seg[i]
        if "=" in t and not t.startswith("=") and re.match(r"^[A-Za-z_][A-Za-z0-9_]*=", t):
            i += 1
            continue
        if t in WRAPPERS:
            i += 1
            continue
        return i, t
    return None, None

def pass1(cmd):
    try:
        tokens = shlex.split(cmd, posix=True)
    except ValueError:
        return []
    seps = {"|", "||", "&&", ";"}
    segments, cur = [], []
    for t in tokens:
        if t in seps:
            if cur:
                segments.append(cur)
            cur = []
        else:
            cur.append(t)
    if cur:
        segments.append(cur)

    found = []
    for seg in segments:
        if not seg:
            continue
        # Redirection: standalone > / >> or glued \d*>>?target
        i = 0
        while i < len(seg):
            t = seg[i]
            if t in (">", ">>"):
                if i + 1 < len(seg):
                    found.append(seg[i + 1])
                i += 2
                continue
            m = re.match(r"^(\d*)(>>?)(.*)$", t)
            if m and m.group(2) in (">", ">>"):
                rest = m.group(3)
                if rest.startswith("&"):
                    i += 1
                    continue
                if rest:
                    found.append(rest)
                elif i + 1 < len(seg):
                    found.append(seg[i + 1])
                    i += 2
                    continue
            i += 1

        idx, cw = command_word(seg)
        if cw is None:
            continue
        body = seg[idx:]
        # tee (also when it is the command after a pipe — already its own segment)
        if cw == "tee":
            for t in body[1:]:
                if t.startswith("-"):
                    continue
                found.append(t)
            continue
        if cw in ("sed", "perl"):
            has_i = any(t == "-i" or t.startswith("-i") for t in body[1:])
            if has_i:
                nonflags = [t for t in body[1:] if not t.startswith("-")]
                if nonflags:
                    found.append(nonflags[-1])
            continue
        if cw in ("cp", "mv"):
            nonflags = [t for t in body[1:] if not t.startswith("-")]
            if nonflags:
                found.append(nonflags[-1])
            continue
    return found

def pass2(cmd):
    found = []
    # Python open(path, mode) with w/a/x in mode
    for m in re.finditer(
        r"""open\(\s*(["'])(?P<p>.+?)\1\s*,\s*(["'])(?P<mode>[^"']*)\3""",
        cmd,
    ):
        mode = m.group("mode")
        if any(c in mode for c in "wax"):
            found.append(m.group("p"))
    # Path(...).write_text / write_bytes
    for m in re.finditer(
        r"""Path\(\s*(["'])(?P<p>.+?)\1\s*\)\s*\.write_(?:text|bytes)\(""",
        cmd,
    ):
        found.append(m.group("p"))
    # Node fs writers
    for m in re.finditer(
        r"""(?:fs\.)?(?:writeFileSync|appendFileSync|writeFile|appendFile|createWriteStream)\(\s*(["'])(?P<p>.+?)\1""",
        cmd,
    ):
        found.append(m.group("p"))
    # Ruby File.write("p", ...) always a write
    for m in re.finditer(
        r"""File\.write\(\s*(["'])(?P<p>.+?)\1""",
        cmd,
    ):
        found.append(m.group("p"))
    # Ruby File.open("p", "w...")
    for m in re.finditer(
        r"""File\.open\(\s*(["'])(?P<p>.+?)\1\s*,\s*(["'])(?P<mode>[^"']*)\3""",
        cmd,
    ):
        mode = m.group("mode")
        if any(c in mode for c in "wax"):
            found.append(m.group("p"))
    return found

try:
    d = json.loads(sys.argv[1])
except Exception:
    sys.exit(0)

if not isinstance(d, dict):
    sys.exit(0)

ti = d.get("tool_input") or {}
if not isinstance(ti, dict):
    ti = {}
fp = ti.get("file_path") or ti.get("notebook_path") or ""
tool_name = (d.get("tool_name") or "") if isinstance(d.get("tool_name"), str) else ""

if isinstance(fp, str) and fp.strip():
    print("path\t" + tool_name)
    print(fp.strip())
    sys.exit(0)

cmd = ""
for key_src in (
    ("ti", "command"),
    ("ti", "script"),
    ("ti", "cmd"),
    ("top", "command"),
    ("top", "script"),
):
    if key_src[0] == "ti":
        val = ti.get(key_src[1])
    else:
        val = d.get(key_src[1])
    if isinstance(val, str) and val.strip():
        cmd = val
        break

if not cmd:
    sys.exit(0)

cands = dedupe(pass1(cmd) + pass2(cmd))
if not cands:
    sys.exit(0)

print("shell\t" + (tool_name or "Bash"))
for c in cands:
    print(c)
PYEOF
)" || exit 0

[ -n "$parsed" ] || exit 0

# cwd from event (top-level), the base for relative candidates; only an
# absolute existing directory is usable. The hook process $PWD is fallback
# when the event cwd is absent, relative, or nonexistent.
event_cwd="$(python3 - "$input" <<'PYEOF' 2>/dev/null
import json, os, sys
try:
    d = json.loads(sys.argv[1])
except Exception:
    print("")
    sys.exit(0)
if not isinstance(d, dict):
    print("")
    sys.exit(0)
c = d.get("cwd")
# Usable event cwd: non-empty, absolute, and an existing directory. A
# relative or nonexistent cwd must not widen the resolution base, so it
# falls back to the hook process $PWD.
if isinstance(c, str) and c.strip() and os.path.isabs(c.strip()) and os.path.isdir(c.strip()):
    print(c.strip())
else:
    print("")
PYEOF
)" || event_cwd=""
cwd="${event_cwd:-$PWD}"

# Read protocol
mode=""
tool_name=""
candidates=()
while IFS= read -r line || [ -n "$line" ]; do
  if [ -z "$mode" ]; then
    mode="${line%%$'\t'*}"
    tool_name="${line#*$'\t'}"
    continue
  fi
  [ -n "$line" ] && candidates+=("$line")
done <<< "$parsed"

[ "${#candidates[@]}" -gt 0 ] || exit 0

# Shared topology-aware decision for PATH and SHELL candidates. The helper is
# deliberately self-contained: it uses only Git facts and canonical paths,
# never the consumer manifest or project Python modules.
resolve_and_check() {
  local candidate="$1"
  local abs decision rest

  # Known non-filesystem internal harness URIs are tool interfaces, not Git
  # destinations — but only in PATH mode, and only when they cannot resolve
  # into the repository: candidates masked by ../ traversal or by an absolute
  # path after the scheme are filesystem paths wearing a URI prefix and must
  # be classified normally. In SHELL mode every candidate is a literal write
  # target, so a URI prefix never bypasses classification.
  case "$candidate" in
    xd://*|skill://*|rule://*|agent://*|history://*|artifact://*|local://*|vault://*|mcp://*|issue://*|pr://*|omp://*)
      if [ "$mode" = path ]; then
        rest="${candidate#*://}"
        case "$candidate" in
          *"/../"*|*"/..") ;;  # traversal-masked path: classify normally
          *) case "$rest" in
               /*) ;;          # absolute-path-masked: classify normally
               *) return 1 ;;  # genuine internal URI: bypass classification
             esac ;;
        esac
      fi
      ;;
  esac
  case "$candidate" in
    /*) abs="$candidate" ;;
    *) abs="$cwd/$candidate" ;;
  esac
  # Allow local, gitignored agent config (machine setup, never committed).
  case "$abs" in
    */.claude/settings*.json|.claude/settings*.json|*/.claude/hooks/*) return 1 ;;
  esac
  case "$candidate" in
    */.claude/settings*.json|.claude/settings*.json|*/.claude/hooks/*) return 1 ;;
  esac

  decision="$(python3 - "$abs" "$gate_scope" "$repo_topology" "$protected" <<'PYEOF'
import os
import subprocess
import sys

target = sys.argv[1]
scope = sys.argv[2]
topology = sys.argv[3]
protected = sys.argv[4].split()

def git(cwd, *args):
    try:
        return subprocess.run(
            ["git", "-C", str(cwd), *args],
            check=True, capture_output=True, text=True,
        ).stdout.strip()
    except (OSError, subprocess.CalledProcessError):
        return ""

def inside(path, root):
    try:
        return os.path.commonpath([path, root]) == root
    except ValueError:
        return False

def existing_ancestor(path):
    path = os.path.abspath(path)
    probe = path
    while not os.path.exists(probe):
        parent = os.path.dirname(probe)
        if parent == probe:
            return ""
        probe = parent
    return probe if os.path.isdir(probe) else os.path.dirname(probe)

def git_common(root):
    value = git(root, "rev-parse", "--path-format=absolute", "--git-common-dir")
    if not value:
        value = git(root, "rev-parse", "--git-common-dir")
    if not value:
        return ""
    # Git versions without --path-format=absolute return a path relative to
    # the repository passed with -C, not the hook process cwd.
    if not os.path.isabs(value):
        value = os.path.join(root, value)
    return os.path.realpath(value)

def module_records(super_root):
    """Return proven initialized module tuples, or None on ambiguity."""
    gm = os.path.join(super_root, ".gitmodules")
    dotgit = os.path.join(super_root, ".git")
    if not os.path.isfile(gm) or not (os.path.isdir(dotgit) or os.path.isfile(dotgit)):
        return None
    raw = git(super_root, "config", "--file", ".gitmodules", "--get-regexp", r"^submodule\..*\.path$")
    if not raw:
        return None
    entries = []
    seen = set()
    for line in raw.splitlines():
        parts = line.split(None, 1)
        if len(parts) != 2:
            continue
        rel = parts[1].strip()
        if not rel:
            continue
        module = os.path.realpath(os.path.join(super_root, rel))
        if not inside(module, super_root) or module == super_root or module in seen:
            return None
        if any(inside(module, prior) or inside(prior, module) for prior in seen):
            return None
        seen.add(module)
        status = git(super_root, "submodule", "status", "--", rel)
        if not status:
            continue
        status_line = status.splitlines()[0]
        if status_line[:1] == "-":
            continue
        common = git_common(module)
        expected = os.path.realpath(os.path.join(super_root, ".git", "modules", rel))
        owner = git(module, "rev-parse", "--show-toplevel")
        owner = os.path.realpath(owner) if owner else ""
        if owner != module or not common or common != expected:
            continue
        entries.append((module, common))
    return entries

def classify(repo_root, repo_common):
    if topology in ("standalone", "monorepo-apps"):
        return "unproven"
    repo_root = os.path.realpath(repo_root)
    repo_common = os.path.realpath(repo_common)
    # The containing superrepo must itself be a primary checkout. Walk all
    # ancestors and reject multiple matching relationships (nested/ambiguous).
    matches = []
    probe = repo_root
    while True:
        records = module_records(probe)
        if records is not None:
            for module, common in records:
                if module == repo_root and common == repo_common:
                    matches.append((probe, module, common))
        parent = os.path.dirname(probe)
        if parent == probe:
            break
        probe = parent
    if len(matches) > 1:
        return "unproven"
    if len(matches) == 1:
        return "subrepo"
    records = module_records(repo_root)
    if records:
        return "superrepo"
    return "unproven"

try:
    canonical = os.path.realpath(target)
    ancestor = existing_ancestor(target)
    if not ancestor:
        print("allow")
        raise SystemExit
    repo_root = git(ancestor, "rev-parse", "--show-toplevel")
    if not repo_root:
        print("allow")
        raise SystemExit
    repo_root = os.path.realpath(repo_root)
    if not inside(canonical, repo_root):
        print("allow")
        raise SystemExit
    git_dir = git(ancestor, "rev-parse", "--absolute-git-dir")
    common = git_common(ancestor)
    if not git_dir or not common:
        print("allow")
        raise SystemExit
    if os.path.realpath(git_dir) != common:
        print("allow")
        raise SystemExit
    branch = git(ancestor, "symbolic-ref", "--short", "HEAD")
    if not branch or branch not in protected:
        print("allow")
        raise SystemExit
    owner = classify(repo_root, common)
    central = os.path.realpath(os.path.join(repo_root, "openspec", "changes"))
    if owner == "superrepo":
        # Explicit subrepo scope intentionally leaves superrepo writes to the
        # caller (Melón central-planning workflow); central paths remain an
        # explicit exception for the enforcing scopes.
        if scope == "subrepo" or inside(canonical, central):
            print("allow")
        else:
            print("block:" + branch)
    elif owner == "subrepo" and scope == "superrepo":
        print("allow")
    else:
        print("block:" + branch)
except Exception:
    # A topology or canonicalization failure must never wedge editing.
    print("allow")
PYEOF
  )" || decision="allow"

  case "$decision" in block:*) blocked_branch="${decision#block:}" ;; *) return 1 ;; esac
  if [ "$mode" = shell ]; then
    echo "worktree-gate: refusing shell command that writes '$candidate' on protected branch '$blocked_branch' in the main worktree — using bash/shell to write here bypasses the worktree gate. Create a dedicated worktree first (e.g. /worktree-new) and run there — exploration ends at the first write." >&2
  else
    echo "worktree-gate: refusing to ${tool_name:-edit} '$candidate' on protected branch '$blocked_branch' in the main worktree. Create a dedicated worktree first (e.g. /worktree-new) and edit there — exploration ends at the first write." >&2
  fi
  if [ "$gate_mode" = ask ]; then
    echo "worktree-gate: to bypass, set WORKTREE_GATE_MODE=off in the environment that launches the agent, then retry. An inline `WORKTREE_GATE_MODE=off <command>` prefix does NOT work: this hook runs before <command> and reads its own environment." >&2
  fi
  exit 2
}

for cand in "${candidates[@]}"; do
  resolve_and_check "$cand"
done

exit 0
