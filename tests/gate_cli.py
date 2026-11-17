"""Build the CLI once and expose its worktree-gate route to Python tests."""
from __future__ import annotations

import atexit
import os
import shlex
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
_TMP = None
_BINARY: Path | None = None
_WRAPPER: Path | None = None


def gate_binary() -> Path:
    """Return the process-shared CLI binary, building it outside the worktree."""
    global _TMP, _BINARY, _WRAPPER
    if _BINARY is not None:
        return _BINARY
    go = shutil.which("go")
    if go is None:
        raise unittest.SkipTest("Go toolchain unavailable; CLI gate tests require go")
    _TMP = tempfile.TemporaryDirectory(prefix="ai-specs-gate-cli-")
    atexit.register(_TMP.cleanup)
    dist = Path(_TMP.name) / "dist"
    dist.mkdir()
    _BINARY = dist / "ai-specs"
    subprocess.run([go, "build", "-o", str(_BINARY), "./cmd/ai-specs"],
                   cwd=ROOT, check=True)
    _WRAPPER = dist / "worktree-gate-current"
    _WRAPPER.write_text(
        "#!/bin/sh\nexec " + shlex.quote(str(_BINARY)) + " worktree-gate \"$@\"\n",
        encoding="utf-8",
    )
    _WRAPPER.chmod(0o755)
    return _BINARY


def gate_command() -> Path:
    gate_binary()
    assert _WRAPPER is not None
    return _WRAPPER


def install_cli(home: Path) -> Path:
    """Point a test's AI_SPECS_HOME/dist/ai-specs at the shared build."""
    binary = gate_binary()
    target = home / "dist" / "ai-specs"
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_bytes(binary.read_bytes())
    target.chmod(0o755)
    return target
