"""Differential parity harness: FROZEN LEGACY GOLDENS vs the Go single-binary.

Card [Go 03] — Trello 6a84e77110b9ac751af676f4.

Purpose
-------
Run the SAME fixture through TWO implementations of the ai-specs CLI —
the legacy Bash/Python implementation, frozen as the versioned goldens under
``goldens/`` before the launcher flip (GO-20 U2e T2a), and the Go root-module
binary (``cmd/ai-specs``, card 04) — and diff:

  * the FULL emitted project file tree (paths, contents via sha256, file modes,
    symlink targets),
  * the exit code of every step, and
  * normalized stdout/stderr,

reporting any delta as a readable per-fixture diff. The harness MEASURES
behavior; it never judges it. Behavior classified FROZEN in
``docs/go-migration-parity-contract.md`` is measured verbatim; the harness
never "fixes" it.

Design constraints (from the card):
  * zero third-party dependencies (stdlib only, like the rest of the suite);
  * hermetic and offline (``AI_SPECS_NO_NETWORK=1``, ``AI_SPECS_GATE_OFFLINE=1``,
    isolated ``HOME``/``TMPDIR``/``AI_SPECS_HOME`` per leg);
  * reuses ``tests/_blackbox.py`` helpers (install-root isolation, capture
    conventions) instead of reimplementing them;
  * the Go build side never silently substitutes a legacy launcher in
    the wired CI phase: run.py REQUIRES the build and fails loudly (exit 2)
    when go is absent or the build fails; the "identical-by-shim" mode
    (both legs run the same launcher, must report ZERO deltas, acceptance a)
    exists ONLY as run.py's explicit ``--self-test`` flag, never as a silent
    fallback. Its historical mode id is ``legacy-vs-legacy``; since the
    GO-20 U2e flip ``bin/ai-specs`` IS the native launcher, so that id names
    the mode, not a legacy execution (see the note run_corpus prints);
  * the corpus runs in TWO gate modes (plan finding F3 / risk R7):
    ``gate-absent`` (no verified gate binary in the isolated home — the mode
    the legacy goldens were captured in) and ``gate-present`` (a locally built
    worktree-gate pinned via ``WORKTREE_GATE_BIN`` for the live leg, exercising
    the Go-authority path). The mode is recorded in the report, never ambient;
    the built binary stays in the temp dir and is never committed.

The known help-heredoc quirk (card 04 finding) and every normalization rule
carry a written justification in NORMALIZATIONS below; nothing is silently
regex-silenced.
"""
from __future__ import annotations

import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
from collections.abc import Callable
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "tests"))
import _blackbox as bb  # noqa: E402  (reuse suite helpers; never lib/_internal)

# Same env contract as tests/run.sh: the sync binding path must stay
# network-hermetic; no fixture may reach the network.
BASE_ENV = {
    "PATH": os.environ.get("PATH", ""),
    "AI_SPECS_NO_NETWORK": "1",
    "AI_SPECS_GATE_OFFLINE": "1",
    "LC_ALL": "C",
    "LANG": "C",
    # Per-step strangler selection (GO-07). Forwarded from the operator's
    # environment so the corpus can be run with GO_SYNC_STEP_GITIGNORE=go (and
    # friends): the frozen legacy golden does not react to these vars, while the
    # live Go spine routes the named step through its native implementation.
    **{k: v for k, v in os.environ.items() if k.startswith("GO_SYNC_STEP_")},
}

# Gate modes: whether the corpus runs with a locally built worktree-gate binary
# pinned via WORKTREE_GATE_BIN for the live leg. `gate-absent` is the historical
# behavior (isolated home with an empty cache/ and no binary), and it is the mode
# the legacy goldens were captured in, when the legacy Python bridges degraded to
# their own fallback; `gate-present` builds the nested worktree-flow gate module
# once and exercises the real Go-authority path (plan finding F3 / risk R7).
# AI_SPECS_GATE_OFFLINE stays set in both modes: it only prevents NETWORK
# acquisition, never a pinned local binary.
GATE_ABSENT = "gate-absent"
GATE_PRESENT = "gate-present"
GATE_MODULE_DIR = ROOT / "catalog" / "recipes" / "worktree-flow" / "gate"


# ── Normalization registry ────────────────────────────────────────────────
# Each rule is a named, justified transformation applied identically to BOTH
# legs. No blanket silencing: rules are anchored to specific surfaces.


@dataclass(frozen=True)
class Normalization:
    rule_id: str
    surfaces: tuple[str, ...]          # "stdout" | "stderr" | "tree"
    description: str
    justification: str
    fn: Callable[[str, dict[str, Any]], str]


# Stable anchors of the help text around the volatile region (both legs emit
# these byte-identically; see internal/cli/help.txt — a byte-exact extraction
# of the legacy heredoc).
_HELP_HUB_PREFIX = "  hub [path] Interactive status + command menu (also bare "
_HELP_TAIL_ANCHOR = "\n  help Show this help"


def _norm_help_cmdsubst(text: str, ctx: dict) -> str:
    start = text.find(_HELP_HUB_PREFIX)
    if start < 0:
        return text
    body_end = text.find(_HELP_TAIL_ANCHOR, start)
    if body_end < 0:
        return text
    return (
        text[:start]
        + _HELP_HUB_PREFIX
        + "<HELP_CMD_SUBST>"
        + text[body_end:]
    )


def _norm_temp_paths(text: str, ctx: dict) -> str:
    for key in ("project_root", "home", "scratch"):
        p = ctx.get(key)
        if p:
            text = text.replace(str(p), "<TEMP>")
    # macOS temp dirs may surface as /private/var/... aliases of /var/...
    return re.sub(r"/(?:private/)?var/folders/[^\s\"']+", "<TEMP>", text)


# Anchored to the CLI's own tempfile prefixes (never a generic pattern);
# '_' included because Python tempfile's random charset contains it.
_RANDOM_TEMPFILE_RE = re.compile(r"(ai-specs-(?:recipe-mcp|vendor)-)[A-Za-z0-9_]+")


def _norm_random_tempfile(text: str, ctx: dict) -> str:
    return _RANDOM_TEMPFILE_RE.sub(r"\1<RANDOM>", text)


# The per-project cache key is a hash of the project's absolute path; each
# leg's project lives under its own scratch root, so the key differs per leg
# and is a harness artifact, not implementation behavior.
_CACHE_PROJECT_KEY_RE = re.compile(r"(cache/projects/)[0-9a-f]+(-project)")


def _norm_cache_project_key(text: str, ctx: dict) -> str:
    return _CACHE_PROJECT_KEY_RE.sub(r"\1<PROJECT_KEY>\2", text)


# ISO-8601 wall-clock instants (lock synced_at, ledger written_at, ...).
_ISO_TS_RE = re.compile(
    r"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?")


def _norm_iso_timestamps(text: str, ctx: dict) -> str:
    return _ISO_TS_RE.sub("<ISO_TS>", text)


_HELP_STDERR_SUBST = "<HELP_CMD_SUBST_STDERR>\n"


def _norm_help_stderr(text: str, ctx: dict) -> str:
    if ctx.get("argv") and ctx["argv"][0] == "help":
        return _HELP_STDERR_SUBST
    return text


# A failed `git clone` in a [[deps]] source. The legacy vendor-skills.py lets
# the CalledProcessError escape as an uncaught Python traceback after git's own
# `fatal:` line; the native port prints git's `fatal:` line followed by one
# `  ✗ exit status N` line. Both carry the same cause and the same exit code
# (compared separately). These two regexes remove only the wrapper text of
# that exact shape, and only on the `sync` verb (see _norm_vendor_clone_failure).
_VENDOR_CLONE_TRACEBACK_RE = re.compile(
    r"Traceback \(most recent call last\):\n"
    r"(?:(?!subprocess\.CalledProcessError:).*\n)*"
    r"subprocess\.CalledProcessError: Command '\[['\"]git['\"], ['\"]clone['\"][^\n]*\n")
_VENDOR_CLONE_STATUS_RE = re.compile(
    r"^(fatal: [^\n]*\n)  ✗ exit status \d+\n", re.MULTILINE)


def _norm_vendor_clone_failure(text: str, ctx: dict) -> str:
    if not (ctx.get("argv") and ctx["argv"][0] == "sync"):
        return text
    text = _VENDOR_CLONE_TRACEBACK_RE.sub("", text)
    return _VENDOR_CLONE_STATUS_RE.sub(r"\1", text)


# GO-20 U2e T7 (Robert's decision): the generated Cursor deny wrapper no longer
# invokes python3. The FROZEN legacy goldens still carry the python3 deny line
# (captured from lib/_internal/hooks-render.py before the flip); the Go leg emits
# the POSIX awk line. This rule canonicalizes EXACTLY those two lines, on BOTH
# legs, to one marker. Exact-string match only.
# EXPIRES with the frozen corpus, not with lib/: measured in GO-20 U2e T5 PR4a
# S7b, dropping N10 fails cache-layout and hooks-five-runtimes in both gate modes
# because the golden tree hash was computed over the canonicalized wrapper.
# Delete only together with a corpus recapture (PR4b).
_T7_LEGACY_DENY_LINE = r"""  printf '{"permission":"deny","agent_message":%s}' "$(printf %s "$out" | python3 -c 'import json,sys;print(json.dumps(sys.stdin.read()))')"
"""
_T7_GO_DENY_LINE = r"""  printf '{"permission":"deny","agent_message":"%s"}' "$(printf %s "$out" | LC_ALL=C awk 'BEGIN{esc["\\"]="\\\\";esc["\""]="\\\"";esc["\r"]="\\r";esc["\t"]="\\t";esc["\b"]="\\b";esc["\f"]="\\f";for(i=1;i<32;i++){c=sprintf("%c",i);if(!(c in esc))esc[c]=sprintf("\\u%04x",i)}} {line=$0;if(NR>1)printf "%s","\\n";for(i=1;i<=length(line);i++){c=substr(line,i,1);printf "%s",((c in esc)?esc[c]:c)}}')"
"""
_T7_DENY_MARKER = "<T7-CURSOR-DENY-LINE>"


def _norm_t7_cursor_deny_line(text: str, ctx: dict) -> str:
    text = text.replace(_T7_LEGACY_DENY_LINE, _T7_DENY_MARKER + "\n")
    return text.replace(_T7_GO_DENY_LINE, _T7_DENY_MARKER + "\n")


# GO-20 U2e T5 (Robert's decision, 2026-11-13): the doctor worktree-gate line
# reports the byte total of the cached gate binary
# (internal/doctor/gatebinary.go gateCacheSize), which moves with EVERY legitimate
# gate-binary change. This slice is the first gate-source change since the legacy
# goldens were frozen (276fa856, T2a), so the frozen 3115 KiB no longer matches the
# live 3261 KiB for a reason that is not behaviour at all. Anchored to the exact
# `; selftest passed (cache <N> KiB)` suffix of that ONE line: the line's identity,
# label and selftest text stay compared verbatim, and no other doctor field is
# touched. Unlike N10/N11 this never expires with the corpus — a byte count is a
# permanently unstable pin.
_GATE_CACHE_KB_RE = re.compile(r"(; selftest passed \(cache )\d+( KiB\))")


def _norm_gate_cache_size(text: str, ctx: dict) -> str:
    if (ctx.get("argv") or ())[:1] != ("doctor",):
        return text
    return _GATE_CACHE_KB_RE.sub(r"\1<CACHE_KB>\2", text)


