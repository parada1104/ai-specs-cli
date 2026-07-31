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

stamped_gate_mode="__WORKTREE_GATE_MODE__"
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

# off → disable the gate entirely.
[ "$gate_mode" = off ] && exit 0

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

# cwd from event (top-level); fallback to script PWD
event_cwd="$(python3 - "$input" <<'PYEOF' 2>/dev/null
import json, sys
try:
    d = json.loads(sys.argv[1])
except Exception:
    print("")
    sys.exit(0)
if not isinstance(d, dict):
    print("")
    sys.exit(0)
print(d.get("cwd") or "")
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

# Shared resolution: PATH mode feeds one candidate; SHELL mode feeds N.
# First protected-main hit blocks; otherwise fail-open.
resolve_and_check() {
  local candidate="$1"
  local abs dir git_dir common_dir branch

  case "$candidate" in
    /*) abs="$candidate" ;;
    *) abs="$cwd/$candidate" ;;
  esac

  # Allow local, gitignored agent config (machine setup, never committed).
  case "$abs" in
    */.claude/settings*.json|.claude/settings*.json|*/.claude/hooks/*) return 1 ;;
  esac
  # Also match relative-style allowlist paths (path mode historically matched file_path as given)
  case "$candidate" in
    */.claude/settings*.json|.claude/settings*.json|*/.claude/hooks/*) return 1 ;;
  esac

  dir="$(dirname "$abs")"
  while [ ! -d "$dir" ] && [ "$dir" != "/" ] && [ "$dir" != "." ]; do
    dir="$(dirname "$dir")"
  done
  [ -d "$dir" ] || return 1

  git -C "$dir" rev-parse --is-inside-work-tree >/dev/null 2>&1 || return 1

  git_dir="$(git -C "$dir" rev-parse --absolute-git-dir 2>/dev/null)" || return 1
  common_dir="$(git -C "$dir" rev-parse --path-format=absolute --git-common-dir 2>/dev/null)" || return 1

  # Linked worktree (git_dir != common_dir) → allowed.
  [ "$git_dir" != "$common_dir" ] && return 1

  # Main worktree: gate on the current branch.
  branch="$(git -C "$dir" symbolic-ref --short HEAD 2>/dev/null)" || return 1
  local b
  for b in $protected; do
    if [ "$branch" = "$b" ]; then
      if [ "$mode" = shell ]; then
        echo "worktree-gate: refusing shell command that writes '$candidate' on protected branch '$branch' in the main worktree — using bash/shell to write here bypasses the worktree gate. Create a dedicated worktree first (e.g. /worktree-new) and run there — exploration ends at the first write." >&2
      else
        echo "worktree-gate: refusing to ${tool_name:-edit} '$candidate' on protected branch '$branch' in the main worktree. Create a dedicated worktree first (e.g. /worktree-new) and edit there — exploration ends at the first write." >&2
      fi
      if [ "$gate_mode" = ask ]; then
        echo "worktree-gate: to bypass for this invocation, re-run with WORKTREE_GATE_MODE=off" >&2
      fi
      exit 2
    fi
  done
  return 1
}

for cand in "${candidates[@]}"; do
  resolve_and_check "$cand"
done

exit 0
