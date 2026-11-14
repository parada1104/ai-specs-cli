"""Build-once access to the authoritative Go CLI binary.

The three sync contract suites must exercise the native ``sync`` route in
``cmd/ai-specs``. ``bin/ai-specs`` is the legacy Bash launcher kept only for
the parity harness's legacy leg, so driving it would test the orphaned shell
script instead of the Go port. This module builds the Go binary once per
process and falls back to the legacy launcher LOUDLY (never silently) when the
build is unavailable, so a go-less machine still runs the suite but is told it
is no longer gating the port.
"""
from __future__ import annotations

import os
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
LEGACY_CLI = ROOT / "bin" / "ai-specs"

# One loud line: a silent fallback would let the suite pass without ever
# exercising the Go spine.
_FALLBACK_WARNING = (
    "sync contract suites: WARNING — the Go build (cmd/ai-specs) is "
    "unavailable; falling back to bin/ai-specs, so these suites DO NOT gate "
    "the Go port.\n"
)

_cli_path: Path | None = None


def cli() -> Path:
    """Return the native Go CLI, building it once per process."""
    global _cli_path
    if _cli_path is None:
        built = _build()
        _cli_path = built if built is not None else LEGACY_CLI
    return _cli_path


def legacy_cli() -> Path:
    """Return the legacy Bash launcher (parity harness's legacy leg)."""
    return LEGACY_CLI


def _build() -> Path | None:
    if shutil.which("go") is None:
        sys.stderr.write(_FALLBACK_WARNING)
        return None
    dest = Path(tempfile.mkdtemp(prefix="ai-specs-go-cli-")) / "ai-specs"
    env = {**os.environ, "CGO_ENABLED": "0"}
    try:
        proc = subprocess.run(
            ["go", "build", "-o", str(dest), "./cmd/ai-specs"],
            cwd=ROOT,
            env=env,
            text=True,
            capture_output=True,
            check=False,
        )
    except OSError:
        sys.stderr.write(_FALLBACK_WARNING)
        return None
    if proc.returncode != 0:
        sys.stderr.write(_FALLBACK_WARNING)
        return None
    return dest