# GO-20 U2e T5 (v0.25.0 merge): CLI version stamps move with every release.
# The frozen goldens were captured at 0.24.0; after the bump the live legs
# legitimately print 0.25.0 in doctor lines, the version verb, the hub banner
# and lock stamps. Same class as N12: a permanent instability, never expires
# with a corpus recapture.
_VERSION_STAMP_CONTEXTUAL_RES = (
    (re.compile(r"installed \d+\.\d+\.\d+"), "installed <CLI_VERSION>"),
    (re.compile(r"last sync \d+\.\d+\.\d+"), "last sync <CLI_VERSION>"),
    (re.compile(r"version: \d+\.\d+\.\d+"), "version: <CLI_VERSION>"),
    (re.compile(r'cli_version = "\d+\.\d+\.\d+"'), 'cli_version = "<CLI_VERSION>"'),
    (re.compile(r'"cli_version": "\d+\.\d+\.\d+"'), '"cli_version": "<CLI_VERSION>"'),
    (re.compile(r'stamped_gate_version="\d+\.\d+\.\d+"'), 'stamped_gate_version="<CLI_VERSION>"'),
)


def _norm_version_stamps(text: str, ctx: dict) -> str:
    for pat, repl in _VERSION_STAMP_CONTEXTUAL_RES:
        text = pat.sub(repl, text)
    return text


def _norm_bare_version_line(text: str, ctx: dict) -> str:
    # The `version` verb prints as "<semver>\n" (surface-verbs).
    return re.sub(r"(?m)^0\.\d+\.\d+$", "<CLI_VERSION>", text)


# GO-20 U2e T4: scoped to add-dep and skills add; expires with the frozen corpus
# (see N11), not with lib/ alone.
def _norm_vendor_add_clone_failure(text: str, ctx: dict) -> str:
    argv = ctx.get("argv") or ()
    if not (argv[:1] == ("add-dep",) or argv[:2] == ("skills", "add")):
        return text
    text = _VENDOR_CLONE_TRACEBACK_RE.sub("", text)
    return _VENDOR_CLONE_STATUS_RE.sub(r"\1", text)


# N7 (legacy bridge-fallback notice) and N8 (legacy hook-bridge reconcile
# notice) were REMOVED in GO-20 U2e T5 PR4a S7b: they stripped announcements only
# the legacy Python modules ever printed, the frozen goldens were captured with
# them already stripped, and no live legacy leg is left to strip anything from.
# The rule ids below keep their numbers so external references stay stable.
NORMALIZATIONS = (
    Normalization(
        rule_id="N1-temp-paths",
        surfaces=("stdout", "stderr", "tree"),
        description="Replace the per-leg temp roots (project, install home, "
                    "scratch dir) and macOS /var/folders aliases with <TEMP>.",
        justification="Each leg runs in its OWN throwaway roots (the runner "
                      "isolates HOME/TMPDIR/AI_SPECS_HOME per leg). Absolute "
                      "temp paths are run artifacts of the harness, not "
                      "implementation behavior; the existing suite's "
                      "_blackbox.normalize_output applies the same rule.",
        fn=_norm_temp_paths,
    ),
    Normalization(
        rule_id="N2-help-heredoc-cmdsubst",
        surfaces=("stdout",),
        description="Collapse the volatile region of `ai-specs help` between "
                    "the hub line prefix and the '  help Show this help' "
                    "anchor into <HELP_CMD_SUBST>.",
        justification="KNOWN QUIRK (card 04 finding): the legacy help is an "
                      "UNQUOTED bash heredoc (cat <<EOF), so the backticked "
                      "`ai-specs` inside the hub line is command-substituted "
                      "AT RUNTIME with whatever binary is first on PATH — the "
                      "substituted bytes are the full non-TTY hub status "
                      "output of that unrelated binary (multi-line, and "
                      "dependent on the machine's global install, PATH order, "
                      "and cwd project state). The Go side (card 04) prints "
                      "the literal backticked name instead. The substituted "
                      "region is defined by the ENVIRONMENT, not by the "
                      "implementation under test, so the harness collapses it "
                      "on both sides. Measured live: with this repo on PATH "
                      "the substitution injected 19 lines of hub output; with "
                      "no ai-specs on PATH it injects nothing plus a 'command "
                      "not found' on stderr. (The stderr channel of the same "
                      "quirk is handled by N6.)",
        fn=_norm_help_cmdsubst,
    ),
    Normalization(
        rule_id="N3-harness-tempfile-names",
        surfaces=("stdout", "stderr", "tree"),
        description="Collapse the random suffix of CLI tempfiles named "
                    "ai-specs-recipe-mcp-* and ai-specs-vendor-* into <RANDOM>.",
        justification="These names are minted by tempfile per process run; "
                      "the random suffix is a run artifact, not behavior. "
                      "Anchored to the two exact CLI tempfile prefixes — never "
                      "a generic pattern. Observed live: RECIPE_MCP_TEMP stdout "
                      "line (fresh-init) and the vendor traceback path "
                      "(missing-optional-deps).",
        fn=_norm_random_tempfile,
    ),
    Normalization(
        rule_id="N4-cache-project-key",
        surfaces=("stdout", "stderr", "tree"),
        description="Collapse the per-project cache key hash in "
                    "cache/projects/<hex>-project into <PROJECT_KEY>.",
        justification="The resolved-skills cache key is a hash of the "
                      "project's absolute path; each leg runs in its own "
                      "scratch root, so the key differs between legs purely "
                      "because of where the harness placed the project. "
                      "Anchored to the exact cache/projects/ path shape.",
        fn=_norm_cache_project_key,
    ),
    Normalization(
        rule_id="N5-iso-timestamps",
        surfaces=("stdout", "stderr", "tree"),
        description="Replace ISO-8601 timestamps with <ISO_TS>.",
        justification="Wall-clock instants (lock synced_at, ledger witness "
                      "written_at) record WHEN a leg ran, not what it did; two "
                      "identical implementations seconds apart must compare "
                      "equal. Anchored to the ISO-8601 shape, never blanket.",
        fn=_norm_iso_timestamps,
    ),
    Normalization(
        rule_id="N6-help-stderr-cmdsubst",
        surfaces=("stderr",),
        description="For the `help` verb only, replace the ENTIRE stderr with "
                    "the constant '<HELP_CMD_SUBST_STDERR>\n'.",
        justification="Extension of the N2 quirk to the stderr channel: the "
                      "legacy help is an unquoted heredoc whose backticked "
                      "`ai-specs` is command-substituted at runtime; the "
                      "substituted child SHARES the parent's stderr, so legacy "
                      "help stderr carries whatever the PATH-resolved binary "
                      "prints (measured live: GO_*_BRIDGE_FALLBACK warnings "
                      "from this machine's global install) or bash's own "
                      "'command not found' when nothing is on PATH. The legacy "
                      "implementation itself writes nothing of its own to help "
                      "stderr, and the Go side has no substitution channel, so "
                      "the whole surface is environment-defined. Replacement "
                      "is deliberately UNCONDITIONAL (not only when nonempty) "
                      "so both legs normalize to the same constant; it is "
                      "scoped to argv[0]=='help' only — every other verb's "
                      "stderr is compared verbatim. Tradeoff (reviewer-noted, "
                      "accepted deliberately): the constant would mask a "
                      "hypothetical future Go-side help stderr channel; "
                      "revisit N6 if the Go implementation ever gains native "
                      "help-stderr output.",
        fn=_norm_help_stderr,
    ),
    Normalization(
        rule_id="N9-vendor-clone-failure-wrapper",
        surfaces=("stderr",),
        description="For the `sync` verb only, remove the legacy uncaught "
                    "CalledProcessError traceback of a failed `git clone` in a "
                    "[[deps]] source, and the native `  ✗ exit status N` line "
                    "that follows git's `fatal:` line. Both legs keep git's "
                    "`fatal:` line; the exit code is compared verbatim.",
        justification="Accepted divergence, same class as the L6/D2 entries: "
                      "vendor-skills.py crashes with an uncaught traceback on a "
                      "clone failure, while the native port reports git's own "
                      "`fatal:` line plus one status line and exits with the "
                      "same code (measured: missing-optional-deps and "
                      "deps-gitignore, both gate modes). The traceback is a "
                      "process artifact of an unhandled exception, not behaviour "
                      "a user relies on, so it is removed on the legacy side "
                      "only where it is exactly that shape, and the native status "
                      "line only where it follows a `fatal:` line. Nothing else "
                      "on stderr is touched, and the scope is argv[0]=='sync'.",
        fn=_norm_vendor_clone_failure,
    ),
    Normalization(
        rule_id="N10-t7-cursor-deny-line",
        surfaces=("tree",),
        description="Canonicalize the Cursor deny line of generated "
                    ".cursor/hooks/*.sh wrappers (exact match of the legacy "
                    "python3 line and of the Go awk line) to one marker, on "
                    "both legs.",
        justification="Intentional divergence, GO-20 U2e T7, Robert's decision: "
                      "the generated Cursor wrapper must not invoke python3. The "
                      "FROZEN LEGACY GOLDEN still carries it (captured from "
                      "lib/_internal/hooks-render.py before the flip) and the "
                      "native port emits the POSIX awk escaper. Only those two "
                      "exact lines change; the rest of each wrapper is compared "
                      "byte for byte. It expires with the frozen corpus, not with "
                      "lib/: GO-20 U2e T5 PR4a S7b measured that dropping it "
                      "fails cache-layout and hooks-five-runtimes in both gate "
                      "modes, because the golden tree hash was computed over the "
                      "canonicalized wrapper. Delete it only with a corpus "
                      "recapture (PR4b).",
        fn=_norm_t7_cursor_deny_line,
    ),
    Normalization(
        rule_id="N11-vendor-add-clone-failure",
        surfaces=("stderr",),
        description="For the `add-dep` and `skills add` verbs only, remove the "
                    "legacy uncaught CalledProcessError traceback of a failed "
                    "`git clone` in a vendored skill source, and the native "
                    "`  ✗ exit status N` line after git's `fatal:` line. Both "
                    "legs keep git's `fatal:` line; the exit code is compared "
                    "verbatim. Expires at T5 with a frozen-corpus recapture "
                    "(PR4b), not with lib/ alone.",
        justification="Same accepted divergence class as N9: vendor-skills.py "
                      "crashes with an uncaught traceback on a clone failure, "
                      "while the native port reports git's `fatal:` line plus "
                      "one status line and the same exit code. N9 is scoped to "
                      "argv[0]=='sync' and is NOT widened; this rule covers "
                      "only the two verbs named above. It expires with the "
                      "frozen corpus, not with lib/ alone: GO-20 U2e T5 PR4a "
                      "S7b measured that dropping it fails "
                      "vendor-add-clone-failure in both gate modes, because the "
                      "golden stderr was stored with the wrapper already "
                      "removed. Delete it only with a corpus recapture (PR4b).",
        fn=_norm_vendor_add_clone_failure,
    ),
    Normalization(
        rule_id="N12-gate-cache-size",
        surfaces=("stdout",),
        description="For the `doctor` verb only, replace the KiB number in the "
                    "worktree-gate line's `; selftest passed (cache <N> KiB)` "
                    "suffix with <CACHE_KB>. The line's identity, label and "
                    "selftest text are compared verbatim, and no other doctor "
                    "field is touched.",
        justification="GO-20 U2e T5, Robert's decision (2026-11-13): that number "
                      "is the byte total of the cached worktree-gate binary "
                      "(internal/doctor/gatebinary.go:68 gateCacheSize), so it "
                      "moves with EVERY legitimate gate-binary change — no gate "
                      "source had changed since the goldens were frozen at "
                      "276fa856 (T2a), which is why the pin had never moved "
                      "before. It is a permanent instability of the metric, not "
                      "an expiring divergence: unlike N10/N11 it must NOT be "
                      "deleted with a corpus recapture, because the next gate "
                      "change moves the number again. Measured live: golden "
                      "3115 KiB vs live 3261 KiB, the only delta in "
                      "doctor-healthy in both gate modes after the gate's writer "
                      "and manifest TOML reads moved in process.",
        fn=_norm_gate_cache_size,
    ),
    Normalization(
        rule_id="N13-version-stamps",
        surfaces=("stdout", "stderr", "tree"),
        description="Canonicalize CLI version stamps (doctor 'installed'/'last "
                    "sync' lines, the hub 'version:' banner, and lock "
                    "cli_version stamps in both TOML and JSON shapes) to "
                    "<CLI_VERSION> on both legs.",
        justification="GO-20 U2e T5 (v0.25.0 release merge), Robert's approved "
                      "release model: the frozen goldens were captured at "
                      "0.24.0 and every version bump legitimately moves these "
                      "stamps in the live legs. Same permanent-instability class "
                      "as N12 (binary-derived values): it must NOT be deleted "
                      "with a corpus recapture, because the next release moves "
                      "the stamps again. No other version-shaped literal is "
                      "touched.",
        fn=_norm_version_stamps,
    ),
    Normalization(
        rule_id="N13b-bare-version-line",
        surfaces=("stdout",),
        description="Canonicalize a stdout that is exactly one semver line "
                    "(the `version` verb) to <CLI_VERSION> on both legs.",
        justification="Same class as N13-version-stamps: the bare `version` "
                      "verb output moves with every release (surface-verbs "
                      "fixture). Scoped to a whole-line exact semver so no "
                      "embedded literal is touched.",
        fn=_norm_bare_version_line,
    ),
)


