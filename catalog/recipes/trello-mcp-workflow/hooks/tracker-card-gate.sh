#!/usr/bin/env bash
# tracker-card-gate.sh — pre-tool-use guard distributed by trello-mcp-workflow.
#
# Semantic model: the ledger is the only grader. This script is the one shell
# host: a thin acquisition/JSON bridge to the verified Go `--ledger` predicate.
# Mode resolution is Go-owned too (`ResolveLedgerMode`): this host forwards the
# raw stamped legacy gate hint as `--ledger-gate-mode` and never resolves or
# skips a checkpoint itself. As a pre-tool-use hook it maps a production path
# write to the `apply-start` checkpoint and `gh pr create` to the `pr-review`
# checkpoint. As a CLI (`--root <root> --checkpoint pre-merge|archive-close`, or
# the retained `--stage pre-merge|pre-archive` alias) it grades the Tracker
# lifecycle checkpoints directly. That CLI mode absorbed the retired
# `lib/_internal/tracker_ledger_host.py` Python host: no separate Python host
# exists for `pre-merge` or `archive-close`.
#
# Dual-input contract (one script, every harness):
#   PATH mode stdin = JSON { "event", "tool_name",
#     "tool_input": {file_path|notebook_path}, "cwd" }
#   SHELL mode stdin = JSON with tool_input.command (or script/cmd) OR Cursor
#     native top-level { "command", "cwd", … }
#   exit 0 → no block. Every Tracker verdict is advisory: a Go `block`/`ask`/
#     `needs-item` is reported on stderr and this host still exits 0. It never
#     blocks a source change or a Tracker lifecycle boundary.
#   exit 2 → reserved for a usage error (bad/missing --root, --checkpoint, or
#     --stage), never for a ledger verdict.
# Fail-open: a missing/unverified binary, an unstamped/missing evidence bridge,
# any parse/lookup/git error, or an ambiguous event allows the action.
# Fail-closed (Go binary, not this host): a `--write` that cannot be recorded
# (validation, lock timeout, ambiguous identity, store IO) exits 2 with no verdict
# JSON. `openspec/**` is never blocked.
#
# Tokens stamped at sync (gitignored project copy):
#   __TRACKER_CARD_GATE_MODE__   (raw legacy gate_mode hint; default warn)
#   __TRACKER_CLI_HOME__         (CLI install home for binary resolution)
#   __TRACKER_LIB_INTERNAL__     (legacy materializer placeholder; not read here)
#
# Config / env (all read by Go's ledger-mode resolver, never by this host):
#   TRACKER_LEDGER_MODE       strongest mode env (always|ask|warn)
#   TRACKER_CARD_GATE_MODE    legacy gate_mode env (off|warn|always)
# Host-only env:
#   TRACKER_CARD_GATE_PATHS   space-separated production dirs
#                             (default: "lib catalog bin src")
#   AI_SPECS_HOME             preferred over stamped CLI home for the binary
#   WORKTREE_GATE_BIN         explicit verified-binary override (debugging/tests)

stamped_gate_mode="__TRACKER_CARD_GATE_MODE__"
stamped_cli_home="__TRACKER_CLI_HOME__"
stamped_lib_internal="__TRACKER_LIB_INTERNAL__"
prod_dirs="${TRACKER_CARD_GATE_PATHS:-lib catalog bin src}"
[ -n "${prod_dirs// /}" ] || prod_dirs="lib catalog bin src"

_tracker_cli_bridge() {
  case "$stamped_cli_home" in
    ""|__*) return 1 ;;
  esac
  [ -x "$stamped_cli_home/dist/ai-specs" ] || return 1
  printf '%s\n' "$stamped_cli_home/dist/ai-specs"
}

