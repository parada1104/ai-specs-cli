"""PATH-level fault injection for the sync contract suites.

Both spines — the native Go ``internal/sync`` and the legacy ``lib/sync.sh`` —
resolve ``python3`` and ``mktemp`` through PATH, so shadowing those two names
is an implementation-independent seam: one CLI test then covers whichever spine
is under test. A ``python3`` wrapper scripts a single module's stdout/stderr/rc;
a ``mktemp`` wrapper hands out recorded probe paths and can fail on demand.
"""
from __future__ import annotations

import os
import shutil
import stat
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
KEPANO_FIXTURE = ROOT / "tests" / "fixtures" / "kepano-obsidian-skills"

_PYTHON3_WRAPPER = """#!/bin/bash
# PATH shadow for python3: intercept the named modules, exec the real one.
STUB_DIR="{stub}"
for arg in "$@"; do
    name="${{arg##*/}}"
    if [[ -f "$STUB_DIR/intercept/$name" ]]; then
        printf '%s\\n' "$*" >> "$STUB_DIR/invocations.log"
        cat "$STUB_DIR/payload/$name.stdout"
        cat "$STUB_DIR/payload/$name.stderr" >&2
        exit "$(cat "$STUB_DIR/payload/$name.rc")"
    fi
done
exec "{real_python}" "$@"
"""

_MKTEMP_WRAPPER = """#!/bin/bash
# PATH shadow for mktemp: ignore every argument, hand out a probe path.
# macOS mktemp honours neither TMPDIR nor -t templates, so the spine execs the
# binary; shadowing it is the only portable way to control the temp sequence.
PROBE="{probe}"
CNT="$PROBE/.count"
n=$(( $(cat "$CNT" 2>/dev/null || echo 0) + 1 ))
echo "$n" > "$CNT"
fail_from="$(cat "$PROBE/.fail_from" 2>/dev/null || true)"
if [[ -n "$fail_from" && "$n" -ge "$fail_from" ]]; then
    exit 1
fi
path="$PROBE/temp$n"
: > "$path"
echo "$path"
"""


class SyncStubs:
    """Per-test PATH stub environment rooted at ``base``."""

    def __init__(self, base: Path):
        self.base = Path(base)
        self.bin = self.base / "bin"
        self.stub = self.base / "stub"
        self.payload = self.stub / "payload"
        self.probe = self.base / "probe"
        self.tmp = self.base / "tmp"
        self.home = self.base / "home"
        for directory in (self.bin, self.stub, self.payload, self.probe,
                          self.tmp, self.home):
            directory.mkdir(parents=True, exist_ok=True)
        (self.stub / "intercept").mkdir(exist_ok=True)
        (self.probe / ".count").write_text("0")

        # Resolve the real python3 BEFORE shadowing PATH, or the wrapper would
        # exec itself in an infinite loop.
        real_python = shutil.which("python3")
        if real_python is None:
            raise RuntimeError("python3 not found on PATH")
        self._real_python = str(Path(real_python).resolve())

        self._write_executable(
            self.bin / "python3",
            _PYTHON3_WRAPPER.format(stub=self.stub, real_python=self._real_python),
        )
        self._write_executable(
            self.bin / "mktemp",
            _MKTEMP_WRAPPER.format(probe=self.probe),
        )

    # ── configuration ────────────────────────────────────────────────────

    def intercept(self, module: str, *, rc: int = 0, stdout: str = "",
                  stderr: str = "") -> None:
        """Script ``module``'s stdout/stderr/rc.

        Payloads are written to files and ``cat``-ed by the wrapper, so they
        reach the child byte-exact (including trailing newlines) instead of
        losing them to ``$(...)``/``printf '%s'`` substitution.
        """
        (self.stub / "intercept" / module).write_text("")
        (self.payload / f"{module}.stdout").write_bytes(stdout.encode("utf-8"))
        (self.payload / f"{module}.stderr").write_bytes(stderr.encode("utf-8"))
        (self.payload / f"{module}.rc").write_text(str(rc))

    def mktemp_fail_from(self, n: int | None) -> None:
        """Fail the ``mktemp`` wrapper from call ``n`` onward (``None`` clears)."""
        marker = self.probe / ".fail_from"
        if n is None:
            marker.unlink(missing_ok=True)
        else:
            marker.write_text(str(n))

    # ── observation ──────────────────────────────────────────────────────

    def invocations(self) -> list[str]:
        """Every intercepted-module command line, in execution order."""
        log = self.stub / "invocations.log"
        if not log.exists():
            return []
        return log.read_text(encoding="utf-8").splitlines()

    def survivors(self) -> list[str]:
        """Probe temp files that still exist (``temp*``; the counter is not one)."""
        return sorted(p.name for p in self.probe.glob("temp*"))

    def created_count(self) -> int:
        """How many ``mktemp`` calls the wrapper has served so far."""
        counter = self.probe / ".count"
        if not counter.exists():
            return 0
        return int(counter.read_text().strip() or "0")

    # ── environment ──────────────────────────────────────────────────────

    def env(self, extra: dict | None = None) -> dict:
        """The hermetic env every CLI test runs under; ``extra`` wins last."""
        env = {
            **os.environ,
            "PATH": str(self.bin) + os.pathsep + os.environ.get("PATH", ""),
            "AI_SPECS_HOME": str(ROOT),
            "AI_SPECS_GATE_OFFLINE": "1",
            "AI_SPECS_NO_NETWORK": "1",
            "AI_SPECS_VENDOR_FIXTURE_ROOT": str(KEPANO_FIXTURE),
            "TMPDIR": str(self.tmp),
            "HOME": str(self.home),
            "LC_ALL": "C",
            "LANG": "C",
            "PYTHONDONTWRITEBYTECODE": "1",
        }
        if extra:
            env.update(extra)
        return env

    # ── internals ────────────────────────────────────────────────────────

    def _write_executable(self, path: Path, body: str) -> None:
        path.write_text(body, encoding="utf-8")
        path.chmod(path.stat().st_mode | stat.S_IEXEC | stat.S_IXGRP | stat.S_IXOTH)


def minimal_project(base: Path) -> Path:
    """Create a network-free manifest-only project (the parity fixture shape).

    Mirrors tests/parity/parity.py's ``idempotent-resync`` fixture: a manifest
    with one claude agent plus skills/ and commands/ dirs, which syncs without
    reaching the network.
    """
    project = Path(base) / "project"
    (project / "ai-specs" / "skills").mkdir(parents=True, exist_ok=True)
    (project / "ai-specs" / "commands").mkdir(parents=True, exist_ok=True)
    (project / "ai-specs" / "ai-specs.toml").write_text(
        "[project]\nname = 'stub-probe'\n\n[agents]\nenabled = ['claude']\n",
        encoding="utf-8",
    )
    return project
