#!/usr/bin/env python3
"""Shared pure-stdlib helpers for ai-specs CLI internals.

Import-time contract: stdlib only. rich/questionary are never imported at
module top — ``ensure_deps`` may import them lazily after the vendor gate.
"""

from __future__ import annotations

import os
import subprocess
import sys
from dataclasses import dataclass
from hashlib import sha256
from pathlib import Path

# Pin range keeps bootstrap reproducible without chasing every major.
DEPS_SPEC = ["rich>=13.0.0,<15", "questionary>=2.0.0,<2.1"]


def ai_specs_home() -> Path:
    env = os.environ.get("AI_SPECS_HOME")
    if env:
        return Path(env).resolve()
    return Path(__file__).resolve().parents[2]


def vendor_dir() -> Path:
    return ai_specs_home() / "lib" / "_vendor"


def is_initialized(root: Path) -> bool:
    """True when <root>/ai-specs/ai-specs.toml exists (thin Path check, not Doctor)."""
    return (root / "ai-specs" / "ai-specs.toml").is_file()


def is_internal_test_recipe(recipe_id: str) -> bool:
    """True for internal test fixtures that must not ship or install.

    Convention: directory/id prefix ``test-`` (hyphen). Used by hub recipe list,
    CLI ``recipe list``, init wizard/onboarding pickers, and ``recipe add`` /
    ``recipe init`` / materialize guards. Fixtures live under
    ``tests/fixtures/recipes/``, not the shipped catalog.
    """
    return recipe_id.startswith("test-")


def internal_test_recipe_message(recipe_id: str) -> str:
    """User-facing reject message for internal ``test-*`` recipe ids."""
    return (
        f"Recipe '{recipe_id}' is an internal test fixture and is not part of "
        "the public catalog."
    )


def ensure_deps(vendor: Path, *, prompt: bool = True) -> int | None:
    """Make rich + questionary importable. Returns exit code 3 if unavailable, else None.

    Body moved from init_tui._ensure_deps, parameterized by ``vendor`` instead of
    calling ``_vendor_dir()`` internally.
    """
    if vendor.is_dir():
        sys.path.insert(0, str(vendor))

    try:
        import questionary  # noqa: F401
        from rich.console import Console  # noqa: F401
        from rich.panel import Panel  # noqa: F401

        return None
    except ImportError:
        pass

    if not prompt or not sys.stdin.isatty() or not sys.stdout.isatty():
        return 3

    print("Interactive init needs 'rich' + 'questionary' packages.")
    print(f"Install into {vendor}? [Y/n] ", end="", flush=True)
    answer = (sys.stdin.readline() or "").strip().lower()
    if answer not in ("", "y", "yes"):
        print("Skipping interactive init.")
        return 3

    try:
        vendor.mkdir(parents=True, exist_ok=True)
    except OSError as exc:
        print(f"ERROR: cannot create vendor dir {vendor}: {exc}", file=sys.stderr)
        return 3

    cmd = [
        sys.executable,
        "-m",
        "pip",
        "install",
        "--quiet",
        "--target",
        str(vendor),
        *DEPS_SPEC,
    ]
    print("▸ installing dependencies…")
    try:
        subprocess.run(cmd, check=True)
    except (subprocess.CalledProcessError, OSError) as exc:
        print(f"ERROR: could not install dependencies: {exc}", file=sys.stderr)
        return 3

    sys.path.insert(0, str(vendor))
    try:
        import questionary  # noqa: F401
        from rich.console import Console  # noqa: F401
        from rich.panel import Panel  # noqa: F401
    except ImportError as exc:
        print(f"ERROR: dependencies still unavailable after install: {exc}", file=sys.stderr)
        return 3
    return None


@dataclass(frozen=True)
class TopologyResolution:
    """Resolved repo topology for worktree-flow surfaces."""

    resolved: str  # "standalone" | "monorepo-apps" | "monorepo-submodules"
    configured: str  # "auto" | one of the above
    via: str  # "config" (explicit) | "auto" (detected)
    submodules: tuple[str, ...]  # initialized submodule paths (rel to repo_root)
    gitmodules_present: bool


def _run_git_config_paths(repo_root: Path) -> set[str]:
    """Return submodule paths registered in ``.gitmodules`` via git config."""
    try:
        proc = subprocess.run(
            [
                "git",
                "-C",
                str(repo_root),
                "config",
                "-f",
                ".gitmodules",
                "--get-regexp",
                r"^submodule\..*\.path$",
            ],
            capture_output=True,
            text=True,
            check=False,
        )
    except (OSError, FileNotFoundError):
        return set()
    if proc.returncode != 0:
        return set()
    paths: set[str] = set()
    for line in proc.stdout.splitlines():
        line = line.strip()
        if not line:
            continue
        # "submodule.<name>.path <path>"
        parts = line.split(None, 1)
        if len(parts) != 2:
            continue
        paths.add(parts[1].strip())
    return paths


