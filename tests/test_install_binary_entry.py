"""GO-20 U1b-2b: install.sh installs the compiled binary as the entry.

Hermetic and offline: a fixture git repo stands in for the source, a local HTTP
server serves the platform asset, and the committed trust root is a file inside
the fixture checkout. No network, and the download path needs no Go toolchain.
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
        self.directory = directory
        handler = self._handler(directory)
        self.httpd = socketserver.TCPServer(("127.0.0.1", 0), handler)
        self.thread = threading.Thread(target=self.httpd.serve_forever, daemon=True)
        self.thread.start()

    @staticmethod
    def _handler(directory: Path):
        class Handler(http.server.SimpleHTTPRequestHandler):
            def __init__(self, *args, **kwargs):
                super().__init__(*args, directory=str(directory), **kwargs)

            def log_message(self, *args):  # keep the test output clean
                pass

        return Handler

    @property
    def base_url(self) -> str:
        return f"http://127.0.0.1:{self.httpd.server_address[1]}"

    def close(self) -> None:
        self.httpd.shutdown()
        self.httpd.server_close()


class InstallBinaryEntryTests(unittest.TestCase):
    def _fixture_repo(self, base: Path, asset: str, digest: str) -> Path:
        """A minimal cloneable repo carrying VERSION and the committed digest."""
        source = base / "source"
        source.mkdir()
        (source / "VERSION").write_text("1.0.0\n", encoding="utf-8")
        (source / "bin").mkdir()
        (source / "bin" / "ai-specs").write_text("#!/bin/sh\nexit 0\n", encoding="utf-8")
        (source / "lib").mkdir()
        # The committed trust root: the checkout supplies the expected digest.
        (source / "bin" / "SHA256SUMS").write_text(f"{digest}  {asset}\n", encoding="utf-8")
        _git(source, "init", "-b", "main")
        _git(source, "add", ".")
        _git(source, "commit", "-m", "init")
        return source

    def _run_install(self, base: Path, source: Path, server: _Server, home: Path):
        env = {
            "PATH": os.environ.get("PATH", ""),
            "HOME": str(base / "userhome"),
            "AI_SPECS_HOME": str(home),
            "INSTALL_BIN": str(base / "bin"),
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

    def test_verified_release_asset_becomes_the_entry(self):
        asset = _platform_asset()
        payload = b"#!/bin/sh\necho 1.0.0\n"
        digest = hashlib.sha256(payload).hexdigest()
        with tempfile.TemporaryDirectory(prefix="ai-specs-u1b2b-") as tmp:
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
            self.assertTrue(entry.exists(), proc.stdout + proc.stderr)
            self.assertFalse(entry.is_symlink(), "the entry must be the installed binary, not a symlink")
            self.assertEqual(entry.read_bytes(), payload, "the VERIFIED bytes must be what got installed")
            self.assertTrue(os.access(entry, os.X_OK), "the installed entry must be executable")

    def test_a_digest_mismatch_installs_nothing(self):
        asset = _platform_asset()
        payload = b"#!/bin/sh\necho tampered\n"
        wrong_digest = hashlib.sha256(b"something else").hexdigest()
        with tempfile.TemporaryDirectory(prefix="ai-specs-u1b2b-") as tmp:
            base = Path(tmp)
            assets = base / "assets"
            assets.mkdir()
            (assets / asset).write_bytes(payload)
            server = _Server(assets)
            self.addCleanup(server.close)
            # The fixture repo has no cmd/ tree, so the local-build fallback
            # cannot succeed either: the install must fail rather than place
            # unverified bytes.
            source = self._fixture_repo(base, asset, wrong_digest)
            home = base / "home"

            proc = self._run_install(base, source, server, home)

            self.assertNotEqual(proc.returncode, 0, proc.stdout)
            entry = base / "bin" / "ai-specs"
            self.assertFalse(entry.exists(), "unverified bytes must never be installed")

    def test_no_leftover_temp_files_in_the_install_bin(self):
        asset = _platform_asset()
        payload = b"#!/bin/sh\necho 1.0.0\n"
        digest = hashlib.sha256(payload).hexdigest()
        with tempfile.TemporaryDirectory(prefix="ai-specs-u1b2b-") as tmp:
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
            leftovers = [p.name for p in (base / "bin").iterdir() if p.name != "ai-specs"]
            self.assertEqual(leftovers, [], "the atomic install must not leave temp files behind")


if __name__ == "__main__":
    unittest.main()
