"""Black-box CLI fixtures and filesystem observability helpers.

Every converted test drives the CLI through its process boundary:
``bin/ai-specs <verb>``. No test may import ``lib/_internal`` modules.
"""
from __future__ import annotations

import hashlib
import os
import re
import shutil
import subprocess
import tempfile
from dataclasses import dataclass
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"


@dataclass
class CLIResult:
    returncode: int
    stdout: str
    stderr: str


def cache_project_key(project_root: Path) -> str:
    """Return the frozen cache key: sha256(realpath)[:12]-sanitized basename."""
    real = str(project_root.resolve())
    basename = re.sub(r"[^A-Za-z0-9._-]+", "-", project_root.resolve().name).strip("-._") or "project"
    return f"{hashlib.sha256(real.encode()).hexdigest()[:12]}-{basename}"


def cache_project_dir(project_root: Path, cli_home: Path) -> Path:
    return cli_home / "cache" / "projects" / cache_project_key(project_root)


def normalize_output(text: str, roots: tuple[Path, ...] = ()) -> str:
    """Normalize path-dependent output while retaining ordering and content."""
    for root in roots:
        text = text.replace(str(root), "<TEMP>")
    return re.sub(r"/(?:private/)?var/folders/[^\s]+", "<TEMP>", text)


# Top-level entries of the install root that the CLI resolves at runtime.
# `cache` is deliberately excluded: it must be a real directory inside the
# isolated home so per-project cache writes land in temp, not in the repo.
_INSTALL_ENTRIES = (
    "bin", "lib", "catalog", "VERSION",
    "bundled-skills", "bundled-commands", "templates", "scripts",
)


def isolated_home(base: Path, *, catalog: bool = True) -> Path:
    """Build a fake CLI install root under `base`.

    Pass `catalog=False` for a home with an EMPTY catalog (no symlink to the
    repo catalog) — needed by tests that assert exact recipe sets.
    AI_SPECS_HOME is the CLI *install root* — bin/ai-specs resolves lib/*.sh
    under it — and it is ALSO the cache root
    ($AI_SPECS_HOME/cache/projects/<key>, see lib/_internal/project-cache.py).
    Pointing it at an empty directory therefore breaks the CLI outright
    (exit 127: "lib/doctor.sh: No such file or directory").

    Symlinking the install tree into a temp dir satisfies both needs at once:
    the CLI finds its own code, and `cache/` is local to the temp dir.
    """
    home = base / "cli-home"
    home.mkdir(parents=True, exist_ok=True)
    for entry in _INSTALL_ENTRIES:
        if entry == "catalog" and not catalog:
            continue
        src = ROOT / entry
        dest = home / entry
        if src.exists() and not dest.exists():
            dest.symlink_to(src)
    if not catalog:
        (home / "catalog" / "recipes").mkdir(parents=True, exist_ok=True)
    (home / "cache").mkdir(exist_ok=True)
    return home


def populate_catalog(cli_home: Path, recipe_id: str, toml: str | None = None) -> Path:
    """Seed a minimal catalog recipe into the isolated home.

    If cli_home/catalog is a symlink to the repo catalog, it is materialized
    into a real directory of per-recipe symlinks first, so test recipes never
    leak into the repository's own catalog. Returns the recipe directory.
    """
    recipes = cli_home / "catalog" / "recipes"
    if recipes.parent.is_symlink():
        real = cli_home / "catalog"
        target = real.resolve()
        real.unlink()
        real.mkdir(parents=True)
        for entry in target.iterdir():
            if entry.is_dir() and (entry / "recipe.toml").exists():
                (real / entry.name).symlink_to(entry)
            elif entry.name == "recipes":
                (real / "recipes").mkdir()  # real: per-recipe symlinks below
                for r in entry.iterdir():
                    (real / "recipes" / r.name).symlink_to(r)
            else:
                (real / entry.name).symlink_to(entry)
    recipes.mkdir(parents=True, exist_ok=True)
    rdir = recipes / recipe_id
    rdir.mkdir(parents=True, exist_ok=True)
    body = toml if toml is not None else (
        f'[recipe]\nid = "{recipe_id}"\ndescription = "test recipe {recipe_id}"\n'
        f'[init]\nworkflow = "echo init-{recipe_id}"\n'
    )
    (rdir / "recipe.toml").write_text(body)
    return rdir


