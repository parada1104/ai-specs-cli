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