# --- Direct host mode (argv-selected; never the stdin hook contract) ----------
# Merge skills/commands grade a Tracker lifecycle checkpoint with an explicit
# host flag:
#   tracker-card-gate.sh --root <project-root> --checkpoint pre-merge|archive-close [slug]
#   tracker-card-gate.sh --root <project-root> --stage pre-merge|pre-archive [slug]
# The mode is selected by argv only, so a piped hook payload can never choose a
# checkpoint, and the stdin contract below is untouched. Like hook mode this is
# acquisition/JSON transport to the verified Go predicate: no predicate, no
# write, no network.
host_mode=0
for _host_arg in "$@"; do
  case "$_host_arg" in --root|--checkpoint|--stage) host_mode=1 ;; esac
done

if [ "$host_mode" -eq 0 ]; then

input="$(cat)"

# Protocol on stdout from `ai-specs tracker classify`:
#   line 1: <kind>\t<tool_name>\t<repo_hint>
#     kind ∈ {path, shell, none}
#   path → line 2: <abs_or_rel_file_path>
#   shell → line 2: <action>\t<details>
#     action ∈ {pr_create}\t<details is empty; archive-close is graded by the
#     tracker ledger host, so archive shell commands are not gated here>
# Fail-open rules unchanged: no CLI, a non-zero `tracker classify` exit, or
# empty output exits 0 with no grading (accepted degradation).
cli="$(_tracker_cli_bridge)" || exit 0
parsed="$(printf '%s' "$input" | "$cli" tracker classify 2>/dev/null)" || exit 0

[ -n "$parsed" ] || exit 0

kind_line="$(printf '%s\n' "$parsed" | head -n 1)"
kind="${kind_line%%$'\t'*}"
rest_kl="${kind_line#*$'\t'}"
tool_name="${rest_kl%%$'\t'*}"
cwd="${rest_kl#*$'\t'}"

fi  # end hook-payload parsing (skipped in direct host mode)
# --- Ledger checkpoint bridge (acquisition + JSON only; no predicate) ---------
# The five ledger checkpoints are graded by the verified Go `--ledger` mode on
# the shared worktree-gate binary (one predicate, one trust root). This host
# only acquires a verified binary, forwards the raw stamped legacy gate hint,
# and maps the JSON verdict; Go resolves the effective mode from env, the
# witness-bound recipe config, that hint and the warn default. A
# missing/unverified binary, a parse error, or an IO failure fails open (exit 0).

# The evidence and classify bridge is the native CLI (`ai-specs tracker
# slug|evidence|classify`) at the stamped CLI home. An empty or unstamped home, or
# a missing/non-executable binary, means the bridge is unavailable, so both the
# event classification and the evidence file are skipped (fail open), exactly
# like a missing/unverified binary. The bridge is acquisition only: no host
# derives a ledger write from a planning-tree artifact (R1).
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
  # stdin: verdict JSON. $1: dotted path in that object. Read through the native
  # `tracker field` verb; without the bridge there is no verdict to report.
  local cli
  cli="$(_tracker_cli_bridge)" || return 1
  "$cli" tracker field "$1" 2>/dev/null
}

_ledger_ask() {
  # $1 bin, $2 checkpoint, $3 root, $4 prefix; stdin: full verdict JSON. The
  # follow-up `--decide` forwards the raw stamped legacy hint so Go re-resolves
  # the same effective mode it graded with (no host-resolved --ledger-mode).
  # Without the bridge the display lines are skipped (accepted degradation) and
  # the opt-out flow below runs exactly as before.
  local bin="$1" checkpoint="$2" root="$3" prefix="$4"
  local payload side value cli
  payload="$(cat)"
  if cli="$(_tracker_cli_bridge)"; then
    for side in local remote code git; do
      value="$(printf '%s' "$payload" | "$cli" tracker field "prompt.evidence.$side" 2>/dev/null)"
      printf '  %s: %s\n' "$side" "${value:-(unavailable)}" >&2
    done
    value="$(printf '%s' "$payload" | "$cli" tracker field prompt.choices 2>/dev/null)"
    printf '  choices: %s\n' "$value" >&2
  fi
  if ! { exec 3</dev/tty; } 2>/dev/null; then
    echo "${prefix}: ${checkpoint} needs a decision but no terminal is available; no opt-out was recorded — Tracker advisory, continuing without blocking." >&2
    return 0
  fi
  printf '%s: opt out of the %s checkpoint? [y/N] ' "$prefix" "$checkpoint" >&2
  local answer=""
  read -r answer <&3 || answer=""
  exec 3<&-
  case "$answer" in
    y|Y|yes|YES|Yes)
      if "$bin" --ledger --checkpoint "$checkpoint" --ledger-gate-mode "$stamped_gate_mode" \
          --project-root "$root" \
          --decide "{\"checkpoint\":\"$checkpoint\",\"kind\":\"opt-out\",\"choice\":\"continue\"}" >/dev/null 2>&1; then
        echo "${prefix}: opt-out recorded for ${checkpoint}; proceeding." >&2
        return 0
      fi
      echo "${prefix}: failed to record the ${checkpoint} opt-out — Tracker advisory, continuing without blocking." >&2
      return 0
      ;;
  esac
  echo "${prefix}: no opt-out recorded for ${checkpoint} — Tracker advisory, continuing without blocking." >&2
  return 0
}