def apply_normalizations(surface: str, text: str, ctx: dict) -> str:
    for n in NORMALIZATIONS:
        if surface in n.surfaces:
            text = n.fn(text, ctx)
    return text


# ── Tree snapshot (paths, contents, modes) ────────────────────────────────


def snapshot_tree(root: Path, ctx: dict) -> dict:
    """Map rel path -> (kind, mode, payload) with symlink targets normalized.

    payload: "" for dirs, sha256 hex for regular files, link target for
    symlinks (N1-normalized: targets may legitimately point into the
    per-leg install home, e.g. skill symlinks into the cache).

    Regular files that decode as UTF-8 are normalized through
    apply_normalizations("tree", ...) BEFORE hashing, applied identically to
    both legs: generated file CONTENTS may embed harness artifacts (per-leg
    temp roots, per-run tempfile names, wall-clock timestamps) whose bytes
    differ between legs without any behavioral difference. Binary files are
    hashed verbatim. .git/index and .git/logs/ are excluded: the index stat
    cache (mtime/ino) and reflog wall-clock entries are filesystem run
    artifacts of git itself, not CLI behavior; pinned git dates keep commits,
    refs and objects deterministic instead.
    """
    result: dict[str, tuple[str, str, str]] = {}
    for path in sorted(root.rglob("*")):
        rel = str(path.relative_to(root))
        if rel == ".git/index" or rel.startswith(".git/logs/"):
            continue
        mode = oct(path.lstat().st_mode & 0o7777)
        if path.is_symlink():
            target = apply_normalizations(
                "tree", os.readlink(path), ctx)
            result[rel] = ("symlink", mode, target)
        elif path.is_dir():
            result[rel] = ("dir", mode, "")
        elif path.is_file():
            raw = path.read_bytes()
            try:
                text = raw.decode("utf-8")
            except UnicodeDecodeError:
                digest = hashlib.sha256(raw).hexdigest()
            else:
                digest = hashlib.sha256(
                    apply_normalizations("tree", text, ctx)
                    .encode("utf-8")).hexdigest()
            result[rel] = ("file", mode, digest)
    return result


# ── Fixture corpus ────────────────────────────────────────────────────────
# THE REAL DELIVERABLE. Each fixture = setup(project_root) + ordered steps.
# Steps run through the CLI process boundary; args are appended with the
# project root LAST (matching every verb's [path] positional), mirroring
# _blackbox.invoke.


@dataclass
class Step:
    argv: tuple[str, ...]
    stdin: str = ""
    append_root: bool = True   # False for verbs with no [path] positional


@dataclass
class Fixture:
    name: str
    description: str
    setup: Callable[[Path], Any]
    steps: tuple[Step, ...]


ALL_AGENTS = ("claude", "cursor", "opencode", "pi", "omp")
# Every selector that has an mcp_config_path/mcp_key pair (platform.sh);
# copilot has no native MCP and is included only as an enabled selector.
ALL_MCP_AGENTS = ("claude", "cursor", "opencode", "codex", "copilot",
                  "gemini", "pi", "omp")
ALL_RECIPES = (
    "bitbucket-pr-flow", "git-pr-flow", "gitlab-mr-flow", "jinna-mcp-recipe",
    "plan-build-flow", "playwright-mcp", "playwright-ui-flow",
    "session-context", "tdd-flow", "trello-mcp-workflow",
    "vault-canonical-store", "worktree-flow",
)


def _manifest(agents: tuple[str, ...] = ("claude",),
              recipes: tuple[str, ...] = (),
              extra: str = "") -> str:
    enabled = ", ".join(repr(a) for a in agents)
    text = f"[project]\nname = 'parity-fixture'\n\n[agents]\nenabled = [{enabled}]\n"
    for rid in recipes:
        text += f"\n[recipes.{rid}]\nenabled = true\n"
    return text + extra


def _write(project: Path, rel: str, content: str) -> None:
    p = project / rel
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(content)


def _git(project: Path, *args: str) -> None:
    subprocess.run(["git", "-C", str(project), *args], check=True,
                   capture_output=True, text=True,
                   env={"PATH": BASE_ENV["PATH"], "HOME": str(project / ".home"),
                        "GIT_AUTHOR_NAME": "parity", "GIT_AUTHOR_EMAIL": "p@x",
                        "GIT_COMMITTER_NAME": "parity", "GIT_COMMITTER_EMAIL": "p@x",
                        # Pinned dates keep commit hashes deterministic across
                        # legs: setup runs per leg at different wall-clock times.
                        "GIT_AUTHOR_DATE": "2000-01-01T00:00:00+0000",
                        "GIT_COMMITTER_DATE": "2000-01-01T00:00:00+0000"})


def _setup_fresh(project: Path) -> None:
    _write(project, "README.md", "# fresh project\n")


def _setup_rules_audit_bare(project: Path) -> None:
    """Bare project: only a README, so `rules-audit` emits the minimal
    (mode B) inventory."""
    _write(project, "README.md", "# bare project\n")


def _setup_rules_audit_inventory(project: Path) -> None:
    """Rich legacy surface for the rules-audit scanner: .cursor rules with and
    without frontmatter, a .cursorrules, AGENTS.md headings, an enabled
    manifest, a local skill, and node/python/go lockfiles."""
    _write(project, ".cursor/rules/with-frontmatter.mdc",
           "---\n"
           "description: TDD workflow rules\n"
           'globs: ["src/**/*.ts"]\n'
           "alwaysApply: true\n"
           "---\n"
           "# Workflow rules\n"
           "Run tests first.\n")
    _write(project, ".cursor/rules/plain.mdc",
           "# Plain rules\n"
           "Deprecate this obsolete rule file.\n")
    _write(project, ".cursorrules",
           "Use the vault and open a pull request.\n")
    _write(project, "AGENTS.md",
           "# Project overview\n"
           "See the `tdd-flow` skill.\n"
           "\n"
           "## Runtime notes\n"
           "The Trello board is the source of truth.\n"
           "\n"
           "### Conflict policy\n"
           "Follow the worktree flow.\n")
    _write(project, "ai-specs/ai-specs.toml", _manifest(
        agents=("claude", "pi"),
        recipes=("tdd-flow", "session-context")))
    _write(project, "ai-specs/skills/local-skill/SKILL.md", "# local skill\n")
    _write(project, "package-lock.json", "{}\n")
    _write(project, "pyproject.toml", "[project]\nname = 'fixture'\n")
    _write(project, "go.mod", "module fixture\n")


def _setup_manifest_only(project: Path) -> None:
    _write(project, "ai-specs/ai-specs.toml", _manifest())
    (project / "ai-specs" / "skills").mkdir(parents=True, exist_ok=True)
    (project / "ai-specs" / "commands").mkdir(exist_ok=True)


def _setup_all_recipes(project: Path) -> None:
    _write(project, "ai-specs/ai-specs.toml", _manifest(recipes=ALL_RECIPES))
    (project / "ai-specs" / "skills").mkdir(parents=True, exist_ok=True)
    (project / "ai-specs" / "commands").mkdir(exist_ok=True)


def _setup_dirty_git(project: Path) -> None:
    """Initialized project committed to git, then dirtied: modified tracked
    file, untracked junk, and merge-conflict markers in a tracked skill file."""
    _setup_manifest_only(project)
    _write(project, "ai-specs/AGENTS.md", "# stale local brief\nlocal edit\n")
    _write(project, "ai-specs/skills/local-skill/SKILL.md", "# local\n")
    _git(project, "init", "-q")
    _git(project, "add", "-A")
    _git(project, "commit", "-q", "-m", "init")
    _write(project, "ai-specs/AGENTS.md", "# stale local brief\nlocal edit\nDIRTY\n")
    _write(project, "untracked-junk.txt", "junk\n")
    _write(project, "ai-specs/skills/local-skill/SKILL.md",
           "# local\n<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> branch\n")


def _setup_missing_dep(project: Path) -> None:
    """Manifest whose [[deps]] source points at a nonexistent local git path."""
    _write(project, "ai-specs/ai-specs.toml", _manifest(extra=(
        "\n[[deps]]\n"
        'id = "ghost-skill"\n'
        f'source = "{project / "nowhere" / "ghost-repo"}"\n'
        'path = "skills/ghost"\n'
        'scope = ["root"]\n'
    )))
    (project / "ai-specs" / "skills").mkdir(parents=True, exist_ok=True)
    (project / "ai-specs" / "commands").mkdir(exist_ok=True)


def _setup_deps_gitignore(project: Path) -> None:
    """Manifest with two [[deps]] pointing at nonexistent local paths.

    Sync renders ai-specs/.gitignore (step 1) and the root agent block (step 2)
    BEFORE deps materialization (step 4) fails, so both legs must still emit
    byte-identical gitignore files despite the later failure.
    """
    _write(project, "ai-specs/ai-specs.toml", _manifest(extra=(
        "\n[[deps]]\n"
        'id = "ghost-one"\n'
        f'source = "{project / "nowhere" / "ghost-one"}"\n'
        'path = "skills/ghost-one"\n'
        'scope = ["root"]\n'
        "\n[[deps]]\n"
        'id = "ghost-two"\n'
        f'source = "{project / "nowhere" / "ghost-two"}"\n'
        'path = "skills/ghost-two"\n'
        'scope = ["root"]\n'
    )))
    (project / "ai-specs" / "skills").mkdir(parents=True, exist_ok=True)
    (project / "ai-specs" / "commands").mkdir(exist_ok=True)


