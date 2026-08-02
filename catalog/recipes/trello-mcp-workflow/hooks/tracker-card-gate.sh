#!/usr/bin/env bash
# tracker-card-gate.sh — pre-tool-use guard distributed by trello-mcp-workflow.
#
# Semantic model: plan-build-gate (artifact before production). Enforces that
# every active OpenSpec change carries a ## Tracker link section (or
# tracker.none) before production writes and high-confidence gh pr create /
# change-archive shell actions.
#
# Dual-input contract (one script, every harness):
#   PATH mode stdin = JSON { "event", "tool_name",
#     "tool_input": {file_path|notebook_path}, "cwd" }
#   SHELL mode stdin = JSON with tool_input.command (or script/cmd) OR Cursor
#     native top-level { "command", "cwd", … }
#   exit 0 → allow.   exit 2 → block (stderr surfaced to the agent).
# Fail-open: any parse/lookup/git/python3/ambiguous error allows the action.
#
# Tokens stamped at sync (gitignored project copy):
#   __TRACKER_CARD_GATE_MODE__   (default warn)
#   __TRACKER_CLI_HOME__         (CLI install home for cache marker resolve)
#
# Config / env:
#   TRACKER_CARD_GATE_MODE    env override beats stamp (off|warn|always)
#   TRACKER_CARD_GATE_PATHS   space-separated production dirs
#                             (default: "lib catalog bin src")
#   AI_SPECS_HOME             preferred over stamped CLI home for marker

stamped_gate_mode="__TRACKER_CARD_GATE_MODE__"
stamped_cli_home="__TRACKER_CLI_HOME__"
prod_dirs="${TRACKER_CARD_GATE_PATHS:-lib catalog bin src}"
[ -n "${prod_dirs// /}" ] || prod_dirs="lib catalog bin src"

_resolve_gate_mode() {
  local candidate="${TRACKER_CARD_GATE_MODE:-$stamped_gate_mode}"
  case "$candidate" in off|warn|always) echo "$candidate" ; return ;;
  esac
  if [ -n "${TRACKER_CARD_GATE_MODE:-}" ]; then
    echo "tracker-card-gate: ignoring invalid TRACKER_CARD_GATE_MODE='${TRACKER_CARD_GATE_MODE}'; falling back to stamped mode." >&2
  elif [ "$stamped_gate_mode" != off ] && [ "$stamped_gate_mode" != warn ] && [ "$stamped_gate_mode" != always ]; then
    echo "tracker-card-gate: invalid stamped gate_mode='${stamped_gate_mode}'; falling back to warn." >&2
  fi
  case "$stamped_gate_mode" in off|warn|always) echo "$stamped_gate_mode" ;;
  *) echo warn ;;
  esac
}
gate_mode="$(_resolve_gate_mode)"
[ "$gate_mode" = off ] && exit 0

input="$(cat)"

# Protocol on stdout from the embedded python:
#   line 1: <kind>\t<tool_name>\t<repo_hint>
#     kind ∈ {path, shell, none}
#   path → line 2: <abs_or_rel_file_path>
#   shell → line 2: <action>\t<details>
#     action ∈ {pr_create, archive}\t<details may be slug or empty>
# Fail-open: any python error → exit 0.
parsed="$(python3 - "$input" "$stamped_cli_home" <<'PYEOF' 2>/dev/null
import json, re, shlex, sys

WRAPPERS = {"sudo", "env", "nice", "time", "nohup", "xargs", "command", "coproc"}
SEPS = {"|", "||", "&&", ";", "&", "|&", ";;", ";&", ";;&"}

def command_word(seg):
    i = 0
    while i < len(seg):
        t = seg[i]
        if "=" in t and not t.startswith("=") and re.match(r"^[A-Za-z_][A-Za-z0-9_]*=", t):
            i += 1
            continue
        if t in WRAPPERS:
            i += 1
            while i < len(seg) and seg[i].startswith("-"):
                flag = seg[i]
                i += 1
                if flag in {"-n", "--max-args", "-I", "--replace", "-P", "--max-procs", "-d", "--delimiter"} and i < len(seg):
                    i += 1
            if t == "coproc" and i + 1 < len(seg) and not seg[i].startswith("-") and seg[i + 1] in {"{", "("}:
                i += 1
            continue
        if t == "case":
            i += 1
            while i < len(seg) and seg[i] != "in":
                i += 1
            if i < len(seg):
                i += 1
                if i < len(seg) and seg[i] != ")":
                    i += 1
                if i < len(seg) and seg[i] == ")":
                    i += 1
            continue
        if t in {"in", "then", "do", "else", "{"}:
            i += 1
            continue
        if t == ")":
            i += 1
            continue
        if t == "*" and i + 1 < len(seg) and seg[i + 1] == ")":
            i += 2
            continue
        if t.startswith("*") and t.endswith(")"):
            i += 1
            continue
        return i, t
    return None, None


