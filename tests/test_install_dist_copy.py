"""GO-20 U2e: the installed home carries a copy of the verified binary.

After the flip the repo launcher bin/ai-specs execs $AI_SPECS_HOME/dist/ai-specs.
install.sh therefore copies the SAME verified bytes it installed at
$INSTALL_BIN/ai-specs into the home's dist/ directory. This test drives the
installer exactly like tests/test_install_binary_entry.py does (hermetic fixture
repo + local HTTP release asset), so no network and no Go toolchain.

PR-B (the launcher flip) owns the assertion that "$AI_SPECS_HOME/bin/ai-specs"
(a BASH launcher today) execs the dist copy; this suite only proves the dist
copy exists, is executable, is byte-equal, and runs.
"""
from __future__ import annotations

import hashlib
import http.server
import os
import platform
import shutil
import socketserver
import subprocess
import tempfile
import threading
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
INSTALL_SH = ROOT / "install.sh"


def _platform_asset() -> str:
    system = platform.system()
    os_name = {"Darwin": "darwin", "Linux": "linux"}.get(system)
    machine = platform.machine().lower()
    arch = {"arm64": "arm64", "aarch64": "arm64", "x86_64": "amd64", "amd64": "amd64"}.get(machine)
    if os_name is None or arch is None:
        raise unittest.SkipTest(f"unsupported platform {system}/{machine}")
    return f"ai-specs-{os_name}-{arch}"


def _git(repo: Path, *args: str) -> None:
    env = {
        **os.environ,
        "GIT_AUTHOR_NAME": "t",
        "GIT_AUTHOR_EMAIL": "t@example.com",
        "GIT_COMMITTER_NAME": "t",
        "GIT_COMMITTER_EMAIL": "t@example.com",
    }
    subprocess.run(["git", *args], cwd=str(repo), env=env, check=True,
                   capture_output=True, text=True)


class _Server:
    """A minimal HTTP server over a directory, on a free port."""

    def __init__(self, directory: Path):
        handler = self._handler(directory)
        self.httpd = socketserver.TCPServer(("127.0.0.1", 0), handler)
        self.thread = threading.Thread(target=self.httpd.serve_forever, daemon=True)
        self.thread.start()

    @staticmethod
    def _handler(directory: Path):
        class Handler(http.server.SimpleHTTPRequestHandler):
            def __init__(self, *args, **kwargs):
                super().__init__(*args, directory=str(directory), **kwargs)

            def log_message(self, format, *args):
                pass

        return Handler

    @property
    def base_url(self) -> str:
        return f"http://127.0.0.1:{self.httpd.server_address[1]}"

    def close(self) -> None:
        self.httpd.shutdown()
        self.httpd.server_close()


class InstallDistCopyTests(unittest.TestCase):
    def _fixture_repo(self, base: Path, asset: str, digest: str) -> Path:
        source = base / "source"
        source.mkdir()
        (source / "VERSION").write_text("1.0.0\n", encoding="utf-8")
        (source / "bin").mkdir()
        (source / "bin" / "ai-specs").write_text("#!/bin/sh\nexit 0\n", encoding="utf-8")
        (source / "lib").mkdir()
        (source / "bin" / "SHA256SUMS").write_text(f"{digest}  {asset}\n", encoding="utf-8")
        _git(source, "init", "-b", "main")
        _git(source, "add", ".")
        _git(source, "commit", "-m", "init")
        return source

    def _run_install(self, base: Path, source: Path, server: _Server, home: Path,
                     install_bin: Path | None = None):
        env = {
            "PATH": os.environ.get("PATH", ""),
            "HOME": str(base / "userhome"),
            "AI_SPECS_HOME": str(home),
            "INSTALL_BIN": str(install_bin or (base / "bin")),
            "AI_SPECS_REPO": str(source),
            "AI_SPECS_REF": "main",
            "AI_SPECS_RELEASE_BASE": server.base_url,
            "AI_SPECS_TRUST_ROOT_FILE": str(home / "bin" / "SHA256SUMS"),
            "AI_SPECS_SKIP_TUI_DEPS": "1",
            "LC_ALL": "C",
            "LANG": "C",
        }
        (base / "userhome").mkdir(exist_ok=True)
        return subprocess.run(["bash", str(INSTALL_SH)], cwd=str(ROOT), env=env,
                              capture_output=True, text=True, check=False, timeout=300)

    def test_installer_copies_the_installed_binary_into_the_home_dist(self):
        asset = _platform_asset()
        payload = b"#!/bin/sh\necho 'ai-specs test-version 1.0.0'\n"
        digest = hashlib.sha256(payload).hexdigest()
        with tempfile.TemporaryDirectory(prefix="ai-specs-u2e-dist-") as tmp:
            base = Path(tmp)
            assets = base / "assets"
            assets.mkdir()
            (assets / asset).write_bytes(payload)
            server = _Server(assets)
            self.addCleanup(server.close)
            source = self._fixture_repo(base, asset, digest)
            home = base / "home"

            proc = self._run_install(base, source, server, home)

            self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
            entry = base / "bin" / "ai-specs"
            dist = home / "dist" / "ai-specs"
            self.assertTrue(dist.is_file(), proc.stdout + proc.stderr)
            self.assertTrue(os.access(dist, os.X_OK), "the dist copy must stay executable")
            self.assertEqual(dist.read_bytes(), entry.read_bytes(),
                             "dist must be a byte-for-byte copy of the installed entry")
            self.assertEqual(dist.read_bytes(), payload,
                             "dist must carry the VERIFIED bytes, not a rebuild")

            # PR-B owns the "$AI_SPECS_HOME/bin/ai-specs" launcher assertion;
            # the flip has not happened yet, so only the dist copy runs here.
            run = subprocess.run([str(dist), "version"], capture_output=True,
                                 text=True, check=False, timeout=60,
                                 env={k: v for k, v in os.environ.items()
                                      if k != "AI_SPECS_GO_BIN"})
            self.assertEqual(run.returncode, 0, run.stdout + run.stderr)
            self.assertTrue(run.stdout.strip(), "the dist binary must print a version")

    def test_a_failed_dist_copy_fails_the_install_loudly(self):
        if os.geteuid() == 0:
            self.skipTest("root ignores directory permissions; the read-only case is unenforceable")
        asset = _platform_asset()
        payload = b"#!/bin/sh\necho 1.0.0\n"
        digest = hashlib.sha256(payload).hexdigest()
        with tempfile.TemporaryDirectory(prefix="ai-specs-u2e-dist-") as tmp:
            base = Path(tmp)
            assets = base / "assets"
            assets.mkdir()
            (assets / asset).write_bytes(payload)
            server = _Server(assets)
            self.addCleanup(server.close)
            source = self._fixture_repo(base, asset, digest)
            home = base / "home"

            first = self._run_install(base, source, server, home)
            self.assertEqual(first.returncode, 0, first.stdout + first.stderr)

            # Re-run against a home whose dist/ cannot accept the copy.
            second_home = base / "home2"
            shutil.copytree(home, second_home, symlinks=True)
            (second_home / "dist" / "ai-specs").unlink()
            (second_home / "dist").chmod(0o555)

            proc = self._run_install(base, source, server, second_home,
                                     install_bin=base / "bin2")

            self.assertNotEqual(proc.returncode, 0, proc.stdout + proc.stderr)
            combined = proc.stdout + proc.stderr
            self.assertIn("dist/ai-specs", combined,
                          "the failure must name the dist path")


if __name__ == "__main__":
    unittest.main()