def _setup_doctor_degraded(project: Path) -> None:
    """Legacy recipe version pin, [brief].render = false with the runtime-brief
    marker, a stale managed .envrc, and declared MCP env missing from
    ai-specs.env."""
    _write(project, "ai-specs/ai-specs.toml", _manifest(
        agents=("claude",),
        recipes=("trello-mcp-workflow",),
        extra=(
            "\n[recipes.trello-mcp-workflow.config]\n"
            'board_id = "69ec097f13e2d38ecd89a557"\n'
            "\n[recipes.tdd-flow]\n"
            "enabled = true\n"
            'version = "9.9.9"\n'
            "\n[brief]\n"
            "render = false\n"
        ),
    ))
    (project / "ai-specs" / "skills").mkdir(parents=True, exist_ok=True)
    (project / "ai-specs" / "commands").mkdir(exist_ok=True)
    _write(project, "AGENTS.md",
           "# project brief\n\n<!-- ai-specs:runtime-brief -->\n")
    # Managed block present but body stale: doctor must WARN stale, not missing.
    _write(project, ".envrc",
           "# .envrc - direnv entry for this project\n"
           "# managed-by: ai-specs (do not remove block)\n"
           "dotenv\n"
           "# end managed-by: ai-specs\n")


def _setup_adopt_brief(project: Path) -> None:
    """User-owned AGENTS.md without the runtime-brief marker and with no lock
    baseline: `sync --adopt-brief` must record the existing bytes as the
    managed baseline without overwriting them (design D6 user-issued handoff)."""
    _write(project, "ai-specs/ai-specs.toml", _manifest())
    (project / "ai-specs" / "skills").mkdir(parents=True, exist_ok=True)
    (project / "ai-specs" / "commands").mkdir(exist_ok=True)
    _write(project, "AGENTS.md", "# my own brief\n\nHand-written notes.\n")


def _setup_mcp_per_agent(project: Path) -> None:
    """Every MCP-capable agent selector with two manifest servers (one local
    with env refs, one HTTP with a header) and pre-seeded target files carrying
    foreign keys / a foreign codex table, so `sync-agent --all` exercises every
    mcp_key/path pair (incl. opencode `mcp` and codex `mcp_servers`) through the
    merge-and-preserve path. Today both legs run the Python renderer via
    sync-agent.sh; this fixture becomes the Go gate when the fan-out is ported
    in S15."""
    _write(project, "ai-specs/ai-specs.toml", _manifest(
        agents=ALL_MCP_AGENTS,
        extra=(
            "\n[mcp.alpha]\n"
            "command = 'npx'\n"
            "args = ['-y', '@scope/server']\n"
            "env = { TOKEN = '${ALPHA_TOKEN}', PLAIN = 'value' }\n"
            "\n[mcp.beta]\n"
            "type = 'http'\n"
            "url = 'https://example.test/mcp'\n"
            "headers = { Authorization = '${env:BETA_TOKEN}' }\n"
            "timeout = 30\n"
        ),
    ))
    (project / "ai-specs" / "skills").mkdir(parents=True, exist_ok=True)
    (project / "ai-specs" / "commands").mkdir(exist_ok=True)
    # Foreign keys + a foreign codex table must survive the merge.
    _write(project, ".mcp.json",
           '{"foreignTop": 1, "mcpServers": {"stale": {"command": "old"}}}\n')
    _write(project, ".cursor/mcp.json", '{"cursorOnly": true}\n')
    _write(project, "opencode.json",
           '{"theme": "dark", "$schema": "https://opencode.ai/config.json",'
           ' "mcp": {"stale": {"type": "local"}}}\n')
    _write(project, ".codex/config.toml",
           '# user comment\n\n[user_table]\nx = 1\n\n'
           '[mcp_servers.stale]\ncommand = "old"\n')
    _write(project, ".gemini/settings.json", '{"geminiOnly": true}\n')
    _write(project, ".omp/mcp.json", '{"ompOnly": true}\n')


def _setup_hooks_five_runtimes(project: Path) -> None:
    """All five runtimes enabled with a hook-declaring catalog recipe
    (`worktree-flow`: one file-write matcher plus one shell matcher) so `sync`
    then `sync-agent --all` renders native runtime-hook wiring for every
    harness: claude settings.json, the cursor wrapper + hooks.json (the
    file-write hook has no Cursor pre-file-write target, so it warns and skips),
    and the opencode/pi/omp TS adapters. Pre-seeded foreign keys and a previous
    managed entry per harness must be preserved / replaced in place, and stale
    adapters must survive. Today both legs run the Python renderer via
    sync-agent.sh; this fixture becomes the Go gate when the fan-out is ported
    in S15."""
    _write(project, "ai-specs/ai-specs.toml", _manifest(
        agents=ALL_AGENTS,
        recipes=("worktree-flow",),
    ))
    (project / "ai-specs" / "skills").mkdir(parents=True, exist_ok=True)
    (project / "ai-specs" / "commands").mkdir(exist_ok=True)
    # Foreign keys + a previous managed entry per harness.
    _write(project, ".claude/settings.json",
           '{"model": "opus", "foreign": {"keep": [1, 2]}, "hooks": {'
           '"PreToolUse": [{"_ai_specs_managed":'
           ' "ai-specs:hooks:worktree-flow:worktree-gate", "matcher": "Stale"}]}}\n')
    _write(project, ".cursor/hooks.json",
           '{"version": 1, "hooks": {"beforeShellExecution": ['
           '{"_ai_specs_managed":'
           ' "ai-specs:hooks:worktree-flow:worktree-gate-shell",'
           ' "command": "./.cursor/hooks/old.sh"}]}}\n')
    _write(project, ".cursor/hooks/old.sh", "#!/usr/bin/env bash\necho stale\n")
    _write(project, ".opencode/plugin/old.ts", "// stale adapter\n")
    _write(project, ".pi/extensions/old.ts", "// stale adapter\n")
    _write(project, ".omp/extensions/old.ts", "// stale adapter\n")


def _setup_cache_layout(project: Path) -> None:
    """Bundled + recipe-managed + hand-authored commands and skills: `sync`
    then `sync-agent --all` must materialize recipes into the per-project CLI
    cache (ensure_cache / .recipe / .bundled), flatten resolved skills, and
    merge commands with local precedence (the colliding `tdd.md` warns and the
    local copy wins). Both legs still run `project-cache.py`; this fixture
    becomes the Go gate when sync-agent's cache calls move to Go."""
    _write(project, "ai-specs/ai-specs.toml", _manifest(
        agents=ALL_AGENTS,
        recipes=("worktree-flow", "tdd-flow"),
    ))
    (project / "ai-specs" / "skills").mkdir(parents=True, exist_ok=True)
    # A local command colliding with the recipe-managed `tdd` command (local
    # hand-authored must win and warn) plus one local-only command.
    _write(project, "ai-specs/commands/tdd.md", "# local tdd override\n")
    _write(project, "ai-specs/commands/local-only.md", "# local only\n")


def _leg_cache_root(project: Path) -> Path:
    """The leg's REAL per-project CLI cache root (project-cache.py layout).

    run_leg builds the isolated install home at <scratch>/home/cli-home
    (bb.isolated_home appends 'cli-home') and the per-project cache lives at
    <home>/cache/projects/<sha256(realpath)[:12]>-<basename>. A fixture setup
    runs BEFORE any CLI step, so it must create that root itself; mirroring the
    documented cache_key shape here puts the seeds at the exact paths the CLI
    scans, never at a guessed lookalike the cleanup would silently miss.
    """
    home = project.parent / "home" / "cli-home"
    if not home.is_dir():
        # Loud rather than vacuous: a changed harness layout would otherwise
        # leave the seeds unseen and the fixture would compare two no-ops.
        raise RuntimeError(
            f"parity fixture layout changed: expected isolated home at {home}")
    resolved = project.resolve()
    key = hashlib.sha256(str(resolved).encode("utf-8")).hexdigest()[:12]
    return home / "cache" / "projects" / f"{key}-{resolved.name}"


def _setup_orphan_cleanup(project: Path) -> None:
    """Stale materialized artifacts in ALL THREE cleanup scopes, live dirs,
    stray files and a stale recipe lock entry, then one verbose `sync`.

    Verbose is REQUIRED, not cosmetic: sync's compact step printer drops every
    line whose first char is a success marker, so the `✓ removed orphaned ...`
    lines (the only evidence the two cache scopes are cleaned, since the cache
    lives OUTSIDE the project tree the harness snapshots) would be filtered
    out and the fixture would compare two no-ops.

    The stale recipe lock entry is pruned by the earlier bundled-refresh step
    (refresh-bundled.py), whose unconditional lock write drops the legacy
    [recipes.*] section BEFORE clean_orphans ever reads it (lock.py's writer
    emits only meta/managed/deps/agents). The fixture therefore pins the
    effective lock outcome: no [recipes.*] entry survives sync. The
    clean_orphans `_prune_stale_lock` branch is only reachable when nothing
    rewrote the lock first, so it is NOT observable here.

    The `[[deps]]` source is a plain LOCAL tree copied by vendor-skills.clone
    (no network), and it is RELATIVE so the rendered skill metadata embeds a
    stable string: an absolute source path would change the raw file hash that
    the lock records and split the two legs.
    """
    _write(project, "ai-specs/ai-specs.toml", _manifest(
        agents=("claude",),
        recipes=("tdd-flow", "session-context"),
        extra=(
            "\n[[deps]]\n"
            'id = "vendor-one"\n'
            'source = "../dep-src"\n'
            "scope = ['root']\n"
        ),
    ))
    (project / "ai-specs" / "skills").mkdir(parents=True, exist_ok=True)
    (project / "ai-specs" / "commands").mkdir(exist_ok=True)
    # Plain local dep tree OUTSIDE the project (not snapshotted; deterministic).
    dep_src = project.parent / "dep-src"
    dep_src.mkdir(parents=True, exist_ok=True)
    (dep_src / "SKILL.md").write_text(
        "---\ndescription: vendored fixture skill\n---\n# vendored\n")
    # Stale recipe lock entry (legacy [recipes.*] shape load_lock reads back).
    _write(project, "ai-specs/.ai-specs.lock",
           "# Managed by ai-specs. Do not edit by hand.\n\n"
           "[recipes.obsolete-recipe.skills.some-skill]\n"
           'SKILL.md = "deadbeef"\n')
    # In-project scope (inside the snapshotted tree): stale orphan + stray file.
    (project / "ai-specs" / ".deps" / "obsolete-dep").mkdir(parents=True)
    _write(project, "ai-specs/.deps/obsolete-dep/SKILL.md", "stale\n")
    _write(project, "ai-specs/.deps/stray.txt", "keep me\n")
    # Cache scopes (outside the project tree): stale + live dirs + stray files.
    cache = _leg_cache_root(project)
    for scope in (".recipe", ".deps"):
        (cache / scope).mkdir(parents=True, exist_ok=True)
        (cache / scope / "stray.txt").write_text("keep me\n")
    (cache / ".recipe" / "tdd-flow").mkdir()          # live (enabled recipe)
    _write(cache, ".recipe/tdd-flow/SKILL.md", "live\n")
    (cache / ".recipe" / "obsolete-recipe").mkdir()   # stale orphan
    _write(cache, ".recipe/obsolete-recipe/SKILL.md", "stale\n")
    (cache / ".deps" / "vendor-one").mkdir()          # live (expected dep)
    _write(cache, ".deps/vendor-one/SKILL.md", "live\n")
    (cache / ".deps" / "obsolete-dep").mkdir()        # stale orphan
    _write(cache, ".deps/obsolete-dep/SKILL.md", "stale\n")


def _setup_brief_render_false(project: Path) -> None:
    """[brief].render = false: sync's policy gate must skip the AGENTS.md step
    entirely (no file written, skip notice on stdout)."""
    _write(project, "ai-specs/ai-specs.toml",
           _manifest(extra="\n[brief]\nrender = false\n"))
    (project / "ai-specs" / "skills").mkdir(parents=True, exist_ok=True)
    (project / "ai-specs" / "commands").mkdir(exist_ok=True)