def _line_continuation_state(line, single, double, at_word_start=None):
    """Return quote state, word-start state, and whether an unquoted trailing backslash joins."""
    i = 0
    if at_word_start is None:
        at_word_start = not (single or double)
    while i < len(line):
        char = line[i]
        if single:
            if char == "'":
                single = False
            i += 1
            continue
        if double:
            if char == "\\" and i + 1 < len(line):
                i += 2
                continue
            if char == '"':
                double = False
            i += 1
            continue
        if char == "'":
            single = True
            at_word_start = False
        elif char == '"':
            double = True
            at_word_start = False
        elif char == "\\":
            if i + 1 < len(line):
                i += 1
                at_word_start = False
        elif char == "#" and at_word_start:
            return single, double, False, at_word_start
        else:
            at_word_start = char in " \t;|&()<>"
        i += 1
    trailing_backslashes = len(line) - len(line.rstrip("\\"))
    return single, double, not single and trailing_backslashes % 2 == 1, at_word_start


def _fold_unquoted_heredoc_line(line, physical_lines, index):
    while (len(line) - len(line.rstrip("\\"))) % 2 == 1 and index + 1 < len(physical_lines):
        line = line[:-1] + physical_lines[index + 1]
        index += 1
    return line, index


def _preprocess_command(cmd: str) -> str:
    """Remove shell comments and heredoc bodies without parsing quoted text."""
    output = []
    pending = []
    single = False
    double = False
    physical_lines = cmd.split("\n")
    index = 0
    while index < len(physical_lines):
        line = physical_lines[index]
        if pending:
            delimiter, strip_tabs, quoted = pending[0]
            if not quoted:
                line, index = _fold_unquoted_heredoc_line(line, physical_lines, index)
            candidate = line.lstrip("\t") if strip_tabs else line
            if candidate == delimiter:
                pending.pop(0)
            index += 1
            continue

        entry_single, entry_double = single, double
        fold_at_word_start = not (single or double)
        logical_line = line
        while True:
            next_single, next_double, joins_next, fold_at_word_start = _line_continuation_state(
                physical_lines[index], single, double, fold_at_word_start
            )
            single, double = next_single, next_double
            if not joins_next or index + 1 >= len(physical_lines):
                break
            logical_line = logical_line[:-1] + physical_lines[index + 1]
            index += 1
        line = logical_line
        visible = []
        single, double = entry_single, entry_double
        at_word_start = not (single or double)
        arithmetic_depth = 0
        arithmetic_brackets = 0
        i = 0
        while i < len(line):
            char = line[i]
            if single:
                visible.append(char)
                if char == "'":
                    single = False
                i += 1
                continue
            if double:
                visible.append(char)
                if char == "\\" and i + 1 < len(line):
                    visible.append(line[i + 1])
                    i += 2
                    continue
                if char == '"':
                    double = False
                i += 1
                continue
            if char == "'":
                single = True
                at_word_start = False
                visible.append(char)
                i += 1
                continue
            if char == '"':
                double = True
                at_word_start = False
                visible.append(char)
                i += 1
                continue
            if char == "\\":
                visible.append(char)
                if i + 1 < len(line):
                    visible.append(line[i + 1])
                    i += 2
                else:
                    i += 1
                at_word_start = False
                continue
            if char == "$" and line[i:i + 3] == "$" + "((":
                visible.extend(line[i:i + 3])
                arithmetic_depth += 1
                i += 3
                at_word_start = False
                continue
            if char == "(" and line[i:i + 2] == "((" and at_word_start:
                visible.extend(line[i:i + 2])
                arithmetic_depth += 1
                i += 2
                at_word_start = False
                continue
            if char == "$" and line[i:i + 2] == "$[":
                visible.extend(line[i:i + 2])
                arithmetic_depth += 1
                arithmetic_brackets += 1
                i += 2
                at_word_start = False
                continue
            if char == "]" and arithmetic_brackets:
                visible.append(char)
                arithmetic_brackets -= 1
                arithmetic_depth -= 1
                i += 1
                at_word_start = False
                continue
            if char == ")" and line[i:i + 2] == "))" and arithmetic_depth > arithmetic_brackets:
                visible.extend(line[i:i + 2])
                arithmetic_depth -= 1
                i += 2
                at_word_start = False
                continue
            if char == "#" and at_word_start:
                break
            if char == "<" and i + 1 < len(line) and line[i + 1] == "<" and not arithmetic_depth:
                delimiter_start = i + 2
                strip_tabs = delimiter_start < len(line) and line[delimiter_start] == "-"
                if strip_tabs:
                    delimiter_start += 1
                if delimiter_start >= len(line) or line[delimiter_start] != "<":
                    while delimiter_start < len(line) and line[delimiter_start] in " \t":
                        delimiter_start += 1
                    end = delimiter_start
                    delimiter = []
                    quoted_delimiter = False
                    while end < len(line) and line[end] not in " \t;|&()<>":
                        if line[end] == "\\" and end + 1 < len(line):
                            delimiter.append(line[end + 1])
                            quoted_delimiter = True
                            end += 2
                        elif line[end] in "'\"":
                            quote = line[end]
                            quoted_delimiter = True
                            end += 1
                            while end < len(line) and line[end] != quote:
                                if quote == '"' and line[end] == "\\" and end + 1 < len(line) and line[end + 1] in ("$", "`", '"', "\\", "\n"):
                                    delimiter.append(line[end + 1])
                                    end += 2
                                    continue
                                delimiter.append(line[end])
                                end += 1
                            if end < len(line):
                                end += 1
                        else:
                            delimiter.append(line[end])
                            end += 1
                    if delimiter:
                        pending.append(("".join(delimiter), strip_tabs, quoted_delimiter))
                visible.extend(line[i:i + 2])
                i += 2
                at_word_start = True
                continue
            visible.append(char)
            at_word_start = char in " \t;|&()<>"
            i += 1
        output.append("".join(visible))
        index += 1
    return "\n".join(output)