_ledger_grade() {
  # $1 checkpoint, $2 root, $3 prefix, $4 explicit change slug (optional).
  # Always returns 0: the Tracker host is advisory, so a block/ask verdict is
  # reported and never blocks. A caller-supplied slug scopes evidence to the
  # change being graded (archive-close and pre-merge know it); without one the
  # single active change is used, and an ambiguous planning tree contributes no
  # evidence. The raw stamped legacy hint is forwarded and Go resolves the mode.
  local checkpoint="$1" root="$2" prefix="$3" explicit_slug="${4:-}"
  local bin
  if ! bin="$(_ledger_binary)"; then
    # Missing/unverified binary fails open rather than blocking a merge; say so,
    # because a silent skip looks identical to a passing checkpoint.
    echo "${prefix}: tracker-ledger: no verified gate binary resolved; failing open (run ai-specs sync / ai-specs doctor)" >&2
    return 0
  fi
  local extra=() evidence="" cli=""
  if cli="$(_tracker_cli_bridge)"; then
    local slug
    # R1: the host never converts an artifact into a ledger write. A planning-tree
    # tracker.none file is presentation and evidence only; recording the exemption is
    # an explicit `--write kind=exempt` made outside this host, never a side effect of
    # grading a checkpoint. The file is never created, modified, or deleted here.
    slug="$explicit_slug"
    [ -n "$slug" ] || slug="$("$cli" tracker slug "$root" 2>/dev/null)"
    evidence="$(mktemp "${TMPDIR:-/tmp}/tracker-ledger-evidence.XXXXXX" 2>/dev/null)"
    if [ -n "$evidence" ]; then
      if "$cli" tracker evidence "$root" "$slug" "$evidence" 2>/dev/null; then
        extra=(--evidence "$evidence")
      else
        rm -f "$evidence"; evidence=""
      fi
    fi
  fi
  local out rc
  out="$("$bin" --ledger --checkpoint "$checkpoint" --ledger-gate-mode "$stamped_gate_mode" \
      --project-root "$root" "${extra[@]}" 2>/dev/null)"
  rc=$?
  [ -z "$evidence" ] || rm -f "$evidence"
  [ -n "$out" ] || return 0
  local decision reason active
  decision="$(printf '%s' "$out" | _ledger_field decision)" || return 0
  [ -n "$decision" ] || return 0
  reason="$(printf '%s' "$out" | _ledger_field reason)"
  active="$(printf '%s' "$out" | _ledger_field active)"
  if [ "$rc" = 2 ] || [ "$decision" = block ]; then
    echo "${prefix}: blocked at ${checkpoint} — ${reason:-missing tracked item} (Tracker advisory: reported, not enforced)." >&2
    return 0
  fi
  if [ "$decision" = ask ] && [ "$reason" = identity_unavailable ]; then
    # No identity means no durable key for a checkpoint-scoped answer (A2/A5), so
    # ask cannot collect a recordable decision here: report and proceed without
    # inferring one (the spec's non-blocking identity_unavailable rule).
    echo "${prefix}: ${checkpoint} cannot be keyed (identity_unavailable); reporting and proceeding without recording a decision." >&2
    return 0
  fi
  if [ "$decision" = ask ]; then
    printf '%s' "$out" | _ledger_ask "$bin" "$checkpoint" "$root" "$prefix"
    return $?
  fi
  if [ "$active" = 1 ] && [ "$decision" != allow ]; then
    echo "${prefix}: ${decision} at ${checkpoint} — ${reason:-no primary item} (Tracker advisory)." >&2
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

# --- Bash relpath (os.path.relpath equivalence, no python) --------------------
# Resolve the leaf's symlinks with a readlink loop, canonicalize the deepest
# existing ancestor with `cd -P` + `pwd -P`, re-append the non-existing tail, and
# strip the canonical root prefix. Bash 3.2: no associative arrays, no ${x,,},
# no readlink -f. A target outside the root yields `..`-prefixed output.
# Lexical `.`/`..` collapse of an absolute path, as realpath does for the parts that
# do not exist (R1-001). Stack-based; a `..` above the root stays at the root.
_collapse_dots() {
  local part n=0 i out=""
  local -a parts stack
  IFS=/ read -r -a parts <<< "$1"
  for part in "${parts[@]}"; do
    case "$part" in
      ""|.) ;;
      ..) [ "$n" -gt 0 ] && n=$((n - 1)) ;;
      *) stack[$n]="$part"; n=$((n + 1)) ;;
    esac
  done
  i=0
  while [ "$i" -lt "$n" ]; do
    out="$out/${stack[$i]}"
    i=$((i + 1))
  done
  printf '%s\n' "${out:-/}"
}