def _run_submodule_status(repo_root: Path) -> list[str]:
    """Return raw ``git submodule status`` lines (non-recursive)."""
    try:
        proc = subprocess.run(
            ["git", "-C", str(repo_root), "submodule", "status"],
            capture_output=True,
            text=True,
            check=False,
        )
    except (OSError, FileNotFoundError):
        return []
    if proc.returncode != 0:
        return []
    return proc.stdout.splitlines()


def detect_submodules(repo_root: Path) -> tuple[bool, tuple[str, ...]]:
    """Return ``(gitmodules_present, initialized_submodule_paths)``.

    Pure inspection; no worktree/branch mutation. Non-recursive (v1).
    Prefixes ``' '``, ``'+'``, ``'U'`` count as initialized; ``'-'`` is skipped.
    """
    gm = repo_root / ".gitmodules"
    if not gm.is_file():
        return (False, ())

    registered_paths = _run_git_config_paths(repo_root)
    initialized: list[str] = []
    for line in _run_submodule_status(repo_root):
        if not line:
            continue
        prefix = line[0]
        rest = line[1:].split()
        if len(rest) < 2:
            continue
        path = rest[1]
        if path not in registered_paths:
            continue
        if prefix != "-":
            initialized.append(path)
    return (True, tuple(sorted(initialized)))


def resolve_repo_topology(
    repo_root: Path, config_value: str = "auto"
) -> TopologyResolution:
    """Resolve configured/auto topology for a project root.

    ``auto`` never resolves to ``monorepo-apps``. Git failures degrade to
    ``standalone`` without raising.
    """
    configured = (config_value or "auto").strip() or "auto"

    if configured in ("standalone", "monorepo-apps"):
        return TopologyResolution(configured, configured, "config", (), False)

    try:
        if configured == "monorepo-submodules":
            present, subs = detect_submodules(repo_root)
            return TopologyResolution(
                "monorepo-submodules", configured, "config", subs, present
            )

        # configured == "auto"
        present, subs = detect_submodules(repo_root)
        resolved = "monorepo-submodules" if subs else "standalone"
        return TopologyResolution(resolved, "auto", "auto", subs, present)
    except (OSError, subprocess.SubprocessError):
        via = "auto" if configured == "auto" else "config"
        return TopologyResolution("standalone", configured, via, (), False)


class SubrepoResolutionError(ValueError):
    """Raised when ``resolve_subrepo`` cannot pick a valid submodule path."""


def _superproject_root(owner_root: Path) -> Path | None:
    """Prove the superproject root of a submodule-owned repository.

    Reads the ``.git`` **file** (absorbed layouts: submodule primaries and
    linked worktrees) and walks the recorded gitdir for a ``.git/modules`` or
    ``.git/worktrees`` marker whose prefix is the superproject's ``.git``
    directory. Returns None when the owner is a regular repo (``.git`` dir),
    the marker is absent, or the candidate superproject cannot be proven
    (missing ``.git`` dir or ``.gitmodules``).
    """
    git_file = owner_root / ".git"
    if not git_file.is_file():
        return None
    try:
        content = git_file.read_text().strip()
    except OSError:
        return None
    if not content.startswith("gitdir:"):
        return None
    gitdir = content[len("gitdir:"):].strip()
    gitdir_path = Path(gitdir)
    if not gitdir_path.is_absolute():
        gitdir_path = owner_root / gitdir_path
    gitdir_path = gitdir_path.resolve()

    parts = gitdir_path.parts
    candidate: Path | None = None
    for index in range(len(parts) - 1):
        if parts[index] == ".git" and parts[index + 1] in ("modules", "worktrees"):
            candidate = Path(*parts[:index])
    if candidate is None:
        return None
    if not candidate.is_dir():
        return None
    if not (candidate / ".git").is_dir():
        return None
    if not (candidate / ".gitmodules").is_file():
        return None
    return candidate


@dataclass(frozen=True)
class RequestContext:
    """One explicit ai-specs request context (owner, topology, planning root).

    Separates the code/VCS owner from the canonical planning-artifact root:
    a subrepo request owns the submodule but plans under the proven
    superproject; a superrepo request owns and plans under the superproject.
    """

    owner_root: Path
    topology: TopologyResolution
    subrepo_path: str | None
    planning_root: Path
    worktrees_dir: str

    def as_dict(self) -> dict:
        return {
            "owner_root": str(self.owner_root),
            "topology": {
                "resolved": self.topology.resolved,
                "via": self.topology.via,
            },
            "subrepo_path": self.subrepo_path,
            "planning_root": str(self.planning_root),
            "worktrees_dir": self.worktrees_dir,
        }


