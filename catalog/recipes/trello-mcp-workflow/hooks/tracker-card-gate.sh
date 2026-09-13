#!/usr/bin/env bash
# tracker-card-gate.sh — pre-tool-use guard distributed by trello-mcp-workflow.
#
# Semantic model: the ledger is the only grader. This host is a thin
# acquisition/JSON bridge to the verified Go `--ledger` predicate: it maps a
# production path write to the `apply-start` checkpoint and `gh pr create` to
# the `pr-review` checkpoint. Archive-close is graded by the pre-merge guardian.
#
# Dual-input contract (one script, every harness):
#   PATH mode stdin = JSON { "event", "tool_name",
#     "tool_input": {file_path|notebook_path}, "cwd" }
#   SHELL mode stdin = JSON with tool_input.command (or script/cmd) OR Cursor
#     native top-level { "command", "cwd", … }
#   exit 0 → allow.   exit 2 → block (stderr surfaced to the agent).
# Fail-open: a missing/unverified binary, any parse/lookup/git/python3 error,
# or an ambiguous event allows the action. `openspec/**` is never blocked.
#
# Tokens stamped at sync (gitignored project copy):
#   __TRACKER_CARD_GATE_MODE__   (legacy gate_mode; default warn)
#   __TRACKER_CLI_HOME__         (CLI install home for binary resolution)
#
# Config / env:
#   TRACKER_LEDGER_MODE       env override for the ledger mode (always|ask|warn)
#   TRACKER_CARD_GATE_MODE    legacy gate_mode env override (off|warn|always)
#   TRACKER_CARD_GATE_PATHS   space-separated production dirs
#                             (default: "lib catalog bin src")
#   AI_SPECS_HOME             preferred over stamped CLI home for the binary
#   WORKTREE_GATE_BIN         explicit verified-binary override (debugging/tests)

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
BT = chr(96)  # backtick literal kept out of source: bash 3.2 misparses it inside heredocs in command substitution

WRAPPERS = {"sudo", "env", "nice", "time", "nohup", "xargs", "command", "coproc"}
SEPS = {"|", "||", "&&", ";", "&", "|&", ";;", ";&", ";;&"}

def command_word(seg, case_context=False):
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
                if i < len(seg):
                    pattern = seg[i]
                    if ")" in pattern:
                        remainder = pattern.split(")", 1)[1]
                        if remainder:
                            return i, remainder
                    if pattern != ")":
                        i += 1
                    if i < len(seg) and seg[i] == ")":
                        i += 1
            continue
        if t in {"in", "then", "do", "else", "{", "("}:
            i += 1
            continue
        if case_context and ")" in t:
            remainder = t.split(")", 1)[1]
            if not remainder:
                i += 1
                continue
            t = remainder
        if t.startswith("(") and t != "((":
            t = t[1:]
            if not t:
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
                                if quote == '"' and line[end] == "\\" and end + 1 < len(line) and line[end + 1] in ("$", BT, '"', "\\", "\n"):
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
    """Return list of (action, detail) with action in {pr_create}."""
    actions = []
    case_context = bool(re.search(r"(?:^|[;|&()\n])\s*case(?:\s|$)", cmd))
    for seg in segments(cmd):
        if not seg:
            continue
        idx, cw = command_word(seg, case_context=case_context)
        if cw is None:
            continue
        body = seg[idx:]
        rest = nonflag_args(body)
        # gh pr create
        if cw == "gh" and len(rest) >= 2 and rest[0] == "pr" and rest[1] == "create":
            actions.append(("pr_create", ""))
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
# --- Ledger checkpoint bridge (acquisition + JSON only; no predicate) ---------
# The five ledger checkpoints are graded by the verified Go `--ledger` mode on
# the shared worktree-gate binary (one predicate, one trust root). This host
# only resolves the A9 mode, acquires a verified binary, and maps the JSON
# verdict. A missing/unverified binary, a parse error, or an IO failure fails
# open (exit 0).

_ledger_mode() {
  local root="$1"
  local gate_hint="${2:-}"
  python3 - "$root" "${TRACKER_LEDGER_MODE:-}" "$gate_hint" <<'PY' 2>/dev/null
import sys, tomllib
from pathlib import Path
root, env_mode, gate_hint = sys.argv[1], sys.argv[2], sys.argv[3]
ledger = gate = ""
try:
    data = tomllib.loads((Path(root) / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8"))
    cfg = ((data.get("recipes") or {}).get("trello-mcp-workflow") or {}).get("config") or {}
    ledger = cfg.get("ledger_mode") or ""
    gate = cfg.get("gate_mode") or ""
except Exception:
    pass
if gate_hint in ("off", "warn", "always"):
    gate = gate_hint
if env_mode in ("always", "ask", "warn"):
    print(env_mode)
elif ledger in ("always", "ask", "warn"):
    print(ledger)
elif gate == "off":
    print("off")
elif gate == "always":
    print("always")
else:
    print("warn")
PY
}

_ledger_binary() {
  if [ -n "${WORKTREE_GATE_BIN:-}" ] && [ -x "$WORKTREE_GATE_BIN" ]; then
    printf '%s\n' "$WORKTREE_GATE_BIN"
    return 0
  fi
  local home="${AI_SPECS_HOME:-}"
  if [ -z "$home" ]; then
    case "$stamped_cli_home" in
      ""|__*) home="$HOME/.ai-specs" ;;
      *) home="$stamped_cli_home" ;;
    esac
  fi
  local version=""
  if [ -f "$home/VERSION" ]; then version="$(tr -d '[:space:]' < "$home/VERSION")"; fi
  [ -n "$version" ] || version="dev"
  local goos goarch
  case "$(uname -s)" in Darwin) goos=darwin ;; Linux) goos=linux ;; *) return 1 ;; esac
  case "$(uname -m)" in arm64|aarch64) goarch=arm64 ;; x86_64|amd64) goarch=amd64 ;; *) return 1 ;; esac
  local candidate="$home/cache/bin/worktree-gate/$version/$goos-$goarch/worktree-gate"
  [ -x "$candidate" ] || return 1
  [ -f "$candidate.verified" ] || return 1
  printf '%s\n' "$candidate"
}