def _unterminated_quote_line(prepared: str):
    single = double = False
    escaped = False
    opened_line = None
    line_no = 1
    for char in prepared:
        if char == "\n":
            line_no += 1
            escaped = False
            continue
        if single:
            if char == "'":
                single = False
            continue
        if escaped:
            escaped = False
            continue
        if char == "\\":
            escaped = True
            continue
        if not double and char == "'":
            single = not single
            if single:
                opened_line = line_no
        elif not single and char == '"':
            double = not double
            if double:
                opened_line = line_no
    return opened_line if (single or double) else None


def segments(cmd: str):
    prepared = _preprocess_command(cmd)
    if (len(prepared) - len(prepared.rstrip("\\"))) % 2 == 1:
        prepared = prepared[:-1]
    lexer = shlex.shlex(prepared.replace("\n", " ;\n"), posix=True, punctuation_chars=";|&")
    lexer.commenters = ""
    lexer.whitespace_split = True
    tokens = []
    error_line = None
    try:
        while True:
            token_line = lexer.lineno
            tokens.append((token_line, next(lexer)))
    except StopIteration:
        pass
    except ValueError:
        error_line = _unterminated_quote_line(prepared)
    if error_line is not None:
        tokens = [(line, token) for line, token in tokens if line < error_line]
    segs, cur = [], []
    case_context = any(t in {"case", "esac"} for _, t in tokens)
    for pos, (_, t) in enumerate(tokens):
        previous = tokens[pos - 1][1] if pos else ""
        following = tokens[pos + 1][1] if pos + 1 < len(tokens) else ""
        if t == "&" and (previous.endswith(("<", ">")) or following.startswith(">")):
            cur.append(t)
            continue
        if t in SEPS and (t not in {";;", ";&", ";;&"} or case_context):
            if cur:
                segs.append(cur)
            cur = []
        else:
            cur.append(t)
    if cur:
        segs.append(cur)
    return segs


def nonflag_args(body):
    return [t for t in body[1:] if not t.startswith("-")]


