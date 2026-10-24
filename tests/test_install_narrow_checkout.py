"""GO-20 U2e U-E: the installer narrows its own checkout, with no lib/_internal dependency.

install.sh used to run lib/_internal/narrow-checkout.sh. That helper is deleted at T5,
so the installer carries the narrowing inline. This suite drives the installer on a
hermetic fixture repo; the release asset is served with a file:// base (curl reads it).
It asserts only observable results: the installed home tree and the installer output.
The fixture deliberately ships NO lib/_internal/narrow-checkout.sh.
"""
from __future__ import annotations

import hashlib
import os
import platform
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
INSTALL_SH = ROOT / "install.sh"

RUNTIME_DIRS = ("lib", "bin", "catalog", "bundled-skills", "bundled-commands",
                "templates", "scripts", "docs")
EXCLUDED_DIRS = ("openspec", "tests", ".github", "tmp")
NARROW_LINE = "narrowed checkout (openspec/, tests/, .github/, tmp/ excluded)"


def _platform_asset() -> str:
    os_name = {"Darwin": "darwin", "Linux": "linux"}.get(platform.system())
    arch = {"arm64": "arm64", "aarch64": "arm64", "x86_64": "amd64", "amd64": "amd64"}.get(
        platform.machine().lower())
    if os_name is None or arch is None:
        raise unittest.SkipTest(f"unsupported platform {platform.system()}/{platform.machine()}")
    return f"ai-specs-{os_name}-{arch}"


def _git(repo: Path, *args: str) -> None:
    env = {
        **os.environ,
        "GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@example.com",
        "GIT_COMMITTER_NAME": "t", "GIT_COMMITTER_EMAIL": "t@example.com",
    }
    subprocess.run(["git", *args], cwd=str(repo), env=env, check=True,
                   capture_output=True, text=True)


class InstallNarrowCheckoutTests(unittest.TestCase):
    def _fixture_repo(self, base: Path, asset: str, digest: str) -> Path:
        source = base / "source"
        source.mkdir()
        (source / "VERSION").write_text("1.0.0\n", encoding="utf-8")
        for name in RUNTIME_DIRS + EXCLUDED_DIRS:
            (source / name).mkdir()
            (source / name / "file.txt").write_text(f"content of {name}\n", encoding="utf-8")
        (source / "bin" / "ai-specs").write_text("#!/bin/sh\nexit 0\n", encoding="utf-8")
        (source / "bin" / "SHA256SUMS").write_text(f"{digest}  {asset}\n", encoding="utf-8")
        _git(source, "init", "-b", "main")
        _git(source, "add", ".")
        _git(source, "commit", "-m", "init")
        return source

    def _run_install(self, base: Path, source: Path, assets: Path, home: Path):
        env = {
            "PATH": os.environ.get("PATH", ""),
            "HOME": str(base / "userhome"),
            "AI_SPECS_HOME": str(home),
            "INSTALL_BIN": str(base / "bin"),
            "AI_SPECS_REPO": str(source),
            "AI_SPECS_REF": "main",
            "AI_SPECS_RELEASE_BASE": assets.as_uri(),
            "AI_SPECS_TRUST_ROOT_FILE": str(home / "bin" / "SHA256SUMS"),
            "AI_SPECS_SKIP_TUI_DEPS": "1",
            "LC_ALL": "C",
            "LANG": "C",
        }
        (base / "userhome").mkdir(exist_ok=True)
        return subprocess.run(["bash", str(INSTALL_SH)], cwd=str(ROOT), env=env,
                              capture_output=True, text=True, check=False, timeout=300)

    def test_installer_narrows_its_checkout_without_the_lib_helper(self):
        asset = _platform_asset()
        payload = b"#!/bin/sh\necho 'ai-specs test-version 1.0.0'\n"
        digest = hashlib.sha256(payload).hexdigest()
        with tempfile.TemporaryDirectory(prefix="ai-specs-u2e-ue-") as tmp:
            base = Path(tmp)
            assets = base / "assets"
            assets.mkdir()
            (assets / asset).write_bytes(payload)
            source = self._fixture_repo(base, asset, digest)
            home = base / "home"

            proc = self._run_install(base, source, assets, home)

            combined = proc.stdout + proc.stderr
            self.assertEqual(proc.returncode, 0, combined)
            self.assertIn(NARROW_LINE, proc.stdout, combined)
            for name in EXCLUDED_DIRS:
                self.assertFalse((home / name).exists(),
                                 f"{name}/ must be left out of the narrowed install")
            for name in RUNTIME_DIRS:
                self.assertTrue((home / name).is_dir(),
                                f"{name}/ is a runtime directory and must survive")

    def test_installer_does_not_depend_on_lib_internal(self):
        text = INSTALL_SH.read_text(encoding="utf-8")
        self.assertNotIn("lib/_internal/narrow-checkout.sh", text,
                         "install.sh must not call the lib/_internal helper (deleted at T5)")


if __name__ == "__main__":
    unittest.main()