def resolve_request_context(
    cwd: Path | str,
    explicit_subrepo: str | None = None,
    worktrees_dir: str = ".worktrees",
    configured_topology: str = "auto",
) -> RequestContext:
    """Resolve one ai-specs request context from proven Git facts.

    ``owner_root`` is the repository owning code/VCS work for the request
    (``git rev-parse --show-toplevel`` of ``cwd``). Under a proven
    ``monorepo-submodules`` topology, ``subrepo_path`` is validated via
    ``resolve_subrepo`` (path-first, then unique name, initialized) and the
    planning root is the proven superproject; every rejection hard-errors
    with a diagnostic before any create. Missing, uninitialized, or
    unprovable topology fails safe: no owner inference beyond the toplevel,
    no planning-root exception (planning root stays the owner root).
    """
    base = Path(cwd).resolve()
    top = _git_show_toplevel(base)
    owner_root = (top or base).resolve()

    super_root = _superproject_root(owner_root) or owner_root
    topology = resolve_repo_topology(super_root, configured_topology)

    subrepo_path: str | None = None
    planning_root = owner_root
    if topology.resolved == "monorepo-submodules":
        subrepo_path = resolve_subrepo(
            super_root,
            worktrees_dir,
            topology.submodules,
            base,
            explicit_subrepo,
            parse_gitmodules_entries(super_root),
        )
        planning_root = super_root

    return RequestContext(
        owner_root=owner_root,
        topology=topology,
        subrepo_path=subrepo_path,
        planning_root=planning_root,
        worktrees_dir=worktrees_dir,
    )



def parse_gitmodules_entries(repo_root: Path) -> tuple[tuple[str, str], ...]:
    """Return ``(name, path)`` pairs registered in ``.gitmodules``.

    Uses ``git config -f .gitmodules --get-regexp`` so parsing stays robust
    versus hand-rolled INI. Order follows git's output; names are the middle
    segment of ``submodule.<name>.path``.
    """
    try:
        proc = subprocess.run(
            [
                "git",
                "-C",
                str(repo_root),
                "config",
                "-f",
                ".gitmodules",
                "--get-regexp",
                r"^submodule\..*\.path$",
            ],
            capture_output=True,
            text=True,
            check=False,
        )
    except (OSError, FileNotFoundError):
        return ()
    if proc.returncode != 0:
        return ()
    entries: list[tuple[str, str]] = []
    for line in proc.stdout.splitlines():
        line = line.strip()
        if not line:
            continue
        parts = line.split(None, 1)
        if len(parts) != 2:
            continue
        key, path = parts[0].strip(), parts[1].strip()
        # key = submodule.<name>.path
        if not (key.startswith("submodule.") and key.endswith(".path")):
            continue
        name = key[len("submodule.") : -len(".path")]
        if name and path:
            entries.append((name, path))
    return tuple(entries)


def _git_show_toplevel(cwd: Path) -> Path | None:
    """Return ``git -C <cwd> rev-parse --show-toplevel`` or None on failure."""
    try:
        proc = subprocess.run(
            ["git", "-C", str(cwd), "rev-parse", "--show-toplevel"],
            capture_output=True,
            text=True,
            check=False,
        )
    except (OSError, FileNotFoundError):
        return None
    if proc.returncode != 0:
        return None
    top = proc.stdout.strip()
    return Path(top) if top else None


def _infer_subrepo_from_cwd(
    super_root: Path,
    worktrees_dir: str,
    initialized_paths: tuple[str, ...],
    cwd: Path,
) -> str | None:
    """Infer submodule path from cwd (design §2a). No superproject-working-tree."""
    top = _git_show_toplevel(cwd)
    if top is None:
        return None
    try:
        super_res = super_root.resolve()
        top_res = top.resolve()
        rel = os.path.relpath(str(top_res), str(super_res))
    except (OSError, ValueError):
        return None
    if rel.startswith(".."):
        # Not under the superproject.
        return None
    rel_posix = Path(rel).as_posix()
    if rel_posix in initialized_paths:
        return rel_posix  # primary submodule checkout

    # linked worktree: top == <super>/<worktrees_dir>/<name>-<slug>
    base = top_res.name
    try:
        parent = Path(os.path.relpath(str(top_res.parent), str(super_res))).as_posix()
    except (OSError, ValueError):
        return None
    wt_dir = worktrees_dir.rstrip("/")
    if parent != wt_dir:
        return None
    cands = [p for p in initialized_paths if base.startswith(p + "-")]
    if not cands:
        return None
    return max(cands, key=len)