def detect_shell_actions(cmd: str):
    """Return list of (action, detail) with action in {pr_create, archive}."""
    actions = []
    for seg in segments(cmd):
        if not seg:
            continue
        idx, cw = command_word(seg)
        if cw is None:
            continue
        body = seg[idx:]
        rest = nonflag_args(body)
        # gh pr create
        if cw == "gh" and len(rest) >= 2 and rest[0] == "pr" and rest[1] == "create":
            actions.append(("pr_create", ""))
            continue
        # openspec archive <slug?>
        if cw == "openspec" and rest and rest[0] == "archive":
            slug = rest[1] if len(rest) >= 2 else ""
            actions.append(("archive", slug))
            continue
        # ai-specs … archive …
        if cw == "ai-specs" and "archive" in rest:
            aidx = rest.index("archive")
            slug = rest[aidx + 1] if aidx + 1 < len(rest) else ""
            actions.append(("archive", slug))
            continue
        # mv / git mv → openspec/changes/archive/
        if cw == "mv" or (cw == "git" and rest and rest[0] == "mv"):
            words = body[1:] if cw == "mv" else body[2:]
            nonflags = [t for t in words if not t.startswith("-")]
            if len(nonflags) >= 2:
                src = nonflags[0]
                dest = next(
                    (t for t in nonflags[1:] if "openspec/changes/archive/" in t.replace("\\", "/")),
                    "",
                )
                if dest:
                    slug = ""
                    src_n = src.replace("\\", "/")
                    m = re.search(r"openspec/changes/([^/]+)/?", src_n)
                    if m and m.group(1) != "archive":
                        slug = m.group(1)
                    actions.append(("archive", slug))
            continue
    return actions




try:
    d = json.loads(sys.argv[1])
except Exception:
    sys.exit(0)
if not isinstance(d, dict):
    sys.exit(0)

ti = d.get("tool_input") or {}
if not isinstance(ti, dict):
    ti = {}
tool_name = d.get("tool_name") if isinstance(d.get("tool_name"), str) else ""
cwd = d.get("cwd") if isinstance(d.get("cwd"), str) else ""

fp = ti.get("file_path") or ti.get("notebook_path") or ""
if isinstance(fp, str) and fp.strip():
    print(f"path\t{tool_name}\t{cwd}")
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
    val = ti.get(key_src[1]) if key_src[0] == "ti" else d.get(key_src[1])
    if isinstance(val, str) and val.strip():
        cmd = val
        break
if not cmd:
    sys.exit(0)

actions = detect_shell_actions(cmd)
if not actions:
    sys.exit(0)

print(f"shell\t{tool_name or 'Bash'}\t{cwd}")
for action, detail in actions:
    print(f"{action}\t{detail}")
PYEOF
)" || exit 0

[ -n "$parsed" ] || exit 0

kind_line="$(printf '%s\n' "$parsed" | head -n 1)"
kind="${kind_line%%$'\t'*}"
rest_kl="${kind_line#*$'\t'}"
tool_name="${rest_kl%%$'\t'*}"
cwd="${rest_kl#*$'\t'}"