_canonical_path() {
  local path="$1" hops=0 target dir tail=""
  while [ -L "$path" ] && [ "$hops" -lt 40 ]; do
    target="$(readlink "$path" 2>/dev/null)" || break
    case "$target" in
      /*) path="$target" ;;
      *) path="$(dirname "$path")/$target" ;;
    esac
    hops=$((hops + 1))
  done
  dir="$path"
  while [ -n "$dir" ] && [ "$dir" != "/" ] && [ ! -d "$dir" ]; do
    tail="/$(basename "$dir")$tail"
    dir="$(dirname "$dir")"
  done
  [ -n "$dir" ] && [ -d "$dir" ] || return 1
  dir="$(cd -P "$dir" 2>/dev/null && pwd -P)" || return 1
  [ -n "$dir" ] || return 1
  _collapse_dots "${dir%/}$tail"
}

_relative_to() {
  local target="$1" base="$2"
  [ "$target" = "$base" ] && { printf '.\n'; return 0; }
  local out="" i=0 j
  local tparts bparts
  # read -a splits without pathname expansion, so a `*` in a path stays literal.
  IFS=/ read -r -a tparts <<< "${target#/}"
  IFS=/ read -r -a bparts <<< "${base#/}"
  while [ "$i" -lt "${#tparts[@]}" ] && [ "$i" -lt "${#bparts[@]}" ]; do
    [ "${tparts[$i]}" = "${bparts[$i]}" ] || break
    i=$((i + 1))
  done
  j=$i
  while [ "$j" -lt "${#bparts[@]}" ]; do
    out="${out}../"
    j=$((j + 1))
  done
  j=$i
  while [ "$j" -lt "${#tparts[@]}" ]; do
    out="${out}${tparts[$j]}"
    j=$((j + 1))
    [ "$j" -lt "${#tparts[@]}" ] && out="${out}/"
  done
  [ -n "$out" ] || out="."
  case "$out" in */) out="${out%/}" ;; esac
  printf '%s\n' "$out"
}

_repo_relative() {
  local target base
  target="$(_canonical_path "$1")" || return 1
  base="$(_canonical_path "$2")" || return 1
  [ -n "$target" ] && [ -n "$base" ] || return 1
  _relative_to "$target" "$base"
}