def _setup_sync_agent_standalone(project: Path) -> None:
    """Standalone `sync-agent --all` (no --source-root/--target) on a declared
    multi-target root that was never synced: the public-root fan-out banner,
    the --resolved-config-only seam, three nested children (each running its
    own materialize fallback — D22: no hooks without --resolved-hooks), child
    banner/footer suppression, and one parent footer. AGENTS.md is seeded by
    hand: the same-root workspace guard requires it, and the root child never
    renders it (only subrepo children render through ensure_target_workspace).
    """
    _write(project, "ai-specs/ai-specs.toml",
           "[project]\n"
           "name = 'parity-fixture'\n"
           "subrepos = ['sub-a', 'sub-b']\n"
           "\n[agents]\n"
           "enabled = ['claude', 'pi']\n")
    _write(project, "AGENTS.md", "# hand-seeded root brief\n")
    (project / "ai-specs" / "skills").mkdir(parents=True, exist_ok=True)
    (project / "ai-specs" / "commands").mkdir(exist_ok=True)
    (project / "sub-a").mkdir(exist_ok=True)
    (project / "sub-b").mkdir(exist_ok=True)


def _setup_nested_fanout_suppression(project: Path) -> None:
    """Declared multi-target root: `sync` fans out to nested children (their
    banner/footer is suppressed by AI_SPECS_SYNC_NESTED=1 inside the parent's
    captured step), then standalone `sync-agent --all` re-runs the same targets
    with full framing. A suppression leak in the native fan-out changes stdout
    in the sync step; a framing leak in the standalone path changes it in the
    second step."""
    _write(project, "ai-specs/ai-specs.toml",
           "[project]\n"
           "name = 'parity-fixture'\n"
           "subrepos = ['sub-a', 'sub-b']\n"
           "\n[agents]\n"
           "enabled = ['claude', 'pi']\n")
    (project / "ai-specs" / "skills").mkdir(parents=True, exist_ok=True)
    (project / "ai-specs" / "commands").mkdir(exist_ok=True)
    (project / "sub-a").mkdir(exist_ok=True)
    (project / "sub-b").mkdir(exist_ok=True)


def _setup_fanout_matrix(project: Path) -> None:
    """All eight agent selectors with claude LAST behind a real CLAUDE.md file
    (the relative instructions symlink must refuse, rc 1) and a user-owned
    file inside .cursor/commands (D3': never rm -rf the commands dir; managed
    names are overwritten in place, the user file is preserved with a
    warning). `sync` fails at the root child's claude step after everything
    before it succeeded; `sync-agent --all` fails the same way standalone —
    both legs must agree on rc, streams and the partial tree, including the
    relative/absolute symlink kinds of the seven agents that did sync."""
    _write(project, "ai-specs/ai-specs.toml", _manifest(
        agents=("cursor", "opencode", "codex", "copilot", "gemini", "pi",
                "omp", "claude"),
    ))
    (project / "ai-specs" / "skills").mkdir(parents=True, exist_ok=True)
    (project / "ai-specs" / "commands").mkdir(exist_ok=True)
    # Non-symlink occupancy on claude's instructions path: hard refusal.
    _write(project, "CLAUDE.md", "manual file — not a symlink\n")
    # User-owned command file in the cursor commands dir: preserved (D3').
    _write(project, ".cursor/commands/user-own.md", "# mine\n")


def _setup_sync_agent_arg_contract(project: Path) -> None:
    """JD-A-001 + JD-A-002 regression fixture. [agents].enabled carries a
    whitespace-padded string, a padded valid agent, an int, a bool and an
    empty string: toml-read.py's _normalize_string_list keeps only stripped
    non-empty strings, so exactly claude and pi must sync (the int/bool are
    dropped, never repr'd as agent names). Remediation batch 2 extends the
    mix with an information-separator prefix (U+001C strips like space, so
    the entry normalizes to laude2 — an unknown agent) and an embedded
    newline (the print + while-read loop splits it into x and y — two more
    unknown-agent notices; three total beyond claude/pi). The trailing
    value-taking flag steps pin the
    `shift 2` failure: rc 1, no output, no writes (D20 class)."""
    _write(project, "ai-specs/ai-specs.toml",
           "[project]\n"
           "name = 'parity-fixture'\n"
           "\n[agents]\n"
           "enabled = [' claude', 'pi ', \"\\u001claude2\", \"x\\ny\", 1, true, '']\n")
    _write(project, "AGENTS.md", "# hand-seeded root brief\n")
    (project / "ai-specs" / "skills").mkdir(parents=True, exist_ok=True)
    (project / "ai-specs" / "commands").mkdir(exist_ok=True)


def _setup_sync_agent_recipe_mcp_matrix(project: Path) -> None:
    """JD remediation batch 2 (recipe-mcp count) regression fixture: the
    byte-comparable len cases of the MCP_COUNT oracle — top-level list (len 2),
    string (len 3), object (len 1), malformed body (0) and a missing file (0)
    — each as its own `--recipe-mcp` invocation so the banner's `mcp:` line
    pins the count. The DEATH cases (a directory path / a number / bool /
    null: uncaught IsADirectoryError, TypeError tracebacks in legacy) are NOT
    fixtureable here — the tracebacks embed the platform python's frames —
    they are unit-gated by TestMCPCountMatrix with the documented deviation."""
    _write(project, "ai-specs/ai-specs.toml",
           "[project]\n"
           "name = 'parity-fixture'\n"
           "\n[agents]\n"
           "enabled = ['claude']\n")
    _write(project, "AGENTS.md", "# hand-seeded root brief\n")
    (project / "ai-specs" / "skills").mkdir(parents=True, exist_ok=True)
    (project / "ai-specs" / "commands").mkdir(exist_ok=True)
    (project / "sub-a").mkdir(exist_ok=True)
    (project / "recipe-mcp").mkdir(exist_ok=True)
    _write(project, "recipe-mcp/list.json", "[1,2]\n")
    _write(project, "recipe-mcp/str.json", '"abc"\n')
    _write(project, "recipe-mcp/obj.json", '{"only": {}}\n')
    _write(project, "recipe-mcp/bad.json", "not json\n")


def _setup_sync_agent_abort_shapes(project: Path) -> None:
    """JD remediation batch 2 (aborting coreutils shapes) regression fixture:
    three sequential single-target invocations, each aborting at a different
    bare/`|| return` site with the measured coreutils stderr and rc 1 —
    (1) managed `cp` into a 0555 .claude/commands (Permission denied naming
    the destination), (2) mirror_directory's `rm -rf` over a 0555 skills
    directory holding a child, (3) `mkdir -p` over an existing ai-specs FILE
    (File exists). Message text, target path and rc are FROZEN; the fixture
    covers all three shapes in one deterministic setup."""
    _write(project, "ai-specs/ai-specs.toml",
           "[project]\n"
           "name = 'parity-fixture'\n"
           "subrepos = ['subA', 'subB', 'subC']\n"
           "\n[agents]\n"
           "enabled = ['claude']\n")
    _write(project, "AGENTS.md", "# hand-seeded root brief\n")
    (project / "ai-specs" / "commands").mkdir(parents=True, exist_ok=True)
    _write(project, "ai-specs/commands/demo.md", "# managed command\n")
    (project / ".claude" / "commands").mkdir(parents=True, exist_ok=True)
    (project / ".claude" / "commands").chmod(0o555)
    (project / "subA").mkdir(exist_ok=True)
    skills_b = project / "subB" / "ai-specs" / "skills" / "child"
    skills_b.mkdir(parents=True, exist_ok=True)
    (skills_b / "f.md").write_text("x\n")
    skills_b.parent.chmod(0o555)
    (project / "subC").mkdir(exist_ok=True)
    _write(project, "subC/ai-specs", "occupied\n")


def _setup_sync_agent_unreadable_mirror(project: Path) -> None:
    """Final-round F2 regression fixture (sanctioned live measurement of the
    unmeasured BSD tree detail): a subrepo whose ai-specs/skills directory is
    EMPTY but UNREADABLE (0333 — no read permission). MEASURED OUTCOME (the
    differential harness against the live legacy rm, replacing the
    safety-blocked manual probe): rm -rf does NOT report the read failure —
    it removes the empty dir SILENTLY (no stderr, rc 0, directory gone) and
    the run continues to full success; the Go port matches. The removal-
    failure shapes (child EACCES + parent `Directory not empty`, rc 1) are
    pinned by sync-agent-abort-shapes and the unit tests."""
    _write(project, "ai-specs/ai-specs.toml",
           "[project]\n"
           "name = 'parity-fixture'\n"
           "subrepos = ['subB']\n"
           "\n[agents]\n"
           "enabled = ['claude']\n")
    _write(project, "AGENTS.md", "# hand-seeded root brief\n")
    (project / "ai-specs" / "commands").mkdir(parents=True, exist_ok=True)
    skills_dir = project / "subB" / "ai-specs" / "skills"
    skills_dir.mkdir(parents=True, exist_ok=True)
    skills_dir.chmod(0o333)
    (project / "subB").mkdir(exist_ok=True)


def _setup_sync_agent_occupied_paths(project: Path) -> None:
    """JD-A-003 regression fixture: .pi and .omp exist as REGULAR FILES while
    pi and omp are enabled. The symlink helpers run bare inside run_step's
    set +e, so mkdir -p (<occupied leaf>: File exists) and ln (Not a
    directory) print coreutils-shaped stderr, the ✓ symlink created line is
    still emitted, the run completes rc 0 with the footer, and claude (no
    occupancy) syncs normally. Byte-compares the error shapes."""
    _write(project, "ai-specs/ai-specs.toml", _manifest(
        agents=("claude", "pi", "omp"),
    ))
    _write(project, "AGENTS.md", "# hand-seeded root brief\n")
    (project / "ai-specs" / "skills").mkdir(parents=True, exist_ok=True)
    (project / "ai-specs" / "commands").mkdir(exist_ok=True)
    _write(project, ".pi", "occupied\n")
    _write(project, ".omp", "occupied\n")


def _setup_sync_agent_ro_mirror(project: Path) -> None:
    """JD-A-004 regression fixture: a local skill directory is read-only
    (0555) when flatten copies it into the cache, and the subrepo mirror then
    copies FROM the read-only cache directory. cp -R writes the contents
    first and applies the source mode afterwards, so the mirror succeeds and
    the destination ends 0555. The explicit single-target form (--source-root
    . --target sub-a) is required: with a multi-target fan-out the SECOND
    child's flatten re-rmtree's the 0555 cache dir and dies in BOTH legs with
    an unnormalizable Python traceback (the deviation documented on the
    Flatten port), which would make the fixture undecidable."""
    _write(project, "ai-specs/ai-specs.toml",
           "[project]\n"
           "name = 'parity-fixture'\n"
           "subrepos = ['sub-a']\n"
           "\n[agents]\n"
           "enabled = ['claude', 'pi']\n")
    _write(project, "AGENTS.md", "# hand-seeded root brief\n")
    (project / "ai-specs" / "commands").mkdir(parents=True, exist_ok=True)
    skill_dir = project / "ai-specs" / "skills" / "ro-skill"
    skill_dir.mkdir(parents=True, exist_ok=True)
    (skill_dir / "SKILL.md").write_text("# read-only skill\n")
    skill_dir.chmod(0o555)
    (project / "sub-a").mkdir(exist_ok=True)