def resolve_subrepo(
    super_root: Path,
    worktrees_dir: str,
    initialized_paths: tuple[str, ...],
    cwd: Path,
    explicit: str | None,
    gitmodules_entries: tuple[tuple[str, str], ...] | list[tuple[str, str]],
) -> str:
    """Resolve ``<subrepo>`` for monorepo-submodules ``/worktree-new`` (design §2).

    Returns the submodule **path** (not name). Raises ``SubrepoResolutionError``
    with a diagnostic message on every rejection path:
    missing inference, explicit/inferred mismatch, unknown, ambiguous name,
    or uninitialized.
    """
    entries = list(gitmodules_entries)
    registered_paths = {p for (_n, p) in entries}

    inferred = _infer_subrepo_from_cwd(
        super_root, worktrees_dir, initialized_paths, cwd
    )

    explicit_norm = (explicit or "").strip() or None

    if explicit_norm and inferred and explicit_norm != inferred:
        raise SubrepoResolutionError(
            f"cwd is inside submodule '{inferred}' but you passed '{explicit_norm}'"
        )

    subrepo = explicit_norm or inferred
    if not subrepo:
        raise SubrepoResolutionError(
            "monorepo-submodules: pass <subrepo> (cannot infer from cwd)"
        )

    # Validate against .gitmodules: path first, then unique name.
    if subrepo in registered_paths:
        resolved_path = subrepo
    else:
        by_name = [p for (name, p) in entries if name == subrepo]
        if len(by_name) == 0:
            raise SubrepoResolutionError(f"unknown submodule '{subrepo}'")
        if len(by_name) > 1:
            raise SubrepoResolutionError(
                f"ambiguous name '{subrepo}'; use its path instead"
            )
        resolved_path = by_name[0]

    if resolved_path not in initialized_paths:
        raise SubrepoResolutionError(
            f"submodule '{resolved_path}' not initialized; run "
            f"git submodule update --init {resolved_path}"
        )

    return resolved_path



def override_is_stale(catalog_src: Path, materialized_dest: Path) -> bool:
    """True when a not_exists override exists but no longer matches catalog bytes.

    Missing dest or missing catalog src → not stale (fresh-copy / no-op path).
    Compares content via sha256, not mtime.
    """
    if not materialized_dest.is_file() or not catalog_src.is_file():
        return False
    return (
        sha256(catalog_src.read_bytes()).digest()
        != sha256(materialized_dest.read_bytes()).digest()
    )


OVERRIDE_POLICIES = ("auto", "confirm", "never-force")
REPO_TOPOLOGY_PLACEHOLDER = "__WORKTREE_REPO_TOPOLOGY__"


def normalized_bytes(data: bytes) -> bytes:
    return data.replace(b"\r\n", b"\n")


def sha256_bytes(data: bytes) -> str:
    return sha256(normalized_bytes(data)).hexdigest()


def render_override_bytes(catalog_src: Path, merged_cfg: dict | None = None) -> bytes:
    """Return the bytes the CLI would write for a template target."""
    data = catalog_src.read_bytes()
    token = REPO_TOPOLOGY_PLACEHOLDER.encode()
    if token not in data:
        return data
    topology = "auto"
    if merged_cfg is not None:
        topology = str(merged_cfg.get("repo_topology", "auto"))
    return data.replace(token, topology.encode())


def classify_managed_override(
    materialized_dest: Path,
    managed_entry: dict | None = None,
    catalog_src: Path | bytes | None = None,
    would_write: bytes | str | None = None,
) -> str:
    """Classify a governed target from disk, lock metadata, and would-write bytes.

    ``catalog_src`` accepts a source path for convenience; callers with rendered
    placeholder content should pass ``would_write`` so comparison uses the exact
    post-render bytes that sync would write.
    """
    if not materialized_dest.is_file():
        return "missing"
    disk_sha = sha256_bytes(materialized_dest.read_bytes())
    if not isinstance(managed_entry, dict) or not managed_entry.get("sha256"):
        return "untracked"
    if disk_sha != str(managed_entry["sha256"]):
        return "user_modified"

    if would_write is None and isinstance(catalog_src, (bytes, str)):
        would_write = catalog_src
        catalog_src = None
    if would_write is None and isinstance(catalog_src, Path) and catalog_src.is_file():
        would_write = render_override_bytes(catalog_src)
    if would_write is None:
        return "managed_current"
    if isinstance(would_write, str):
        would_write = would_write.encode()
    return "managed_current" if disk_sha == sha256_bytes(would_write) else "managed_stale"