# --- Direct host mode dispatch (CLI lifecycle surface) ------------------------
# `--root` is required and is resolved to the owning git work tree, never assumed
# from the process cwd. The requested checkpoint always decides: an entry under
# `openspec/changes/archive/` never selects `archive-close`, which is tracker item
# closure, not an OpenSpec archive. Nothing is written or inferred here.
_host_usage() {
  echo "usage: tracker-card-gate.sh --root <project-root> [--checkpoint pre-merge|archive-close | --stage pre-merge|pre-archive] [slug]" >&2
}

tracker_host() {
  local root="" checkpoint="" stage="" slug=""
  while [ $# -gt 0 ]; do
    local arg="$1"
    shift
    case "$arg" in
      --root) root="${1:-}"; [ $# -gt 0 ] && shift ;;
      --checkpoint) checkpoint="${1:-}"; [ $# -gt 0 ] && shift ;;
      --stage) stage="${1:-}"; [ $# -gt 0 ] && shift ;;
      -h|--help) _host_usage; return 0 ;;
      --*) echo "tracker-card-gate: unknown option '${arg}'" >&2; _host_usage; return 2 ;;
      *)
        if [ -n "$slug" ]; then
          echo "tracker-card-gate: unexpected argument '${arg}'" >&2
          _host_usage
          return 2
        fi
        slug="$arg"
        ;;
    esac
  done
  if [ -z "$root" ]; then
    echo "tracker-card-gate: --root is required for the direct host mode" >&2
    _host_usage
    return 2
  fi
  local from_stage=""
  case "$stage" in
    "") ;;
    pre-archive) from_stage="archive-close" ;;
    pre-merge) from_stage="pre-merge" ;;
    *) echo "tracker-card-gate: invalid --stage '${stage}'" >&2; _host_usage; return 2 ;;
  esac
  case "$checkpoint" in
    ""|pre-merge|archive-close) ;;
    *) echo "tracker-card-gate: invalid --checkpoint '${checkpoint}'" >&2; _host_usage; return 2 ;;
  esac
  if [ -n "$checkpoint" ] && [ -n "$from_stage" ] && [ "$checkpoint" != "$from_stage" ]; then
    echo "tracker-card-gate: --checkpoint ${checkpoint} conflicts with --stage ${stage}" >&2
    _host_usage
    return 2
  fi
  [ -n "$checkpoint" ] || checkpoint="${from_stage:-pre-merge}"
  local repo_root
  repo_root="$(_resolve_repo "$root")" || {
    echo "tracker-card-gate: no git work tree at '${root}'; failing open" >&2
    return 0
  }
  _ledger_grade "$checkpoint" "$repo_root" "tracker-card-gate" "$slug"
}

if [ "$host_mode" -eq 1 ]; then
  tracker_host "$@"
  exit $?
fi

if [ "$kind" = path ]; then
  file_path="$(printf '%s\n' "$parsed" | sed -n '2p')"
  [ -n "$file_path" ] || exit 0
  case "$file_path" in
    /*) abs="$file_path" ;;
    *)  abs="${cwd:-$PWD}/$file_path" ;;
  esac
  repo_root="$(_resolve_repo "$abs")" || exit 0
  [ -n "$repo_root" ] || exit 0

  rel="$(_repo_relative "$abs" "$repo_root")" || exit 0
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

  _ledger_grade "apply-start" "$repo_root" "tracker-card-gate"
  exit 0
fi

if [ "$kind" = shell ]; then
  repo_root="$(_resolve_repo "${cwd:-$PWD}")" || exit 0
  [ -n "$repo_root" ] || exit 0
  while IFS=$'\t' read -r action _detail; do
    [ -n "$action" ] || continue
    case "$action" in
      pr_create)
        _ledger_grade "pr-review" "$repo_root" "tracker-card-gate"
        ;;
      *)
        ;;
    esac
  done < <(printf '%s\n' "$parsed" | tail -n +2)
  exit 0
fi

exit 0