# Shared evaluator: given repo_root + optional slug focus, print deficient list
# or empty. Exit code unused; stdout = comma-separated deficient slugs.
_eval_deficient() {
  local repo_root="$1"
  local focus_slug="${2:-}"
  python3 - "$repo_root" "$focus_slug" "$stamped_cli_home" <<'PYEOF' 2>/dev/null
import hashlib, os, re, sys
from pathlib import Path

PAIR_RE = re.compile(
    r"^\s*(?:[-*]\s+)?\*{0,2}(?P<key>[A-Za-z_][A-Za-z0-9_]*)\*{0,2}\s*:\s*(?P<value>.*)$"
)
RECOGNIZED = frozenset({"card_id", "shortlink", "url", "list", "pr"})
TRACKER_HEADING = re.compile(r"^##\s+Tracker\s*$")
H2 = re.compile(r"^##\s+")
BASENAME_SAFE = re.compile(r"[^A-Za-z0-9._-]+")

def clean_value(raw: str) -> str:
    value = raw.strip()
    hash_at = value.find(" #")
    if hash_at != -1:
        before = value[:hash_at]
        if before.count("`") % 2 == 0:
            value = before.strip()
    if len(value) >= 2 and value.startswith("`") and value.endswith("`"):
        value = value[1:-1].strip()
    return value

def extract_body(text: str):
    lines = text.splitlines()
    start = None
    in_fence = False
    fence_marker = ""
    for i, line in enumerate(lines):
        stripped = line.lstrip()
        if stripped.startswith("```") or stripped.startswith("~~~"):
            marker = stripped[:3]
            if not in_fence:
                in_fence = True
                fence_marker = marker
            elif stripped.startswith(fence_marker):
                in_fence = False
                fence_marker = ""
            continue
        if in_fence:
            continue
        if TRACKER_HEADING.match(line):
            start = i + 1
            break
    if start is None:
        return None
    body = []
    in_fence = False
    fence_marker = ""
    for line in lines[start:]:
        stripped = line.lstrip()
        if stripped.startswith("```") or stripped.startswith("~~~"):
            marker = stripped[:3]
            if not in_fence:
                in_fence = True
                fence_marker = marker
            elif stripped.startswith(fence_marker):
                in_fence = False
                fence_marker = ""
            body.append(line)
            continue
        if not in_fence and H2.match(line):
            break
        body.append(line)
    return "\n".join(body)

def parse_section(paths):
    for path in paths:
        try:
            p = Path(path)
            if not p.is_file():
                continue
            text = p.read_text(encoding="utf-8")
        except OSError:
            continue
        body = extract_body(text)
        if body is None:
            continue
        out = {}
        in_fence = False
        fence_marker = ""
        for line in body.splitlines():
            stripped = line.lstrip()
            if stripped.startswith("```") or stripped.startswith("~~~"):
                marker = stripped[:3]
                if not in_fence:
                    in_fence = True
                    fence_marker = marker
                elif stripped.startswith(fence_marker):
                    in_fence = False
                    fence_marker = ""
                continue
            if in_fence or not line.strip() or stripped.startswith("#"):
                continue
            m = PAIR_RE.match(line)
            if not m:
                continue
            key = m.group("key").lower()
            if key not in RECOGNIZED or key in out:
                continue
            out[key] = clean_value(m.group("value"))
        return out
    return {}

def is_valid_link(change_dir: Path) -> bool:
    data = parse_section([change_dir / "proposal.md", change_dir / "tasks.md"])
    return bool(data.get("card_id"))

def sanitize_basename(name: str) -> str:
    cleaned = BASENAME_SAFE.sub("-", name).strip("-._")
    return cleaned or "project"

def marker_present(repo_root: Path, stamped_home: str) -> bool:
    local = repo_root / ".recipe" / "trello-mcp-workflow" / "bootstrap-ready"
    if local.is_file():
        return True
    home = os.environ.get("AI_SPECS_HOME") or stamped_home
    if not home or str(home).startswith("__TRACKER_CLI_HOME"):
        return False
    try:
        resolved = repo_root.resolve()
    except OSError:
        return False
    digest = hashlib.sha256(str(resolved).encode("utf-8")).hexdigest()[:12]
    key = f"{digest}-{sanitize_basename(resolved.name)}"
    primary = Path(home) / "cache" / "projects" / key / ".recipe" / "trello-mcp-workflow" / "bootstrap-ready"
    return primary.is_file()

repo_root = Path(sys.argv[1])
focus = sys.argv[2] if len(sys.argv) > 2 else ""
stamped_home = sys.argv[3] if len(sys.argv) > 3 else ""

if not marker_present(repo_root, stamped_home):
    # inactive → print nothing special; bash treats empty+marker-miss via sentinel
    print("__INACTIVE__")
    sys.exit(0)

changes_dir = repo_root / "openspec" / "changes"
deficient = []
if changes_dir.is_dir():
    for child in sorted(changes_dir.iterdir()):
        if not child.is_dir() or child.name == "archive":
            continue
        if not any((child / f).is_file() for f in ("proposal.md", "tasks.md", "spec.md", "design.md")):
            continue
        if focus and child.name != focus:
            continue
        if (child / "tracker.none").is_file():
            continue
        if is_valid_link(child):
            continue
        deficient.append(child.name)

print(",".join(deficient))
PYEOF
}

