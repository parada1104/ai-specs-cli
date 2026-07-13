#!/usr/bin/env python3
"""Shared pure-stdlib helpers for ai-specs CLI internals.

Import-time contract: stdlib only. rich/questionary are never imported at
module top — ``ensure_deps`` may import them lazily after the vendor gate.
"""

from __future__ import annotations

import os
import subprocess
import sys
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
