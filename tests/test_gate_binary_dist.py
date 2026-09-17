"""Distribution tests for the worktree-gate binary (Phase 3, tasks 3.15-3.16).

Covers lib/_internal/gate_binary.py: platform detection (incl. Rosetta
mapping), version-keyed cache path construction, digest-verified install,
digest mismatch (no install, no execution, recorded for doctor), partial
download (never installed), and the degradation matrix (offline auto and go both fail open with no
Bash fallback; unsupported platform → WARN).
"""
from __future__ import annotations

import hashlib
import importlib.util
import os
import shutil
import sys
import tempfile
import unittest
import urllib.request
from pathlib import Path
from unittest import mock

ROOT = Path(__file__).resolve().parents[1]
GATE_BINARY_PY = ROOT / "lib" / "_internal" / "gate_binary.py"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


class GateBinaryDistTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.gb = load_module(GATE_BINARY_PY, "gate_binary_dist_under_test")

    def _home(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        home = Path(tmp.name)
        (home / "VERSION").write_text("9.9.9\n")
        catalog = home / "catalog" / "recipes" / "worktree-flow" / "bin"
        catalog.mkdir(parents=True)
        return home

    # --- 3.15 platform mapping + cache path -------------------------------

    def test_uname_mapping_covers_matrix_and_rosetta(self):
        cases = [
            ("Darwin", "arm64", "darwin", "arm64"),
            ("Darwin", "x86_64", "darwin", "amd64"),   # Rosetta on Apple Silicon
            ("Linux", "x86_64", "linux", "amd64"),
            ("Linux", "amd64", "linux", "amd64"),
            ("Linux", "aarch64", "linux", "arm64"),
            ("Darwin", "arm64", "darwin", "arm64"),
        ]
        for system, machine, want_os, want_arch in cases:
            with self.subTest(system=system, machine=machine):
                goos, goarch = self.gb.detect_platform(system, machine)
                self.assertEqual((goos, goarch), (want_os, want_arch))

    def test_uname_mapping_unknown_platform_is_empty(self):
        self.assertEqual(self.gb.detect_platform("Windows", "amd64"), ("", "amd64"))
        self.assertEqual(self.gb.detect_platform("Darwin", "mips"), ("darwin", ""))

    def test_cache_path_is_version_keyed(self):
        home = self._home()
        p = self.gb.cache_bin_path(home, version="1.2.3", goos="linux", goarch="amd64")
        self.assertEqual(
            p,
            home / "cache" / "bin" / "worktree-gate" / "1.2.3" / "linux-amd64" / "worktree-gate",
        )

    def test_cache_path_defaults_to_installed_version_and_host_platform(self):
        home = self._home()
        p = self.gb.cache_bin_path(home)
        self.assertEqual(p.parent.parent.name, "9.9.9")
        self.assertIn("worktree-gate", str(p))

    # --- 3.10 digest-verified install --------------------------------------

    def test_digest_match_installs_executable_binary(self):
        home = self._home()
        asset = b"\x7fELF fake binary content"
        digest = hashlib.sha256(asset).hexdigest()
        (home / "catalog" / "recipes" / "worktree-flow" / "bin" / "SHA256SUMS").write_text(
            f"{digest}  worktree-gate-darwin-arm64\n"
        )
        with mock.patch.object(self.gb, "_run_selftest", return_value=None), \
             mock.patch.object(self.gb, "binary_version", return_value="9.9.9"), \
             mock.patch.object(urllib.request, "urlretrieve", side_effect=lambda url, tmp: Path(tmp).write_bytes(asset)), \
             mock.patch.object(self.gb, "detect_platform", return_value=("darwin", "arm64")):
            status = self.gb.acquire(gate_impl="auto", ai_specs_home=home)
        self.assertTrue(status["installed"], status)
        installed = Path(status["cache_path"])
        self.assertTrue(installed.is_file())
        self.assertTrue(os.access(installed, os.X_OK))
        self.assertEqual(installed.read_bytes(), asset)

    def test_stale_executable_cache_is_revalidated_and_reacquired(self):
        home = self._home()
        stale = b"stale cache bytes"
        fresh = b"fresh verified bytes"
        digest = hashlib.sha256(fresh).hexdigest()
        (home / "catalog" / "recipes" / "worktree-flow" / "bin" / "SHA256SUMS").write_text(
            f"{digest}  worktree-gate-darwin-arm64\n"
        )
        cache = self.gb.cache_bin_path(home, version="9.9.9", goos="darwin", goarch="arm64")
        cache.parent.mkdir(parents=True)
        cache.write_bytes(stale)
        cache.chmod(0o755)

        with mock.patch.object(self.gb, "detect_platform", return_value=("darwin", "arm64")), \
             mock.patch.object(self.gb, "binary_version", return_value="9.9.9"), \
             mock.patch.object(self.gb, "_run_selftest", return_value=None), \
             mock.patch.object(
                 urllib.request,
                 "urlretrieve",
                 side_effect=lambda url, tmp: Path(tmp).write_bytes(fresh),
             ) as download:
            status = self.gb.acquire(gate_impl="auto", ai_specs_home=home)

        self.assertTrue(status["installed"], status)
        self.assertEqual(cache.read_bytes(), fresh)
        download.assert_called_once()
        self.assertTrue(
            self.gb.verification_record_path(cache).is_file(),
            "a successful cache refresh must leave current verification evidence",
        )
        self.assertIn("re-acquired", status["verification"]["reason"])

    def test_cache_version_mismatch_is_not_executed_before_reacquisition(self):
        home = self._home()
        fresh = b"fresh after version mismatch"
        digest = hashlib.sha256(fresh).hexdigest()
        (home / "catalog" / "recipes" / "worktree-flow" / "bin" / "SHA256SUMS").write_text(
            f"{digest}  worktree-gate-darwin-arm64\n"
        )
        cache = self.gb.cache_bin_path(home, version="9.9.9", goos="darwin", goarch="arm64")
        cache.parent.mkdir(parents=True)
        cache.write_bytes(fresh)
        cache.chmod(0o755)

        versions = iter(("8.8.8", "9.9.9"))
        with mock.patch.object(self.gb, "detect_platform", return_value=("darwin", "arm64")), \
             mock.patch.object(self.gb, "binary_version", side_effect=lambda path: next(versions)), \
             mock.patch.object(self.gb, "_run_selftest", return_value=None), \
             mock.patch.object(
                 urllib.request,
                 "urlretrieve",
                 side_effect=lambda url, tmp: Path(tmp).write_bytes(fresh),
             ):
            status = self.gb.acquire(gate_impl="auto", ai_specs_home=home)

        self.assertTrue(status["installed"], status)
        self.assertEqual(cache.read_bytes(), fresh)
        self.assertIn("version", status["verification"]["reason"])

    def test_cache_selftest_failure_forces_reacquisition(self):
        home = self._home()
        fresh = b"fresh after selftest failure"
        digest = hashlib.sha256(fresh).hexdigest()
        (home / "catalog" / "recipes" / "worktree-flow" / "bin" / "SHA256SUMS").write_text(
            f"{digest}  worktree-gate-darwin-arm64\n"
        )
        cache = self.gb.cache_bin_path(home, version="9.9.9", goos="darwin", goarch="arm64")
        cache.parent.mkdir(parents=True)
        cache.write_bytes(fresh)
        cache.chmod(0o755)

        selftests = iter(("selftest failed", None, None))
        with mock.patch.object(self.gb, "detect_platform", return_value=("darwin", "arm64")), \
             mock.patch.object(self.gb, "binary_version", return_value="9.9.9"), \
             mock.patch.object(self.gb, "_run_selftest", side_effect=lambda path: next(selftests)) as selftest, \
             mock.patch.object(
                 urllib.request,
                 "urlretrieve",
                 side_effect=lambda url, tmp: Path(tmp).write_bytes(fresh),
             ) as download:
            status = self.gb.acquire(gate_impl="auto", ai_specs_home=home)

        self.assertTrue(status["installed"], status)
        self.assertEqual(cache.read_bytes(), fresh)
        self.assertEqual(selftest.call_count, 2)
        download.assert_called_once()

    def test_verification_receipt_failure_leaves_cache_unselected(self):
        home = self._home()
        asset = b"asset with receipt failure"
        digest = hashlib.sha256(asset).hexdigest()
        (home / "catalog" / "recipes" / "worktree-flow" / "bin" / "SHA256SUMS").write_text(
            f"{digest}  worktree-gate-darwin-arm64\n"
        )
        with mock.patch.object(self.gb, "detect_platform", return_value=("darwin", "arm64")), \
             mock.patch.object(self.gb, "binary_version", return_value="9.9.9"), \
             mock.patch.object(self.gb, "_run_selftest", return_value=None), \
             mock.patch.object(
                 urllib.request,
                 "urlretrieve",
                 side_effect=lambda url, tmp: Path(tmp).write_bytes(asset),
             ), \
             mock.patch.object(self.gb, "_write_verification_record", side_effect=OSError("receipt denied")):
            status = self.gb.acquire(gate_impl="auto", ai_specs_home=home)

        self.assertFalse(status["installed"], status)
        cache = Path(status["cache_path"])
        self.assertFalse(cache.exists())
        self.assertFalse(self.gb.verification_record_path(cache).exists())
        self.assertIn("receipt write failed", status["warn"])

    def test_digest_mismatch_never_installs_and_records(self):
        home = self._home()
        (home / "catalog" / "recipes" / "worktree-flow" / "bin" / "SHA256SUMS").write_text(
            "0" * 64 + "  worktree-gate-darwin-arm64\n"
        )
        fake = home / "fake.bin"
        fake.write_bytes(b"corrupted bytes")
        with mock.patch.object(urllib.request, "urlretrieve", side_effect=lambda url, tmp: shutil.copy(fake, tmp)), \
             mock.patch.object(self.gb, "detect_platform", return_value=("darwin", "arm64")):
            status = self.gb.acquire(gate_impl="auto", ai_specs_home=home)
        self.assertFalse(status["installed"])
        self.assertIsNotNone(status.get("mismatch"))
        self.assertFalse(Path(status["cache_path"]).exists())
        self.assertIn("digest mismatch", status["warn"])
        # Recorded for doctor.
        self.assertTrue(self.gb.digest_mismatch_record_path(home).is_file())

    def test_partial_download_never_installed(self):
        home = self._home()
        (home / "catalog" / "recipes" / "worktree-flow" / "bin" / "SHA256SUMS").write_text(
            "0" * 64 + "  worktree-gate-darwin-arm64\n"
        )
        def broken_download(url, tmp):
            Path(tmp).write_bytes(b"half")
            raise OSError("connection reset")
        with mock.patch.object(urllib.request, "urlretrieve", side_effect=broken_download), \
             mock.patch.object(self.gb, "detect_platform", return_value=("darwin", "arm64")):
            status = self.gb.acquire(gate_impl="auto", ai_specs_home=home)
        self.assertFalse(status["installed"])
        self.assertFalse(Path(status["cache_path"]).exists())
        self.assertIn("download failed", status["warn"])

    # --- 3.12 local build --------------------------------------------------

    def test_opt_in_local_build_populates_cache(self):
        home = self._home()
        # Point the in-repo source at the real gate module.
        src = ROOT / "catalog" / "recipes" / "worktree-flow" / "gate"
        (home / "catalog" / "recipes" / "worktree-flow").mkdir(parents=True, exist_ok=True)
        if not (home / "catalog" / "recipes" / "worktree-flow" / "gate").exists():
            shutil.copytree(src, home / "catalog" / "recipes" / "worktree-flow" / "gate")
        with mock.patch.dict(os.environ, {"AI_SPECS_GATE_BUILD": "1"}), \
             mock.patch.object(self.gb, "detect_platform", return_value=("darwin", "arm64")):
            status = self.gb.acquire(gate_impl="auto", ai_specs_home=home)
        self.assertTrue(status["installed"], status)
        installed = Path(status["cache_path"])
        self.assertTrue(installed.is_file())
        self.assertTrue(os.access(installed, os.X_OK))

    # --- 3.16 degradation matrix -------------------------------------------

    def test_offline_auto_and_go_degrade_without_bash_fallback(self):
        home = self._home()
        for impl in ("auto", "go"):
            with self.subTest(impl=impl):
                with mock.patch.object(self.gb, "detect_platform", return_value=("darwin", "arm64")), \
                     mock.patch("shutil.which", return_value=None):
                    status = self.gb.acquire(gate_impl=impl, ai_specs_home=home, offline=True)
                self.assertTrue(status["attempted"], status)
                self.assertFalse(status["installed"])
                warn = status["warn"] or ""
                self.assertIn("failing open", warn)
                self.assertNotIn("Bash", warn)
                blob = str(status).lower()
                self.assertNotIn("bash fallback", blob)
                self.assertNotIn("falling back to the bash", blob)

    def test_acquire_runs_for_auto_and_go_no_bash_early_return(self):
        home = self._home()
        for impl in ("auto", "go"):
            with self.subTest(impl=impl):
                with mock.patch.object(self.gb, "detect_platform", return_value=("darwin", "arm64")), \
                     mock.patch("shutil.which", return_value=None):
                    status = self.gb.acquire(gate_impl=impl, ai_specs_home=home, offline=True)
                self.assertTrue(status["attempted"])
                self.assertFalse(status["installed"])

    def test_unsupported_platform_warns_no_install(self):
        home = self._home()
        with mock.patch.object(self.gb, "detect_platform", return_value=("windows", "amd64")):
            status = self.gb.acquire(gate_impl="auto", ai_specs_home=home)
        self.assertFalse(status["installed"])
        self.assertIn("unsupported platform", status["warn"])
        self.assertIn("darwin/arm64", status["warn"])

    def test_acquisition_never_raises(self):
        home = self._home()
        with mock.patch.object(urllib.request, "urlretrieve",
                               side_effect=Exception("boom")):
            status = self.gb.acquire(gate_impl="auto", ai_specs_home=home)
        self.assertIsInstance(status, dict)
        self.assertIn("warn", status)

    # --- release blocker: canonical asset URL owner -------------------------

    def test_asset_url_uses_canonical_repository_owner(self):
        # Release blocker regression: the download URL must target the
        # canonical repository owner used by the remote, install.sh, and
        # the release workflow (parada1104). The previous hardcoded
        # `nnodes` owner would 404 every `ai-specs sync` binary
        # acquisition.
        url = self.gb._asset_url("9.9.9", "worktree-gate-darwin-arm64")
        self.assertEqual(
            url,
            "https://github.com/parada1104/ai-specs-cli/releases/download/"
            "v9.9.9/worktree-gate-darwin-arm64",
        )

    def test_gate_binary_module_has_no_divergent_repository_owner(self):
        # Guard against reintroducing the divergent `nnodes` owner
        # anywhere else in the module (not only in _asset_url).
        source = GATE_BINARY_PY.read_text(encoding="utf-8")
        self.assertNotIn(
            "nnodes", source,
            "gate_binary.py must not reference the divergent 'nnodes' "
            "repository owner",
        )


if __name__ == "__main__":
    unittest.main()
