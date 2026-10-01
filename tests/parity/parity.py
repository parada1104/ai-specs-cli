"""Differential parity harness: legacy (Bash/Python) vs Go single-binary.

Card [Go 03] — Trello 6a84e77110b9ac751af676f4.

Purpose
-------
Run the SAME fixture through TWO implementations of the ai-specs CLI —
the legacy launcher ``bin/ai-specs`` (this tree's Bash/Python implementation,
frozen at the checked-out revision) and the Go root-module binary
(``cmd/ai-specs``, card 04) — and diff:

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
  * the Go build side never silently substitutes the legacy launcher in
    the wired CI phase: run.py REQUIRES the build and fails loudly (exit 2)
    when go is absent or the build fails; the legacy-vs-legacy
    "identical-by-shim" mode (both legs legacy, must report ZERO deltas,
    acceptance a) exists ONLY as run.py's explicit ``--self-test`` flag,
    never as a silent fallback;
  * the corpus runs in TWO gate modes (plan finding F3 / risk R7):
    ``gate-absent`` (no verified gate binary, every bridge takes its Python
    fallback) and ``gate-present`` (a locally built worktree-gate pinned via
    ``WORKTREE_GATE_BIN`` for both legs, exercising the Go-authority path).
    The mode is recorded in the report, never ambient; the built binary stays
    in the temp dir and is never committed.

The known help-heredoc quirk (card 04 finding) and every normalization rule
carry a written justification in NORMALIZATIONS below; nothing is silently
regex-silenced.
"""
from __future__ import annotations

import hashlib
import os
import re
import shutil
import subprocess
import sys
import tempfile
from dataclasses import dataclass, field
from pathlib import Path

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
    # friends): the legacy leg ignores these vars entirely, while the Go spine
    # routes the named step through its native implementation.
    **{k: v for k, v in os.environ.items() if k.startswith("GO_SYNC_STEP_")},
}

# Gate modes: whether the corpus runs with a locally built worktree-gate binary
# pinned via WORKTREE_GATE_BIN for BOTH legs. `gate-absent` is the historical
# behavior (isolated home with an empty cache/ and no binary, so every bridge
# degrades to the Python fallback); `gate-present` builds the nested
# worktree-flow gate module once and exercises the real Go-authority bridge
# path (plan finding F3 / risk R7). AI_SPECS_GATE_OFFLINE stays set in both
# modes: it only prevents NETWORK acquisition, never a pinned local binary.
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
    fn: object                          # (text, ctx) -> text


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


# The legacy Python modules announce their degraded Python authority on stderr
# when no verified worktree-gate binary is present in the install. Anchored to
# the two exact notice prefixes; nothing else is touched.
_BRIDGE_FALLBACK_NOTICE_RE = re.compile(
    r"^(?:  ! )?GO_[A-Z_]+_BRIDGE_FALLBACK: .*$\n?", re.MULTILINE)


def _norm_bridge_fallback_notice(text: str, ctx: dict) -> str:
    return _BRIDGE_FALLBACK_NOTICE_RE.sub("", text)


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
        rule_id="N7-legacy-bridge-fallback-notice",
        surfaces=("stderr",),
        description="Remove only the legacy GO_*_BRIDGE_FALLBACK announcement "
                    "lines, with or without the '  ! ' prefix, from stderr.",
        justification="The legacy Python modules announce their degraded "
                      "Python authority on stderr when no verified "
                      "worktree-gate binary is present in the install "
                      "(`GO_RESOLVED_CONFIG_BRIDGE_FALLBACK`, "
                      "`GO_CLASSIFY_OVERRIDE_BRIDGE_FALLBACK`, and the other "
                      "`GO_*_BRIDGE_FALLBACK` notices). The Go port performs "
                      "those projections and classifications natively, so it "
                      "has no temporary authority to announce and must never "
                      "claim to be using Python. Only the announcement lines "
                      "are removed; the check roster, severities, messages, "
                      "exit codes and report formatting stay compared "
                      "verbatim.",
        fn=_norm_bridge_fallback_notice,
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
    setup: object              # (project_root: Path) -> None
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