def invoke(project_root: Path, *args: str, cli_home: Path | None = None,
           tmpdir: Path | None = None, append_root: bool = True,
           stdin: str | None = "") -> CLIResult:
    """Run bin/ai-specs hermetically against a project.

    stdin defaults to "" (closed pipe) so the CLI can never block on a
    prompt; pass explicit text only for tests that feed stdin deliberately.

    IMPORTANT — pass an explicit `cli_home` for any SEQUENCE of commands.
    With `cli_home=None` a fresh isolated install root is built per call, so
    state one command writes to the cache is invisible to the next. A
    `sync` then `doctor` sequence run that way reports phantom
    `bundled-skill ... missing` ERRORs and exits 1; sharing one home via
    `isolated_home()` gives the true result, exit 0.

    Args given here come BEFORE the project root (which is appended last,
    matching every verb's `[path]` positional). Pass `append_root=False`
    for verbs that take no project path (version, help, upgrade).
    """
    home_tmp = tempfile.TemporaryDirectory(prefix="ai-specs-home-") if cli_home is None else None
    tmp_tmp = tempfile.TemporaryDirectory(prefix="ai-specs-tmp-") if tmpdir is None else None
    home = isolated_home(Path(home_tmp.name)) if home_tmp else cli_home
    temp = Path(tmp_tmp.name) if tmp_tmp else tmpdir
    env = {
        "PATH": os.environ.get("PATH", ""),
        "HOME": str(temp / "home"),
        "TMPDIR": str(temp),
        "AI_SPECS_HOME": str(home),
        "AI_SPECS_NO_NETWORK": "1",
        "LC_ALL": "C",
        "LANG": "C",
    }
    (temp / "home").mkdir(parents=True, exist_ok=True)
    argv = [str(CLI), *args]
    if append_root:
        argv.append(str(project_root))
    try:
        proc = subprocess.run(argv, cwd=ROOT, env=env, text=True,
                              capture_output=True, check=False, input=stdin)
        return CLIResult(proc.returncode, normalize_output(proc.stdout, (project_root, home, temp)),
                         normalize_output(proc.stderr, (project_root, home, temp)))
    finally:
        if home_tmp:
            home_tmp.cleanup()
        if tmp_tmp:
            tmp_tmp.cleanup()


def snapshot(root: Path) -> dict[str, tuple[str, str]]:
    """Map relative paths to (kind, link target/content hash), including symlink kind."""
    result = {}
    for path in sorted(root.rglob("*")):
        rel = str(path.relative_to(root))
        if path.is_symlink():
            result[rel] = ("symlink", os.readlink(path))
        elif path.is_dir():
            result[rel] = ("dir", "")
        elif path.is_file():
            result[rel] = ("file", hashlib.sha256(path.read_bytes()).hexdigest())
    return result


def tree_diff(before: dict, after: dict) -> dict[str, list[str]]:
    return {
        "created": sorted(set(after) - set(before)),
        "deleted": sorted(set(before) - set(after)),
        "modified": sorted(k for k in set(before) & set(after) if before[k] != after[k]),
    }


def temp_project(*, name: str = "fixture", agents: tuple[str, ...] = ()):
    """Create a minimal isolated project; caller must cleanup the returned object."""
    td = tempfile.TemporaryDirectory(prefix="ai-specs-project-")
    root = Path(td.name)
    (root / "ai-specs" / "skills").mkdir(parents=True)
    (root / "ai-specs" / "commands").mkdir()
    enabled = ", ".join(repr(a) for a in agents)
    (root / "ai-specs" / "ai-specs.toml").write_text(
        f"[project]\nname = {name!r}\n\n[agents]\nenabled = [{enabled}]\n"
    )
    return td, root