def _setup_doctor_healthy(project: Path) -> None:
    """Synced project: sync + sync-agent --all, then doctor must agree on the
    generated surface.

    Deliberately excludes the tracker recipe. Its required board_id makes sync
    materialize a path-stamped tracker-card-gate.sh whose raw hash the lock
    records, so the lock can never agree between two isolated scratch roots
    (an environment artifact, not port behavior). The tracker recipe's own dep
    and env coverage lives in doctor-degraded, which is zero-delta.
    """
    _write(project, "ai-specs/ai-specs.toml", _manifest(
        agents=("claude", "pi"),
        recipes=("tdd-flow", "session-context", "worktree-flow"),
    ))
    (project / "ai-specs" / "skills").mkdir(parents=True, exist_ok=True)
    (project / "ai-specs" / "commands").mkdir(exist_ok=True)


CORPUS: tuple[Fixture, ...] = (
    Fixture(
        name="fresh-init",
        description="Empty project: first `init` bootstraps ai-specs/.",
        setup=_setup_fresh,
        steps=(Step(("init",)),),
    ),
    Fixture(
        name="idempotent-resync",
        description="Pre-seeded manifest: `sync` then `sync` again — the "
                    "second run must be idempotent (tree snapshot after each "
                    "step is diffed, so drift between run 1 and run 2 shows).",
        setup=_setup_manifest_only,
        steps=(Step(("sync",)), Step(("sync",))),
    ),
    Fixture(
        name="all-recipes-enabled",
        description="Every shipped catalog recipe enabled: full `sync` "
                    "(vendor + bundled flatten + brief render + per-agent).",
        setup=_setup_all_recipes,
        steps=(Step(("sync",)),),
    ),
    Fixture(
        name="multi-agent-fanout",
        description="All five runtimes enabled: `sync` then `sync-agent --all`.",
        setup=lambda p: (_write(p, "ai-specs/ai-specs.toml",
                                _manifest(agents=ALL_AGENTS)),
                         (p / "ai-specs" / "skills").mkdir(parents=True),
                         (p / "ai-specs" / "commands").mkdir()),
        steps=(Step(("sync",)), Step(("sync-agent", "--all"))),
    ),
    Fixture(
        name="dirty-conflicted-project",
        description="Git repo with modified AGENTS.md, untracked junk and "
                    "conflict markers, then `doctor` and `sync`.",
        setup=_setup_dirty_git,
        steps=(Step(("doctor",)), Step(("sync",))),
    ),
    Fixture(
        name="missing-optional-deps",
        description="Manifest [[deps]] pointing at a nonexistent local repo: "
                    "`doctor` then `sync` must degrade identically.",
        setup=_setup_missing_dep,
        steps=(Step(("doctor",)), Step(("sync",))),
    ),
    Fixture(
        name="deps-gitignore",
        description="Manifest with two [[deps]] pointing at nonexistent local "
                    "paths: `sync` renders ai-specs/.gitignore (step 1) and the "
                    "root agent block (step 2) before deps materialization "
                    "fails, so both gitignore files must be byte-identical in "
                    "both legs.",
        setup=_setup_deps_gitignore,
        steps=(Step(("sync",)),),
    ),
    Fixture(
        name="surface-verbs",
        description="Dispatcher surface: `help` (heredoc quirk), `version`, "
                    "an unknown verb (exit-code contract), and `doctor` with "
                    "an unknown flag (exit 2 contract).",
        setup=_setup_manifest_only,
        steps=(
            Step(("help",), append_root=False),
            Step(("version",), append_root=False),
            Step(("bogus-verb",), append_root=False),
            Step(("doctor", "--bogus-flag")),
        ),
    ),
    Fixture(
        name="recipe-surface",
        description="Recipe verbs: read-only `recipe list`, read-only "
                    "`recipe init` brief, then `recipe add` (writes the "
                    "manifest) and `recipe list` again.",
        setup=_setup_manifest_only,
        steps=(
            Step(("recipe", "list")),
            Step(("recipe", "init", "worktree-flow")),
            Step(("recipe", "add", "tdd-flow")),
            Step(("recipe", "list")),
        ),
    ),
    Fixture(
        name="doctor-broken",
        description="Manifest without any sync: `doctor` must report the "
                    "missing generated surface.",
        setup=_setup_manifest_only,
        steps=(Step(("doctor",)),),
    ),
    Fixture(
        name="doctor-degraded",
        description="Legacy recipe `version=`, `[brief].render = false` with "
                    "the runtime-brief marker, a stale managed `.envrc`, and "
                    "declared MCP env missing from `ai-specs.env`.",
        setup=_setup_doctor_degraded,
        steps=(Step(("doctor",)),),
    ),
    Fixture(
        name="doctor-healthy",
        description="Synced project: `sync`, `sync-agent --all`, then "
                    "`doctor` must agree on the generated surface. Enables "
                    "`tdd-flow`, `session-context` and `worktree-flow` only: "
                    "the tracker recipe's required `board_id` makes sync "
                    "materialize a path-stamped `tracker-card-gate.sh` whose "
                    "raw hash the lock records, so the lock can never agree "
                    "between two isolated scratch roots; the tracker recipe's "
                    "own dep and env coverage lives in `doctor-degraded`, "
                    "which is zero-delta.",
        setup=_setup_doctor_healthy,
        steps=(Step(("sync",)), Step(("sync-agent", "--all")),
               Step(("doctor",))),
    ),
    Fixture(
        name="rules-audit-bare",
        description="Empty project: `rules-audit` must emit the minimal "
                    "inventory.",
        setup=_setup_rules_audit_bare,
        steps=(Step(("rules-audit",)),),
    ),
    Fixture(
        name="rules-audit-inventory",
        description="Rich legacy surface: `.cursor/rules/*.mdc` with and "
                    "without frontmatter, a `.cursorrules`, an `AGENTS.md` "
                    "with 1-3 hash headings and a backticked skill id, a "
                    "manifest with enabled agents/recipes, a local "
                    "`ai-specs/skills/<id>/SKILL.md`, and node/python/go "
                    "lockfiles must produce the same inventory.",
        setup=_setup_rules_audit_inventory,
        steps=(Step(("rules-audit",)),),
    ),
    Fixture(
        name="sync-adopt-brief",
        description="User-owned AGENTS.md with no runtime-brief marker and no "
                    "lock baseline: `sync --adopt-brief` must adopt the existing "
                    "bytes as the managed baseline without overwriting them.",
        setup=_setup_adopt_brief,
        steps=(Step(("sync", "--adopt-brief")),),
    ),
    Fixture(
        name="sync-brief-render-false",
        description="[brief].render = false: sync's policy gate must skip the "
                    "AGENTS.md step entirely (no file written, skip notice on "
                    "stdout).",
        setup=_setup_brief_render_false,
        steps=(Step(("sync",)),),
    ),
    Fixture(
        name="mcp-per-agent",
        description="All eight agent selectors enabled with two [mcp.*] "
                    "servers and pre-seeded target files carrying foreign keys "
                    "and a foreign codex table: `sync` then `sync-agent --all` "
                    "must render every mcp_key/path pair (claude/cursor/pi "
                    "mcpServers, opencode mcp, codex mcp_servers, gemini, omp) "
                    "while preserving the foreign content.",
        setup=_setup_mcp_per_agent,
        steps=(Step(("sync",)), Step(("sync-agent", "--all"))),
    ),
    Fixture(
        name="cache-layout",
        description="Bundled + recipe + local commands: `sync` then "
                    "`sync-agent --all` materializes the per-project cache "
                    "(.recipe/.bundled/commands/resolved-skills), flattens "
                    "resolved skills, and merges commands with local "
                    "precedence (colliding `tdd.md` warns).",
        setup=_setup_cache_layout,
        steps=(Step(("sync",)), Step(("sync-agent", "--all"))),
    ),
    Fixture(
        name="orphan-cleanup",
        description="Stale materialized artifacts in all three cleanup scopes "
                    "(cache/.recipe, cache/.deps, ai-specs/.deps) plus a stale "
                    "recipe lock entry, with live dirs and stray files that "
                    "must SURVIVE, then one `sync --verbose`. Verbose is "
                    "load-bearing: the `✓ removed orphaned ...` lines are the "
                    "only harness-visible evidence for the two cache scopes, "
                    "because the cache lives outside the snapshotted project "
                    "tree. Oracle pinning: the legacy CLI still owns sync "
                    "until S14 connects the native materialize orchestration.",
        setup=_setup_orphan_cleanup,
        steps=(Step(("sync", "--verbose")),),
    ),
    Fixture(
        name="hooks-five-runtimes",
        description="All five runtimes enabled with the hook-declaring "
                    "`worktree-flow` recipe (file-write + shell matchers): "
                    "`sync` then `sync-agent --all` must render claude "
                    "settings.json, the cursor wrapper + hooks.json (skipping "
                    "the file-write hook), and the opencode/pi/omp TS adapters "
                    "while preserving foreign config and stale adapters.",
        setup=_setup_hooks_five_runtimes,
        steps=(Step(("sync",)), Step(("sync-agent", "--all"))),
    ),
    Fixture(
        name="sync-agent-standalone",
        description="Standalone `sync-agent --all` (no --source-root/--target) "
                    "on a declared multi-target root that was never synced: "
                    "public-root fan-out banner, the --resolved-config-only "
                    "seam, three nested children each running its own "
                    "materialize fallback (D22: no hooks), child suppression, "
                    "one parent footer.",
        setup=_setup_sync_agent_standalone,
        steps=(Step(("sync-agent", "--all")),),
    ),
    Fixture(
        name="nested-fanout-suppression",
        description="Declared multi-target root: `sync` fans out to nested "
                    "children whose banner/footer is suppressed "
                    "(AI_SPECS_SYNC_NESTED=1) inside the parent's captured "
                    "step, then standalone `sync-agent --all` re-runs the same "
                    "targets with full framing.",
        setup=_setup_nested_fanout_suppression,
        steps=(Step(("sync",)), Step(("sync-agent", "--all"))),
    ),
    Fixture(
        name="fanout-matrix",
        description="All eight agent selectors with claude LAST behind a real "
                    "CLAUDE.md (hard refusal on non-symlink occupancy, rc 1) "
                    "and a user-owned file in .cursor/commands (D3' "
                    "preservation): `sync` then `sync-agent --all` must fail "
                    "identically in both legs on rc, streams and the partial "
                    "tree, including the relative/absolute symlink kinds of "
                    "the seven agents that did sync.",
        setup=_setup_fanout_matrix,
        steps=(Step(("sync",)), Step(("sync-agent", "--all"))),
    ),
    Fixture(
        name="sync-agent-arg-contract",
        description="JD-A-001/JD-A-002 regression: [agents].enabled mixes "
                    "whitespace-padded strings, an information-separator "
                    "prefix, an embedded newline, an int, a bool and an empty "
                    "string — normalization keeps exactly claude, pi, laude2, "
                    "x and y; then trailing `--target`/`--source-root` "
                    "invocations die rc 1 with no output and no writes.",
        setup=_setup_sync_agent_arg_contract,
        steps=(
            Step(("sync-agent", "--all")),
            Step(("sync-agent", "--target"), append_root=False),
            Step(("sync-agent", "--source-root"), append_root=False),
        ),
    ),
    Fixture(
        name="sync-agent-recipe-mcp-matrix",
        description="Remediation batch 2 (recipe-mcp count) regression: the "
                    "byte-comparable MCP_COUNT cases — top-level list (2), "
                    "string (3), object (1), malformed (0) and missing file "
                    "(0) — each pinned by the banner's `mcp:` line. The "
                    "uncaught-death cases (directory/number/bool/null) stay "
                    "unit-gated with stable portable class diagnostics "
                    "(IsADirectoryError/TypeError); the legacy tracebacks embed "
                    "platform-python frames, so they are not fixtureable.",
        setup=_setup_sync_agent_recipe_mcp_matrix,
        steps=(
            Step(("sync-agent", "--source-root", ".", "--target", "sub-a", "--all",
                  "--recipe-mcp", "recipe-mcp/list.json"), append_root=False),
            Step(("sync-agent", "--source-root", ".", "--target", "sub-a", "--all",
                  "--recipe-mcp", "recipe-mcp/str.json"), append_root=False),
            Step(("sync-agent", "--source-root", ".", "--target", "sub-a", "--all",
                  "--recipe-mcp", "recipe-mcp/obj.json"), append_root=False),
            Step(("sync-agent", "--source-root", ".", "--target", "sub-a", "--all",
                  "--recipe-mcp", "recipe-mcp/bad.json"), append_root=False),
            Step(("sync-agent", "--source-root", ".", "--target", "sub-a", "--all",
                  "--recipe-mcp", "recipe-mcp/missing.json"), append_root=False),
        ),
    ),
    Fixture(
        name="sync-agent-unreadable-mirror",
        description="Final-round F2 regression: a subrepo whose ai-specs/skills "
                    "directory is empty but UNREADABLE (0333) — MEASURED via this "
                    "harness: legacy `rm -rf` reports nothing and removes the "
                    "empty dir silently (rc 0, full run success), and the Go port "
                    "matches byte-for-byte. Removal-failure shapes live in "
                    "sync-agent-abort-shapes.",
        setup=_setup_sync_agent_unreadable_mirror,
        steps=(Step(("sync-agent", "--source-root", ".", "--target", "subB", "--all"),
                    append_root=False),),
    ),

    Fixture(
        name="sync-agent-abort-shapes",
        description="Remediation batch 2 (aborting coreutils shapes) "
                    "regression: three sequential single-target invocations "
                    "aborting at the managed `cp` into a 0555 commands dir, "
                    "the mirror `rm -rf` over a 0555 skills dir with a child, "
                    "and `mkdir -p` over an existing ai-specs file — the "
                    "measured coreutils stderr shapes and rc 1 are FROZEN.",
        setup=_setup_sync_agent_abort_shapes,
        steps=(
            Step(("sync-agent", "--source-root", ".", "--target", "subA", "--all"), append_root=False),
            Step(("sync-agent", "--source-root", ".", "--target", "subB", "--all"), append_root=False),
            Step(("sync-agent", "--source-root", ".", "--target", "subC", "--all"), append_root=False),
        ),
    ),
    Fixture(
        name="sync-agent-occupied-paths",
        description="JD-A-003 regression: .pi/.omp exist as regular files "
                    "while pi/omp are enabled — the bare mkdir/ln inside the "
                    "symlink helpers print coreutils-shaped errors, the ✓ "
                    "symlink created line is still emitted, and the run "
                    "completes rc 0 with the footer.",
        setup=_setup_sync_agent_occupied_paths,
        steps=(Step(("sync-agent", "--all")),),
    ),
    Fixture(
        name="sync-agent-ro-mirror",
        description="JD-A-004 regression: a 0555 local skill directory flows "
                    "through flatten into the cache and the subrepo mirror "
                    "then copies FROM the read-only directory — cp -R writes "
                    "contents first and applies the mode after, so the mirror "
                    "succeeds and the destination ends 0555. Single explicit "
                    "target: the second child of a multi-target fan-out would "
                    "re-rmtree the 0555 cache dir and die in both legs with "
                    "an unnormalizable traceback.",
        setup=_setup_sync_agent_ro_mirror,
        steps=(Step(("sync-agent", "--source-root", ".", "--target", "sub-a", "--all"),
                    append_root=False),),
    ),
    Fixture(
        name="hub-surface",
        description="S3 gap `hub`: bare hub status and the hub help text, so "
                    "the interactive surface is measured rather than assumed.",
        setup=_setup_manifest_only,
        steps=(
            Step(("hub",), stdin=""),
            Step(("hub", "--help"), stdin=""),
        ),
    ),
    Fixture(
        name="upgrade-surface",
        description="S3 gap `upgrade`: the help text and an unknown-option "
                    "refusal, so the argument contract is compared.",
        setup=_setup_manifest_only,
        steps=(
            Step(("upgrade", "--help"), append_root=False),
            Step(("upgrade", "--bogus"), append_root=False),
        ),
    ),
    Fixture(
        name="skills-surface",
        description="S3 gap `skills`: bare skills, help, list, remove of an "
                    "absent skill and an unknown subcommand, so the whole "
                    "subcommand surface is compared.",
        setup=_setup_manifest_only,
        steps=(
            Step(("skills",), append_root=False),
            Step(("skills", "--help"), append_root=False),
            Step(("skills", "list")),
            Step(("skills", "remove", "nope")),
            Step(("skills", "bogus"), append_root=False),
        ),
    ),
    Fixture(
        name="vendor-add-clone-failure",
        description="S3 gap `add-dep`: add-dep and `skills add` against a "
                    "nonexistent clone source, so the clone-failure report and "
                    "exit code are compared (see N11).",
        setup=_setup_manifest_only,
        steps=(
            Step(("add-dep",), append_root=False),
            Step(("add-dep", "/nonexistent/skill-repo")),
            Step(("skills", "add", "/nonexistent/skill-repo")),
        ),
    ),
    Fixture(
        name="configure-recipes-surface",
        description="S3 gap `configure-recipes`: help plus the uninitialized "
                    "project refusal, so both legs' exit 1 is compared.",
        setup=_setup_fresh,
        steps=(
            Step(("configure-recipes", "--help"), append_root=False),
            Step(("configure-recipes",), stdin=""),
        ),
    ),
    Fixture(
        name="recipe-remove-configure",
        description="S3 gaps `recipe configure` and `recipe remove`: "
                    "configure on an already-enabled recipe, remove an enabled "
                    "recipe, then list, so the mutation is compared.",
        setup=_setup_all_recipes,
        steps=(
            Step(("recipe", "configure", "bitbucket-pr-flow"), stdin=""),
            Step(("recipe", "remove", "tdd-flow")),
            Step(("recipe", "list")),
        ),
    ),
)


