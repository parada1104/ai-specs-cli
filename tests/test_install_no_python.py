"""GO-20 U2e T5: install.sh requires and provisions no Python.

The installer used to refuse to run without an ambient `python3` and to
`pip install` rich + questionary into `$AI_SPECS_HOME/lib/_vendor` for the
still-Python wizard. Both are gone: the entry point is the compiled binary and
`lib/_vendor` is committed, so the installer needs git and the Go toolchain
only.

Two kinds of assertion live here, matching the two ways the removal is
observable:

- static guards over install.sh: no `python3` reference, no pip/vendor step,
  no `AI_SPECS_SKIP_TUI_DEPS` escape hatch, and the surviving step numbering
  stays consistent;
- one functional install with a `python3` shim FIRST on PATH that records every
  invocation and fails loudly. The install must succeed without ever calling it,
  which is what makes the removal a behaviour and not only a text edit.

The surviving installer behaviours (verified release asset, digest mismatch,
dist copy, narrow checkout, dirty-tree gate) stay pinned by the existing
`tests/test_install_*.py` modules; nothing here relaxes them.
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


class InstallNoPythonStaticTests(unittest.TestCase):
    """Static contract: the Python requirement and the vendor step are gone."""

    @classmethod
    def setUpClass(cls):
        cls.text = INSTALL_SH.read_text(encoding="utf-8")

    def test_installer_never_references_python3(self):
        self.assertNotIn(
            "python3",
            self.text,
            "install.sh must not mention python3: the entry point is the "
            "compiled binary and no install step may depend on an interpreter.",
        )

    def test_installer_does_not_provision_vendored_python_packages(self):
        for needle, why in (
            ("pip install", "the rich/questionary pip install step belongs to the retired wizard"),
            ("_vendor", "lib/_vendor is committed; the installer no longer writes it"),
            ("questionary", "questionary belongs to the retired Python wizard"),
        ):
            self.assertNotIn(needle, self.text, f"install.sh must not contain {needle!r}: {why}")

    def test_installer_drops_the_tui_deps_escape_hatch(self):
        self.assertNotIn(
            "AI_SPECS_SKIP_TUI_DEPS",
            self.text,
            "the escape hatch existed only to keep the pip step out of the tests",
        )

    def test_installer_step_numbering_stays_consistent(self):
        self.assertIn("[1/2]", self.text)
        self.assertIn("[2/2]", self.text)
        for stale in ("[1/3]", "[2/3]", "[3/3]"):
            self.assertNotIn(stale, self.text, f"{stale} counts a step that no longer exists")

    def test_installer_still_requires_git(self):
        self.assertIn(
            "error: git is required",
            self.text,
            "the git requirement is a surviving behaviour, not part of the Python removal",
        )


class InstallNoPythonFunctionalTests(unittest.TestCase):
    """A python3 shim first on PATH catches any surviving invocation."""

    def _python_shim(self, base: Path) -> tuple[Path, Path]:
        shim_dir = base / "shim"
        shim_dir.mkdir()
        marker = base / "python3-calls"
        shim = shim_dir / "python3"
        shim.write_text(
            "#!/bin/sh\n"
            f'printf "%s\\n" "$*" >> "{marker}"\n'
            'echo "python3 must not be invoked by the installer" >&2\n'
            "exit 97\n",
            encoding="utf-8",
        )
        shim.chmod(0o755)
        return shim_dir, marker

    def _fixture_repo(self, base: Path, asset: str, digest: str) -> Path:
        source = base / "source"
        source.mkdir()
        (source / "VERSION").write_text("1.0.0\n", encoding="utf-8")
        (source / "bin").mkdir()
        (source / "bin" / "ai-specs").write_text("#!/bin/sh\nexit 0\n", encoding="utf-8")
        (source / "bin" / "SHA256SUMS").write_text(f"{digest}  {asset}\n", encoding="utf-8")
        _git(source, "init", "-b", "main")
        _git(source, "add", ".")
        _git(source, "commit", "-m", "init")
        return source

    def test_install_succeeds_without_calling_python3(self):
        asset = _platform_asset()
        payload = b"#!/bin/sh\necho 1.0.0\n"
        digest = hashlib.sha256(payload).hexdigest()
        with tempfile.TemporaryDirectory(prefix="ai-specs-nopy-") as tmp:
            base = Path(tmp)
            assets = base / "assets"
            assets.mkdir()
            (assets / asset).write_bytes(payload)
            server = _Server(assets)
            self.addCleanup(server.close)
            source = self._fixture_repo(base, asset, digest)
            home = base / "home"
            (base / "userhome").mkdir()
            shim_dir, marker = self._python_shim(base)

            env = {
                # The shim comes FIRST: an ambient interpreter cannot mask the call.
                "PATH": f"{shim_dir}{os.pathsep}{os.environ.get('PATH', '')}",
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
            proc = subprocess.run(
                ["bash", str(INSTALL_SH)],
                cwd=str(ROOT),
                env=env,
                capture_output=True,
                text=True,
                check=False,
                timeout=300,
            )

            calls = marker.read_text(encoding="utf-8") if marker.exists() else ""
            self.assertEqual(
                calls,
                "",
                f"the installer invoked python3: {calls!r}\nstdout:\n{proc.stdout}",
            )
            self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
            entry = base / "bin" / "ai-specs"
            self.assertTrue(entry.exists(), proc.stdout + proc.stderr)
            self.assertEqual(entry.read_bytes(), payload)
            self.assertNotIn("python3", proc.stdout + proc.stderr)
            self.assertNotIn("TUI deps", proc.stdout)


if __name__ == "__main__":
    unittest.main()
