"""GO-07 S19b: lib/init.sh's baseline brief on the native seam.

The seam (``AI_SPECS_GO_BIN``) is what the Go dispatcher exports to a script it
execs. These tests pin both directions at the shell boundary — a usable seam
runs the native verbs and spawns no Python for the brief, and a failing chain
still falls back to the placeholder with init at exit 0 — and then diff the two
routes' rendered bytes.
"""
from __future__ import annotations

import os
import re
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import _go_cli  # noqa: E402
from _blackbox import isolated_home  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]
LEGACY_CLI = ROOT / "bin" / "ai-specs"
PLACEHOLDER = "# AGENTS.md - Runtime context"
BRIEF_MODULES = ("brief-render-policy.py", "agents-render.py", "recipe-materialize.py")

# Golden of the legacy `init` route (U2e T3d). Captured from bin/ai-specs on a
# fresh target with an isolated home; the native binary must reproduce it. The
# legacy stderr is only the GO_*_BRIDGE_FALLBACK warnings of the Python path, which
# appear without a verified worktree-gate binary and are dropped at capture time, so the
# golden stderr is empty. Procedure and mapping: odd/tasks/go-20-u2e-flip.md (L17).
GOLDEN_DIR = ROOT / "tests" / "fixtures" / "init-brief"
_MCP_TEMP = re.compile(r"ai-specs-recipe-mcp-[A-Za-z0-9_]+\.json")
_FALLBACK_LINE = re.compile(r"^\s*!?\s*GO_[A-Z_]*BRIDGE_FALLBACK:.*\n?", re.M)


def _normalize(text: str, fixture: _Fixture) -> str:
    text = text.replace(str(fixture.target), "<TARGET>").replace(str(fixture.base), "<BASE>")
    return _MCP_TEMP.sub("<MCP_TEMP>", text)


def _drop_fallback_lines(text: str) -> str:
    return _FALLBACK_LINE.sub("", text)


class _Fixture:
    """A fresh target project, an isolated CLI home and a scratch tmpdir."""

    def __init__(self, test: unittest.TestCase, prefix: str):
        self.base = Path(tempfile.mkdtemp(prefix=prefix))
        test.addCleanup(shutil.rmtree, self.base, ignore_errors=True)
        self.home = isolated_home(self.base)
        self.tmp = self.base / "tmp"
        self.tmp.mkdir()
        (self.tmp / "home").mkdir()
        self.target = self.base / "project"
        self.target.mkdir()

    def env(self, *, seam: Path | None = None, path_bin: Path | None = None) -> dict:
        env = {
            "PATH": (str(path_bin) + os.pathsep if path_bin else "") + os.environ.get("PATH", ""),
            "HOME": str(self.tmp / "home"),
            "TMPDIR": str(self.tmp),
            "AI_SPECS_HOME": str(self.home),
            # The launcher execs the native binary (GO-20 U2e T2); pin it to the
            # build of this worktree so the route never depends on dist/.
            "AI_SPECS_GO_BIN": str(_go_cli.cli()),
            "AI_SPECS_NO_NETWORK": "1",
            "LC_ALL": "C",
            "LANG": "C",
        }
        if seam is not None:
            env["AI_SPECS_GO_BIN"] = str(seam)
        return env

    def run(self, cli: Path, *args: str, env: dict) -> subprocess.CompletedProcess:
        return subprocess.run(
            [str(cli), *args, str(self.target)],
            cwd=ROOT, env=env, text=True, capture_output=True, check=False, timeout=300,
        )


def _seam_stub(base: Path, *, chain_rc: int = 0) -> tuple[Path, Path]:
    """A seam stub: records its argv, answers the gate with `true`, then exits chain_rc."""
    log = base / "seam.log"
    stub = base / "ai-specs-seam"
    stub.write_text(
        "#!/bin/sh\n"
        f"printf '%s\\n' \"$*\" >> \"{log}\"\n"
        'if [ "$1" = brief-render-policy ]; then\n'
        "  echo true\n"
        "  exit 0\n"
        "fi\n"
        f"exit {chain_rc}\n",
        encoding="utf-8",
    )
    stub.chmod(0o755)
    return stub, log


def _recording_python(base: Path) -> tuple[Path, Path]:
    """A python3 that records its argv and delegates to the real interpreter."""
    bindir = base / "fake-bin"
    bindir.mkdir()
    log = base / "python.log"
    stub = bindir / "python3"
    stub.write_text(
        "#!/bin/sh\n"
        f"printf '%s\\n' \"$*\" >> \"{log}\"\n"
        f'exec "{sys.executable}" "$@"\n',
        encoding="utf-8",
    )
    stub.chmod(0o755)
    return bindir, log