# ── Leg execution ─────────────────────────────────────────────────────────


@dataclass
class StepResult:
    argv: tuple[str, ...]
    rc: int
    stdout: str
    stderr: str
    tree: dict


@dataclass
class LegResult:
    label: str
    cli_path: str
    steps: list[StepResult] = field(default_factory=list)


def make_home(base: Path) -> Path:
    """Isolated install root that carries NO lib/ at all.

    Reuses _blackbox.isolated_home (symlinked install entries + local cache/)
    and drops the lib/ symlink it creates while this checkout still has lib/,
    the same fixture shape the black-box suites use (GO-20 U2e T5 PR4a S7).
    The native binary needs no Python implementation in the install root: the
    legacy side of the comparison is the frozen golden, never a live run.

    Tolerant of absence on purpose: PR4b deletes lib/, and then there is no
    symlink to remove (`missing_ok=True` is a no-op instead of a crash).
    """
    home = bb.isolated_home(base, catalog=True)
    (home / "lib").unlink(missing_ok=True)
    return home


def run_leg(label: str, cli_path: Path, fixture: Fixture, scratch: Path,
            gate_bin: Path | None = None) -> LegResult:
    """Run one implementation against one fixture in an isolated scratch root.

    ``gate_bin`` pins WORKTREE_GATE_BIN for the leg when provided (gate-present
    mode); when None the isolated home has no verified gate binary (gate-absent
    mode — the mode the legacy goldens were captured in).
    """
    home = make_home(scratch / "home")
    home_str = str(home)
    project = scratch / "project"
    project.mkdir(parents=True)
    fixture.setup(project)
    ctx: dict[str, Any] = {"project_root": str(project), "home": home_str,
           "scratch": str(scratch)}
    env = {**BASE_ENV, "HOME": str(scratch / "user-home"), "TMPDIR": str(scratch),
           "AI_SPECS_HOME": home_str}
    if gate_bin is not None:
        env["WORKTREE_GATE_BIN"] = str(gate_bin)
    (scratch / "user-home").mkdir()
    leg = LegResult(label=label, cli_path=str(cli_path))
    for step in fixture.steps:
        ctx["argv"] = step.argv  # N6 scopes itself to the help verb only
        argv = [str(cli_path), *step.argv]
        if step.append_root:
            argv.append(str(project))
        proc = subprocess.run(argv, cwd=project, env=env, text=True,
                              capture_output=True, check=False,
                              input=step.stdin, timeout=600)
        leg.steps.append(StepResult(
            argv=step.argv, rc=proc.returncode,
            stdout=apply_normalizations("stdout", proc.stdout, ctx),
            stderr=apply_normalizations("stderr", proc.stderr, ctx),
            tree=snapshot_tree(project, ctx),
        ))
    return leg


# ── Comparison ────────────────────────────────────────────────────────────


def _fmt_tree_delta(before: dict, after: dict, who: str) -> list[str]:
    lines = []
    for rel in sorted(set(before) | set(after)):
        a, b = before.get(rel), after.get(rel)
        if a is None:
            lines.append(f"    {who} extra path: {rel} -> {b}")
        elif b is None:
            lines.append(f"    {who} missing path: {rel} (other={a})")
        elif a != b:
            lines.append(f"    {who} changed: {rel}\n      other={a}\n      {who}={b}")
    return lines


def diff_legs(legacy: LegResult, other: LegResult) -> list[str]:
    """Readable per-fixture delta report; empty list == zero deltas."""
    deltas: list[str] = []
    if len(legacy.steps) != len(other.steps):
        return [f"  step count differs: legacy={len(legacy.steps)} "
                f"other={len(other.steps)}"]
    for i, (a, b) in enumerate(zip(legacy.steps, other.steps, strict=False)):
        head = f"  step {i}: `{' '.join(b.argv)}`"
        if a.rc != b.rc:
            deltas.append(f"{head} exit code: legacy={a.rc} other={b.rc}")
        if a.stdout != b.stdout:
            deltas.append(f"{head} stdout differs:\n"
                          f"    legacy={a.stdout!r}\n    other ={b.stdout!r}")
        if a.stderr != b.stderr:
            deltas.append(f"{head} stderr differs:\n"
                          f"    legacy={a.stderr!r}\n    other ={b.stderr!r}")
        deltas.extend(_fmt_tree_delta(a.tree, b.tree, "other"))
    return deltas


# ── Go build side ─────────────────────────────────────────────────────────


def build_go_binary(dest_dir: Path) -> Path | None:
    """Build the card-04 Go root-module binary (CGO_ENABLED=0).

    Returns the binary path, or None when go is unavailable or the build
    fails. Policy is the CALLER's: the wired suite entry (run.py) requires
    the build and fails loudly (exit 2); run.py --self-test uses None
    explicitly as the identical-by-shim same-launcher mode (acceptance a).
    Build artifacts stay in the temp dir and are never committed.
    """
    if shutil.which("go") is None:
        return None
    dest = dest_dir / "ai-specs-go"
    env = {**BASE_ENV, "CGO_ENABLED": "0", "HOME": os.environ.get("HOME", ""),
           "GOPATH": os.environ.get("GOPATH", str(dest_dir / "gopath")),
           "GOCACHE": str(dest_dir / "gocache")}
    proc = subprocess.run(
        ["go", "build", "-o", str(dest), "./cmd/ai-specs"],
        cwd=ROOT, env=env, text=True, capture_output=True, check=False)
    if proc.returncode != 0:
        sys.stderr.write(f"parity: go build failed — identical-by-shim "
                         f"fallback\n{proc.stderr}\n")
        return None
    return dest


