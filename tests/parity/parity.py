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
  * the Go build side, when absent or failing to build, falls back to the
    legacy launcher for BOTH legs ("identical-by-shim" self-test mode):
    legacy-vs-legacy must then report ZERO deltas (acceptance a).

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
}


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
                      "stderr is compared verbatim.",
        fn=_norm_help_stderr,
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


def run_leg(label: str, cli_path: Path, fixture: Fixture, scratch: Path) -> LegResult:
    """Run one implementation against one fixture in an isolated scratch root."""
    home = make_home(scratch / "home")
    home_str = str(home)
    project = scratch / "project"
    project.mkdir(parents=True)
    fixture.setup(project)
    ctx = {"project_root": str(project), "home": home_str,
           "scratch": str(scratch)}
    env = {**BASE_ENV, "HOME": str(scratch / "user-home"), "TMPDIR": str(scratch),
           "AI_SPECS_HOME": home_str}
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
    fails — the caller then falls back to the identical-by-shim self-test
    (both legs = legacy launcher), which must report ZERO deltas.
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


# ── Comparison runner ─────────────────────────────────────────────────────


def run_comparison(fixture: Fixture, go_cli: Path | None,
                   workdir: Path,
                   mutate=None) -> tuple[list[str], str]:
    """Run both legs for one fixture; return (deltas, mode-note).

    `mutate` is a TEST-ONLY hook applied to the second leg's results after
    capture (never used by the suite runner): it exists so the negative
    self-tests can deliberately inject a behavioral change and prove the
    harness flags it.
    """
    scratch_a = workdir / "leg-legacy"
    scratch_b = workdir / "leg-other"
    leg_a = run_leg("legacy", ROOT / "bin" / "ai-specs", fixture, scratch_a)
    mode = "legacy-vs-go"
    cli_b = go_cli if go_cli is not None else ROOT / "bin" / "ai-specs"
    if go_cli is None:
        mode = "legacy-vs-legacy (identical-by-shim: Go build absent)"
    leg_b = run_leg("other", cli_b, fixture, scratch_b)
    if mutate is not None:
        leg_b = mutate(leg_b, scratch_b)
    return diff_legs(leg_a, leg_b), mode


def run_corpus(go_cli: Path | None, workdir: Path, fixtures=CORPUS) -> tuple[int, str]:
    """Run the corpus; return (exit_code, report). Exit 1 on ANY delta."""
    lines: list[str] = ["differential parity harness", ""]
    failures = 0
    for fixture in fixtures:
        fdir = workdir / fixture.name
        fdir.mkdir(parents=True, exist_ok=True)
        try:
            deltas, mode = run_comparison(fixture, go_cli, fdir)
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
        lines.append("note: Go build unavailable — identical-by-shim self-test "
                     "mode (both legs legacy); acceptance (a) path.")
    lines.append(f"fixtures: {len(fixtures)}, failing: {failures}")
    report = "\n".join(lines)
    return (1 if failures else 0), report