_resolve_repo() {
  local hint="$1"
  local dir
  if [ -n "$hint" ]; then
    case "$hint" in
      /*) dir="$hint" ;;
      *) dir="${cwd:-$PWD}/$hint" ;;
    esac
  else
    dir="${cwd:-$PWD}"
  fi
  # If hint is a file path, use its dirname.
  if [ -f "$dir" ]; then
    dir="$(dirname "$dir")"
  fi
  while [ ! -d "$dir" ] && [ "$dir" != "/" ] && [ "$dir" != "." ]; do
    dir="$(dirname "$dir")"
  done
  [ -d "$dir" ] || return 1
  git -C "$dir" rev-parse --is-inside-work-tree >/dev/null 2>&1 || return 1
  git -C "$dir" rev-parse --show-toplevel 2>/dev/null
}

_emit_and_exit() {
  local action_desc="$1"
  local deficient="$2"
  local sample=""
  local path_hint
  local slug
  local -a deficient_slugs
  IFS=',' read -r -a deficient_slugs <<< "$deficient"
  for slug in "${deficient_slugs[@]}"; do
    [ -n "$sample" ] && sample+=", "
    sample+="$slug"
  done
  if [ "${#deficient_slugs[@]}" = 1 ]; then
    path_hint="or add openspec/changes/${sample}/tracker.none with a reason"
  else
    path_hint="or add a tracker.none exemption with a reason in each deficient change directory"
  fi
  if [ "$gate_mode" = warn ]; then
    echo "tracker-card-gate: warning — active change(s) missing ## Tracker link section: ${sample}. Create/link Trello cards and write the ## Tracker section (card_id + url), ${path_hint}. Writing under openspec/** is never blocked." >&2
    exit 0
  fi
  echo "tracker-card-gate: refusing to ${action_desc} — active change(s) '${sample}' have no ## Tracker link section in their proposal.md. Create/link Trello cards and write the ## Tracker section (card_id + url), ${path_hint}. Writing under openspec/** is never blocked." >&2
  exit 2
}


if [ "$kind" = path ]; then
  file_path="$(printf '%s\n' "$parsed" | sed -n '2p')"
  [ -n "$file_path" ] || exit 0
  case "$file_path" in
    /*) abs="$file_path" ;;
    *)  abs="${cwd:-$PWD}/$file_path" ;;
  esac
  repo_root="$(_resolve_repo "$abs")" || exit 0
  [ -n "$repo_root" ] || exit 0

  rel="$(python3 -c 'import os,sys; print(os.path.relpath(os.path.realpath(sys.argv[1]), os.path.realpath(sys.argv[2])))' "$abs" "$repo_root" 2>/dev/null)" || exit 0
  [ -n "$rel" ] || exit 0
  case "$rel" in ..|../*) exit 0 ;; esac

  # Never block openspec/changes/** or gitignored agent config.
  case "$rel" in
    openspec|openspec/*) exit 0 ;;
    .claude/settings*.json|*/.claude/settings*.json|.claude/hooks/*|*/.claude/hooks/*) exit 0 ;;
  esac

  first="${rel%%/*}"
  is_prod=0
  for p in $prod_dirs; do
    [ "$first" = "$p" ] && is_prod=1 && break
  done
  [ "$is_prod" -eq 1 ] || exit 0

  deficient="$(_eval_deficient "$repo_root")" || exit 0
  [ "$deficient" = "__INACTIVE__" ] && exit 0
  [ -n "$deficient" ] || exit 0
  _emit_and_exit "${tool_name:-edit} '$rel'" "$deficient"
fi

if [ "$kind" = shell ]; then
  repo_root="$(_resolve_repo "${cwd:-$PWD}")" || exit 0
  [ -n "$repo_root" ] || exit 0
  while IFS=$'\t' read -r action detail; do
    [ -n "$action" ] || continue
    case "$action" in
      pr_create)
        deficient="$(_eval_deficient "$repo_root")" || exit 0
        [ "$deficient" = "__INACTIVE__" ] && exit 0
        [ -n "$deficient" ] || continue
        _emit_and_exit "gh pr create" "$deficient"
        ;;
      archive)
        if [ -n "$detail" ] && [ -d "$repo_root/openspec/changes/$detail" ]; then
          deficient="$(_eval_deficient "$repo_root" "$detail")" || exit 0
        else
          deficient="$(_eval_deficient "$repo_root")" || exit 0
        fi
        [ "$deficient" = "__INACTIVE__" ] && exit 0
        [ -n "$deficient" ] || continue
        _emit_and_exit "archive '$detail'" "$deficient"
        ;;
      *)
        ;;
    esac
  done < <(printf '%s\n' "$parsed" | tail -n +2)
  exit 0
fi

exit 0