class InitBriefRouteTests(unittest.TestCase):
    # GO-20 U2e T2 finding: the bash init seam (AI_SPECS_GO_BIN stub) no longer
    # exists once bin/ai-specs execs the binary; the stub receives `init` itself.
    # Owner: T5 (deletes lib/_internal and these seam tests). Expires with T5.
    @unittest.expectedFailure
    def test_usable_seam_runs_the_native_verbs_and_no_brief_python(self):
        fixture = _Fixture(self, "ai-specs-initbrief-route-")
        seam, seam_log = _seam_stub(fixture.base)
        bindir, python_log = _recording_python(fixture.base)

        proc = fixture.run(LEGACY_CLI, "init", env=fixture.env(seam=seam, path_bin=bindir))

        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertIn("✓ render AGENTS.md (baseline brief)", proc.stdout)
        calls = seam_log.read_text(encoding="utf-8").splitlines()
        self.assertEqual(len(calls), 2, calls)
        self.assertEqual(calls[0], f"brief-render-policy {fixture.target / 'ai-specs' / 'ai-specs.toml'}")
        self.assertTrue(
            calls[1].startswith(f"baseline-brief {fixture.target} --resolved-config-out "),
            calls[1],
        )
        python_calls = python_log.read_text(encoding="utf-8") if python_log.exists() else ""
        for module in BRIEF_MODULES:
            self.assertNotIn(module, python_calls, f"{module} still ran:\n{python_calls}")

    # Same finding and owner as above (T5); expires with T5.
    @unittest.expectedFailure
    def test_native_chain_failure_writes_the_placeholder(self):
        fixture = _Fixture(self, "ai-specs-initbrief-fail-")
        seam, _ = _seam_stub(fixture.base, chain_rc=1)

        proc = fixture.run(LEGACY_CLI, "init", env=fixture.env(seam=seam))

        self.assertEqual(proc.returncode, 0, proc.stderr)
        agents = (fixture.target / "AGENTS.md").read_text(encoding="utf-8")
        self.assertEqual(agents.strip(), PLACEHOLDER)
        self.assertIn("render skipped", proc.stderr)

    def test_both_routes_render_the_same_bytes(self):
        go_cli = _go_cli.cli()
        if go_cli == LEGACY_CLI:
            self.skipTest("the Go build is unavailable, so the two routes would be one program")
        legacy = _Fixture(self, "ai-specs-initbrief-legacy-")
        native = _Fixture(self, "ai-specs-initbrief-native-")

        legacy_proc = legacy.run(LEGACY_CLI, "init", env=legacy.env())
        native_proc = native.run(go_cli, "init", env=native.env())

        self.assertEqual(legacy_proc.returncode, 0, legacy_proc.stderr)
        self.assertEqual(native_proc.returncode, 0, native_proc.stderr)
        for label, proc in (("legacy", legacy_proc), ("native", native_proc)):
            self.assertIn("✓ render AGENTS.md (baseline brief)", proc.stdout, f"{label} route")
        self.assertEqual(
            (legacy.target / "AGENTS.md").read_bytes(),
            (native.target / "AGENTS.md").read_bytes(),
        )


class InitBriefGoldenTests(unittest.TestCase):
    def test_native_init_matches_the_legacy_golden(self):
        go_cli = _go_cli.cli()
        if go_cli == LEGACY_CLI:
            self.skipTest("the Go build is unavailable, so the golden would test the legacy route")
        fixture = _Fixture(self, "ai-specs-initbrief-golden-")

        proc = fixture.run(go_cli, "init", env=fixture.env())

        self.assertEqual(proc.returncode, 0, proc.stderr)
        golden_stdout = (GOLDEN_DIR / "stdout.golden.txt").read_text(encoding="utf-8")
        golden_stderr = (GOLDEN_DIR / "stderr.golden.txt").read_text(encoding="utf-8")
        golden_agents = (GOLDEN_DIR / "AGENTS.md.golden").read_bytes()
        self.assertEqual(_normalize(proc.stdout, fixture), golden_stdout)
        self.assertEqual(_normalize(proc.stderr, fixture), golden_stderr)
        self.assertEqual((fixture.target / "AGENTS.md").read_bytes(), golden_agents)


if __name__ == "__main__":
    unittest.main()