def _setup_brief_render_false(project: Path) -> None:
    """[brief].render = false: sync's policy gate must skip the AGENTS.md step
    entirely (no file written, skip notice on stdout)."""
    _write(project, "ai-specs/ai-specs.toml",
           _manifest(extra="\n[brief]\nrender = false\n"))
    (project / "ai-specs" / "skills").mkdir(parents=True, exist_ok=True)
    (project / "ai-specs" / "commands").mkdir(exist_ok=True)


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
    """Isolated install root with a REAL lib copy.

    Reuses _blackbox.isolated_home (symlinked install entries + local cache/)
    but materializes lib/ as a real copy: doctor.py derives its cache root
    from its own realpath, so a symlinked lib would resolve back into the
    repository and read repo cache state (proven in tests/test_doctor.py).
    The copy is made from this checkout's frozen revision — the legacy
    reference implementation for BOTH legs.
    """
    home = bb.isolated_home(base, catalog=True)
    (home / "lib").unlink()
    shutil.copytree(ROOT / "lib", home / "lib", symlinks=True,
                    ignore=shutil.ignore_patterns("_vendor"))
    return home


def run_leg(label: str, cli_path: Path, fixture: Fixture, scratch: Path,
            gate_bin: Path | None = None) -> LegResult:
    """Run one implementation against one fixture in an isolated scratch root.

    ``gate_bin`` pins WORKTREE_GATE_BIN for the leg when provided (gate-present
    mode); when None the ambient isolated home has no verified gate binary and
    every bridge takes its Python fallback (gate-absent mode).
    """
    home = make_home(scratch / "home")
    home_str = str(home)
    project = scratch / "project"
    project.mkdir(parents=True)
    fixture.setup(project)
    ctx = {"project_root": str(project), "home": home_str,
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
    for i, (a, b) in enumerate(zip(legacy.steps, other.steps)):
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
    explicitly as the legacy-vs-legacy identical-by-shim mode (acceptance a).
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


# ── Comparison runner ─────────────────────────────────────────────────────


def run_comparison(fixture: Fixture, go_cli: Path | None,
                   workdir: Path,
                   mutate=None, gate_mode: str = GATE_ABSENT,
                   gate_bin: Path | None = None) -> tuple[list[str], str]:
    """Run both legs for one fixture; return (deltas, mode-note).

    `mutate` is a TEST-ONLY hook applied to the second leg's results after
    capture (never used by the suite runner): it exists so the negative
    self-tests can deliberately inject a behavioral change and prove the
    harness flags it.

    `gate_mode` names the gate condition and `gate_bin` pins WORKTREE_GATE_BIN
    for BOTH legs when provided. The mode-note records the leg pairing AND the
    gate mode, so a zero-delta result can never be read without knowing which
    gate authority produced it.
    """
    scratch_a = workdir / "leg-legacy"
    scratch_b = workdir / "leg-other"
    leg_a = run_leg("legacy", ROOT / "bin" / "ai-specs", fixture, scratch_a,
                    gate_bin=gate_bin)
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
               gate_bin: Path | None = None) -> tuple[int, str]:
    """Run the corpus in one gate mode; return (exit_code, report).

    Exit 1 on ANY delta. `gate_mode`/`gate_bin` are recorded in the header and
    forwarded to every comparison, so the mode that produced each result is
    part of the report rather than ambient state.
    """
    lines: list[str] = ["differential parity harness", f"gate mode: {gate_mode}",
                        ""]
    failures = 0
    for fixture in fixtures:
        fdir = workdir / fixture.name
        fdir.mkdir(parents=True, exist_ok=True)
        try:
            deltas, mode = run_comparison(fixture, go_cli, fdir,
                                          gate_mode=gate_mode, gate_bin=gate_bin)
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
                     "legs legacy; acceptance (a) path.")
    lines.append(f"fixtures: {len(fixtures)}, failing: {failures}")
    report = "\n".join(lines)
    return (1 if failures else 0), report