def build_gate_binary(dest_dir: Path) -> Path | None:
    """Build the nested worktree-gate module (CGO_ENABLED=0) into dest_dir.

    Returns the binary path, or None when go is unavailable or the build
    fails. The module is `catalog/recipes/worktree-flow/gate` (its own go.mod,
    module ai-specs.dev/worktree-gate) and is built with the same hermetic
    temp GOCACHE/GOPATH pattern as build_go_binary.

    Policy is the CALLER's: the wired suite entry (run.py) requires this build
    in its default path and fails loudly (exit 2); the explicit --self-test
    mode reports gate-present as unavailable instead. Build artifacts stay in
    the temp dir and are never committed.
    """
    if shutil.which("go") is None:
        return None
    dest = dest_dir / "worktree-gate"
    env = {**BASE_ENV, "CGO_ENABLED": "0", "HOME": os.environ.get("HOME", ""),
           "GOPATH": os.environ.get("GOPATH", str(dest_dir / "gopath")),
           "GOCACHE": str(dest_dir / "gocache")}
    proc = subprocess.run(
        ["go", "-C", str(GATE_MODULE_DIR), "build", "-o", str(dest), "."],
        env=env, text=True, capture_output=True, check=False)
    if proc.returncode != 0:
        sys.stderr.write(f"parity: worktree-gate build failed\n{proc.stderr}\n")
        return None
    return dest


# ── Legacy goldens (GO-20 U2e T2a) ────────────────────────────────────────
# The legacy leg is FROZEN as versioned goldens captured before the launcher
# flip: the harness compares the live Go leg against that captured legacy output
# instead of executing a legacy implementation at comparison time — there is no
# legacy implementation left to execute. The only live two-leg path left is the
# explicit --self-test mode (go_cli is None), which runs bin/ai-specs twice; that
# launcher is native since the flip, so the mode proves harness symmetry, not
# legacy execution.
GOLDENS_DIR = Path(__file__).resolve().parent / "goldens"

# Fixtures whose legacy output cannot be captured deterministically (an
# environment-dependent artifact no narrow normalization covers). Excluded
# fixtures run live in run_comparison so the corpus still measures them; they
# are never silently compared against a stale golden. Empty today; capture is
# retired (tests/parity/capture_goldens.py refuses), so an entry here is the
# only way to keep measuring a fixture whose golden is unusable.
GOLDEN_EXCLUDED: dict[str, str] = {}


def golden_path(fixture_name: str, gate_mode: str) -> Path:
    return GOLDENS_DIR / gate_mode / f"{fixture_name}.json"


def tree_to_json(tree: dict) -> list:
    """Snapshot dict -> sorted [[relpath, [kind, mode, payload]], ...]."""
    return [[rel, [kind, mode, payload]]
            for rel, (kind, mode, payload) in sorted(tree.items())]


def tree_from_json(items) -> dict:
    """Inverse of tree_to_json: relpath-keyed dict with tuple values."""
    return {rel: (kind, mode, payload)
            for rel, (kind, mode, payload) in items}


def load_golden_leg(fixture: Fixture, gate_mode: str) -> LegResult:
    """Load the frozen legacy leg for one fixture and gate mode.

    A missing golden for a fixture that is not GOLDEN_EXCLUDED is a hard
    error: the harness must never silently fall back to live-running a
    launcher in normal comparison mode. Capture is retired — no legacy
    implementation is left to capture from — so a missing golden is a repo
    defect (restore it from git), not something to regenerate here.
    """
    path = golden_path(fixture.name, gate_mode)
    if not path.is_file():
        raise RuntimeError(
            f"parity: no legacy golden for fixture {fixture.name!r} in gate "
            f"mode {gate_mode!r} ({path}); the corpus is frozen, so restore "
            f"it from git, or add the fixture to GOLDEN_EXCLUDED with a "
            f"reason so it runs live")
    data = json.loads(path.read_text(encoding="utf-8"))
    steps = []
    for item in data["steps"]:
        # Both legs go through the same normalizations (N10 states the rule; the
        # frozen text was captured WITH the normalizations of its time applied, so
        # re-applying them is idempotent — measured over all 35 fixtures in both
        # gate modes: only N12-gate-cache-size changes anything, and only the two
        # doctor-healthy goldens, whose stored byte count the frozen capture could
        # not mark). Harness artifacts and permanently unstable metrics are
        # canonicalized on both sides; no golden byte is rewritten.
        ctx = {"argv": tuple(item["argv"])}
        steps.append(StepResult(
            argv=tuple(item["argv"]), rc=item["rc"],
            stdout=apply_normalizations("stdout", item["stdout"], ctx),
            stderr=apply_normalizations("stderr", item["stderr"], ctx),
            tree=tree_from_json(item["tree"])))
    return LegResult(label="legacy", cli_path=data["legacy_launcher"],
                     steps=steps)


# ── Comparison runner ─────────────────────────────────────────────────────


def run_comparison(fixture: Fixture, go_cli: Path | None,
                   workdir: Path,
                   mutate=None, gate_mode: str = GATE_ABSENT,
                   gate_bin: Path | None = None,
                   live_legacy: bool | None = None) -> tuple[list[str], str]:
    """Run both legs for one fixture; return (deltas, mode-note).

    `mutate` is a TEST-ONLY hook applied to the second leg's results after
    capture (never used by the suite runner): it exists so the negative
    self-tests can deliberately inject a behavioral change and prove the
    harness flags it.

    `gate_mode` names the gate condition and `gate_bin` pins WORKTREE_GATE_BIN
    for BOTH legs when provided. The mode-note records the leg pairing AND the
    gate mode, so a zero-delta result can never be read without knowing which
    gate authority produced it.

    The legacy leg (leg_a) is loaded from the versioned golden unless
    `live_legacy` is true. `live_legacy=None` (the default) preserves the
    explicit --self-test contract: when `go_cli` is None both legs run
    bin/ai-specs live (the same native launcher twice since the flip). The
    `legacy` label names that frozen/launcher leg throughout; fixtures in
    GOLDEN_EXCLUDED always run live.
    """
    if live_legacy is None:
        live_legacy = go_cli is None
    scratch_a = workdir / "leg-legacy"
    scratch_b = workdir / "leg-other"
    if live_legacy or fixture.name in GOLDEN_EXCLUDED:
        leg_a = run_leg("legacy", ROOT / "bin" / "ai-specs", fixture,
                        scratch_a, gate_bin=gate_bin)
    else:
        leg_a = load_golden_leg(fixture, gate_mode)
    leg_mode = "legacy-vs-go"
    cli_b = go_cli if go_cli is not None else ROOT / "bin" / "ai-specs"
    if go_cli is None:
        leg_mode = "legacy-vs-legacy (identical-by-shim: explicit --self-test)"
    leg_b = run_leg("other", cli_b, fixture, scratch_b, gate_bin=gate_bin)
    if mutate is not None:
        leg_b = mutate(leg_b, scratch_b)
    return diff_legs(leg_a, leg_b), f"{leg_mode} | {gate_mode}"


def run_corpus(go_cli: Path | None, workdir: Path, fixtures=CORPUS,
               gate_mode: str = GATE_ABSENT,
               gate_bin: Path | None = None,
               live_legacy: bool | None = None) -> tuple[int, str]:
    """Run the corpus in one gate mode; return (exit_code, report).

    Exit 1 on ANY delta. `gate_mode`/`gate_bin` are recorded in the header and
    forwarded to every comparison, so the mode that produced each result is
    part of the report rather than ambient state. `live_legacy` is forwarded
    so the self-test path keeps running the legacy launcher for both legs.
    """
    lines: list[str] = ["differential parity harness", f"gate mode: {gate_mode}",
                        ""]
    failures = 0
    for fixture in fixtures:
        fdir = workdir / fixture.name
        fdir.mkdir(parents=True, exist_ok=True)
        try:
            deltas, mode = run_comparison(fixture, go_cli, fdir,
                                          gate_mode=gate_mode,
                                          gate_bin=gate_bin,
                                          live_legacy=live_legacy)
        except Exception as exc:  # a crashed leg is a delta too
            failures += 1
            lines.append(f"✗ {fixture.name}: harness error: {exc!r}")
            continue
        if deltas:
            failures += 1
            lines.append(f"✗ {fixture.name} [{mode}] — {len(deltas)} delta(s):")
            lines.extend(deltas)
        else:
            lines.append(f"✓ {fixture.name} [{mode}] — zero deltas")
        lines.append("")
    if go_cli is None:
        lines.append("note: identical-by-shim self-test mode (--self-test): both "
                     "legs run bin/ai-specs, which is the native launcher since "
                     "the GO-20 U2e flip — 'legacy-vs-legacy' is the historical "
                     "mode id, not a legacy execution; acceptance (a) path.")
    lines.append(f"fixtures: {len(fixtures)}, failing: {failures}")
    report = "\n".join(lines)
    return (1 if failures else 0), report


# ── S3 gap registry (GO-20 U2e T4) ────────────────────────────────────────
# Every verb the U2 S7 gap rule names must either be covered by a corpus step
# (`covers`: fixture name + argv prefix that the fixture must contain) or carry
# an explicit recorded reason (`recorded`) naming why it is not compared. The
# suite fails on a gap that is neither: silent gaps are not acceptable.
# A recorded reason is a divergence reported to a human decision, never a
# normalization: the corpus compares only what matches byte for byte.
S3_GAP_COVERAGE: dict[str, dict[str, object]] = {
    "hub": {
        "covers": [("hub-surface", ("hub",)), ("hub-surface", ("hub", "--help"))],
    },
    "upgrade": {
        "covers": [("upgrade-surface", ("upgrade", "--help")),
                   ("upgrade-surface", ("upgrade", "--bogus"))],
        "equivalent": [
            "upgrade --dry-run refusal (dev-channel guard, exit 2 on both legs). "
            "Driver: legacy runs lib/upgrade.sh and names that script in the "
            "message; Go runs its own binary and names it. Equivalent by decision: "
            "same exit code, the path names the running program. Not compared by "
            "the corpus (no normalization of product output in the harness).",
        ],
    },
    "skills": {
        "covers": [("skills-surface", ("skills",)),
                   ("skills-surface", ("skills", "list")),
                   ("skills-surface", ("skills", "remove"))],
    },
    "add-dep": {
        "covers": [("vendor-add-clone-failure", ("add-dep",)),
                   ("vendor-add-clone-failure", ("skills", "add"))],
    },
    "configure-recipes": {
        "covers": [("configure-recipes-surface", ("configure-recipes", "--help")),
                   ("configure-recipes-surface", ("configure-recipes",))],
        "recorded": [
            "configure-recipes on an initialized project without a TTY, with "
            "rich/questionary not vendored (exit 3 on both legs). Contract: section 7 "
            "requires a TTY and exit 3, with no FROZEN marker on the message; no "
            "section 2 row. Evidence: legacy lib/_internal/util.py ensure_deps returns "
            "3 silently before config_wizard.py prints the TTY line, so legacy stderr "
            "is empty; Go checks the TTY first and prints 'configure-recipes requires "
            "an interactive TTY' on stderr. Registered, no product change.",
        ],
    },
    "refresh-bundled": {
        "recorded": [
            "standalone refresh-bundled is the no-op of compatibility decided by "
            "Robert (D26, U2b; contract line 258): 'sync is enough'. Go prints the "
            "embedded provenance and changes nothing; legacy flattens the cache, "
            "restamps the lock, and exits 1 in an uninitialized project where Go "
            "exits 0. Not comparable by design; the refresh that matters runs inside "
            "sync, covered by the sync fixtures. The help text that still describes "
            "the flatten is corrected in its own product PR, not here.",
        ],
    },
    "recipe remove": {
        "covers": [("recipe-remove-configure", ("recipe", "remove"))],
    },
    "recipe configure": {
        "covers": [("recipe-remove-configure", ("recipe", "configure"))],
    },
}