_ledger_field() {
  # stdin: verdict JSON. $1: top-level key.
  python3 -c '
import json, sys
try:
    data = json.load(sys.stdin)
except Exception:
    raise SystemExit(1)
value = data.get(sys.argv[1], "")
if isinstance(value, bool):
    print("1" if value else "0")
elif value is None:
    print("")
else:
    print(value)
' "$1" 2>/dev/null
}

_ledger_ask() {
  # $1 bin, $2 checkpoint, $3 mode, $4 root, $5 prefix; stdin: prompt JSON.
  local bin="$1" checkpoint="$2" mode="$3" root="$4" prefix="$5"
  python3 -c '
import json, sys
try:
    prompt = json.load(sys.stdin) or {}
except Exception:
    prompt = {}
ev = prompt.get("evidence") or {}
for side in ("local", "remote", "code", "git"):
    print("  %s: %s" % (side, ev.get(side) or "(unavailable)"))
print("  choices: " + ", ".join(prompt.get("choices") or []))
' >&2 2>/dev/null
  if ! { exec 3</dev/tty; } 2>/dev/null; then
    echo "${prefix}: ${checkpoint} needs a decision but no terminal is available; proceeding." >&2
    return 0
  fi
  printf '%s: opt out of the %s checkpoint? [y/N] ' "$prefix" "$checkpoint" >&2
  local answer=""
  read -r answer <&3 || answer=""
  exec 3<&-
  case "$answer" in
    y|Y|yes|YES|Yes)
      if "$bin" --ledger --checkpoint "$checkpoint" --ledger-mode "$mode" --project-root "$root" \
          --decide "{\"checkpoint\":\"$checkpoint\",\"kind\":\"opt-out\",\"choice\":\"continue\"}" >/dev/null 2>&1; then
        echo "${prefix}: opt-out recorded for ${checkpoint}; proceeding." >&2
        return 0
      fi
      echo "${prefix}: failed to record the ${checkpoint} opt-out; blocking." >&2
      return 2
      ;;
  esac
  echo "${prefix}: no opt-out recorded for ${checkpoint}; blocking." >&2
  return 2
}

_ledger_grade() {
  # $1 checkpoint, $2 mode, $3 root, $4 prefix. Returns 0 allow / 2 block.
  local checkpoint="$1" mode="$2" root="$3" prefix="$4"
  local bin
  bin="$(_ledger_binary)" || return 0
  local out rc
  out="$("$bin" --ledger --checkpoint "$checkpoint" --ledger-mode "$mode" --project-root "$root" 2>/dev/null)"
  rc=$?
  [ -n "$out" ] || return 0
  local decision reason active
  decision="$(printf '%s' "$out" | _ledger_field decision)" || return 0
  [ -n "$decision" ] || return 0
  reason="$(printf '%s' "$out" | _ledger_field reason)"
  active="$(printf '%s' "$out" | _ledger_field active)"
  if [ "$rc" = 2 ] || [ "$decision" = block ]; then
    echo "${prefix}: blocked at ${checkpoint} — ${reason:-missing tracked item}" >&2
    return 2
  fi
  if [ "$decision" = ask ]; then
    printf '%s' "$out" | python3 -c 'import json,sys; d=json.load(sys.stdin); print(json.dumps(d.get("prompt")))' 2>/dev/null \
      | _ledger_ask "$bin" "$checkpoint" "$mode" "$root" "$prefix"
    return $?
  fi
  if [ "$active" = 1 ] && [ "$decision" != allow ]; then
    echo "${prefix}: ${decision} at ${checkpoint} — ${reason:-no primary item}" >&2
  fi
  return 0
}

# Resolve the owning repo root from a cwd or path hint (never $PWD for assets).
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

  mode="$(_ledger_mode "$repo_root" "$gate_mode")"
  [ "$mode" = off ] && exit 0
  _ledger_grade "apply-start" "$mode" "$repo_root" "tracker-card-gate" || exit 2
  exit 0
fi

if [ "$kind" = shell ]; then
  repo_root="$(_resolve_repo "${cwd:-$PWD}")" || exit 0
  [ -n "$repo_root" ] || exit 0
  while IFS=$'\t' read -r action detail; do
    [ -n "$action" ] || continue
    case "$action" in
      pr_create)
        mode="$(_ledger_mode "$repo_root" "$gate_mode")"
        [ "$mode" = off ] && continue
        _ledger_grade "pr-review" "$mode" "$repo_root" "tracker-card-gate" || exit 2
        ;;
      *)
        ;;
    esac
  done < <(printf '%s\n' "$parsed" | tail -n +2)
  exit 0
fi

exit 0
