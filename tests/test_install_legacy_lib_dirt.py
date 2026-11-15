"""install.sh must not treat the legacy runtime leftover under lib/ as dirt.

A 0.24.x install that is fast-forwarded to 0.25.x keeps an UNTRACKED lib/
(the removed Python runtime's __pycache__ and vendored packages). `ai-specs
upgrade` and install.sh both refuse a dirty working tree, so that leftover made
the documented recovery (re-running install.sh) abort for every migrated user.

- the leftover alone is not dirt: a re-run installs instead of aborting;
- a real change to a tracked file still aborts exactly as before;
- the checkout ignores /lib/, so `git status` (and `ai-specs upgrade`) is clean.
"""
from __future__ import annotations

import hashlib
import os
import subprocess
import tempfile
import unittest
from pathlib import Path

from test_install_binary_entry import _git, _platform_asset, _Server

ROOT = Path(__file__).resolve().parents[1]
INSTALL_SH = ROOT / "install.sh"


class InstallLegacyLibDirtTests(unittest.TestCase):
    def _fixture(self, base: Path):
        asset = _platform_asset()
        payload = b"#!/bin/sh\necho 1.0.0\n"
        digest = hashlib.sha256(payload).hexdigest()
        assets = base / "assets"
        assets.mkdir()
        (assets / asset).write_bytes(payload)
        server = _Server(assets)
        self.addCleanup(server.close)
        source = base / "source"
        source.mkdir()
        (source / "VERSION").write_text("1.0.0\n", encoding="utf-8")
        (source / "README.md").write_text("readme\n", encoding="utf-8")
        # Like the real repo: install.sh copies the binary into dist/, which is ignored.
        (source / ".gitignore").write_text("dist/\n", encoding="utf-8")
        (source / "bin").mkdir()
        (source / "bin" / "ai-specs").write_text("#!/bin/sh\nexit 0\n", encoding="utf-8")
        (source / "bin" / "SHA256SUMS").write_text(f"{digest}  {asset}\n", encoding="utf-8")
        _git(source, "init", "-b", "main")
        _git(source, "add", ".")
        _git(source, "commit", "-m", "init")
        return source, server

    def _install(self, base: Path, source: Path, server: _Server, home: Path):
        env = {
            "PATH": os.environ.get("PATH", ""),
            "HOME": str(base / "userhome"),
            "AI_SPECS_HOME": str(home),
            "INSTALL_BIN": str(base / "bin"),
            "AI_SPECS_REPO": str(source),
            "AI_SPECS_REF": "main",
            "AI_SPECS_RELEASE_BASE": server.base_url,
            "AI_SPECS_TRUST_ROOT_FILE": str(home / "bin" / "SHA256SUMS"),
            "LC_ALL": "C",
            "LANG": "C",
        }
        (base / "userhome").mkdir(exist_ok=True)
        return subprocess.run(["bash", str(INSTALL_SH)], cwd=str(ROOT), env=env,
                              capture_output=True, text=True, check=False, timeout=300)

    def _installed(self, tmp: str):
        base = Path(tmp)
        source, server = self._fixture(base)
        home = base / "home"
        first = self._install(base, source, server, home)
        self.assertEqual(first.returncode, 0, first.stdout + first.stderr)
        return base, source, server, home

    def test_untracked_legacy_lib_does_not_abort_a_rerun(self):
        with tempfile.TemporaryDirectory(prefix="ai-specs-libdirt-") as tmp:
            base, source, server, home = self._installed(tmp)
            # What the removed runtime leaves behind: bytecode plus the pip-vendored
            # TUI packages (the vendored files, not the .pyc, are what git reports).
            vendored = home / "lib" / "_vendor" / "rich"
            vendored.mkdir(parents=True)
            (vendored / "__init__.py").write_text("# vendored\n", encoding="utf-8")
            cache = home / "lib" / "_internal" / "__pycache__"
            cache.mkdir(parents=True)
            (cache / "util.cpython-312.pyc").write_bytes(b"\x00pyc")

            again = self._install(base, source, server, home)

            self.assertEqual(again.returncode, 0, again.stdout + again.stderr)
            self.assertNotIn("uncommitted changes", again.stdout + again.stderr)

    def test_a_real_tracked_change_still_aborts(self):
        with tempfile.TemporaryDirectory(prefix="ai-specs-libdirt-") as tmp:
            base, source, server, home = self._installed(tmp)
            (home / "README.md").write_text("edited by the user\n", encoding="utf-8")

            again = self._install(base, source, server, home)

            self.assertNotEqual(again.returncode, 0)
            self.assertIn("uncommitted changes", again.stdout + again.stderr)
            self.assertEqual((home / "README.md").read_text(encoding="utf-8"),
                             "edited by the user\n")

    def test_checkout_ignores_the_legacy_lib_directory(self):
        lines = (ROOT / ".gitignore").read_text(encoding="utf-8").splitlines()
        self.assertIn("/lib/", [line.strip() for line in lines])


if __name__ == "__main__":
    unittest.main()
