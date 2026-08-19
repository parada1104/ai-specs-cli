"""Black-box distribution tests for the worktree-gate binary.

These tests drive the shipped CLI (`bin/ai-specs sync`) as the process
boundary — no `lib/_internal/` import, no direct `lib/*.sh` invocation.
Gate acquisition happens inside `sync` when the worktree-flow recipe is
enabled and `gate_impl` is `auto`/`go`; the resulting binary, verification
receipt, quarantine, and degradation warnings are observed on disk and in
stderr.

Platform note: the gate release matrix is fixed (darwin/arm64, darwin/amd64,
linux/amd64, linux/arm64) and this host is darwin/arm64. The uname-to-goos
mapping matrix and unknown-platform behavior are triaged (no observable
equivalent through the shipped CLI on a fixed host).
"""
from __future__ import annotations

import hashlib
import os
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

from _blackbox import CLI, ROOT, cache_project_dir, isolated_home


GATE_BINARY_PY = ROOT / "lib" / "_internal" / "gate_binary.py"
SHA256SUMS = ROOT / "catalog" / "recipes" / "worktree-flow" / "bin" / "SHA256SUMS"

# Installed CLI version (version key for the cache layout). isolated_home
# symlinks ROOT/VERSION into the fake install root, so cli_version(home)
# == this.
VERSION = (ROOT / "VERSION").read_text(encoding="utf-8").strip()

# Fixed host platform (see module docstring / triage note).
PLATFORM = "darwin-arm64"

# The version-keyed cache segment, as built by lib/_internal/gate_binary.py
# (cache/bin/worktree-gate/<version>/<goos>-<goarch>/worktree-gate).
_CACHE_REL = Path("cache") / "bin" / "worktree-gate"

# PATH that has the working Homebrew python3 (tomllib) and system tools but
# deliberately omits the mise shims dir where the only `go` shim lives.
PATH_WITHOUT_GO = "/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin"


def _committed_digest() -> str | None:
    """The committed darwin-arm64 digest from SHA256SUMS (text read is allowed)."""
    for line in SHA256SUMS.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if line.startswith("#") or not line:
            continue
        parts = line.split(None, 1)
        if len(parts) == 2 and parts[1] == "worktree-gate-darwin-arm64":
            return parts[0].lower()
    return None


class GateBinaryDistTests(unittest.TestCase):
    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory(prefix="gate-dist-")
        self.addCleanup(self._tmp.cleanup)
        # isolated_home builds home = <base>/cli-home over a real cache dir.
        self.home = isolated_home(Path(self._tmp.name))

    # --- fixtures ---------------------------------------------------------

    def _project(self, gate_impl: str = "auto") -> Path:
        root = Path(self._tmp.name) / "proj"
        (root / "ai-specs" / "skills").mkdir(parents=True)
        (root / "ai-specs" / "commands").mkdir()
        manifest = (
            "[project]\nname = 'gate-dist-fixture'\nsubrepos = []\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            "[recipes.worktree-flow]\nenabled = true\n\n"
            "[recipes.worktree-flow.config]\n"
            f"gate_impl = {gate_impl!r}\ngate_mode = 'always'\n"
        )
        (root / "ai-specs" / "ai-specs.toml").write_text(manifest)
        return root

    def _cache_root(self) -> Path:
        """Observed farm cache root (<home>/cache).

        cache_project_dir(root, home) == <home>/cache/projects/<key>, so the
        project-cache dir's grandparent is the farm cache root; the gate
        binary cache (<home>/cache/bin/worktree-gate/...) is a sibling of the
        project-cache dir under it. The derivation is cross-checked in
        test_gate_binary_lands_in_farm_cache_not_project.
        """
        return self.home / "cache"

    def _cache_bin(self) -> Path:
        return self._cache_root() / "bin" / "worktree-gate" / VERSION / PLATFORM / "worktree-gate"

    def _mismatch_record(self) -> Path:
        return self._cache_root() / "bin" / "worktree-gate" / VERSION / "last-digest-mismatch.txt"

    def _seed_stale(self, content: bytes, *, receipt: bool = True) -> Path:
        cb = self._cache_bin()
        cb.parent.mkdir(parents=True, exist_ok=True)
        cb.write_bytes(content)
        cb.chmod(0o755)
        if receipt:
            (cb.with_name(cb.name + ".verified")).write_text(
                "status=verified\n"
                f"version={VERSION}\n"
                "digest=deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef\n"
                "selftest=passed\n"
            )
        return cb

    def _tamper_catalog(self) -> None:
        """Replace the symlinked catalog with a REAL COPY minus the Go gate module."""
        cat = self.home / "catalog"
        cat.unlink()  # drop the symlink; never mutate the shared ROOT catalog
        shutil.copytree(ROOT / "catalog", cat)
        gate = cat / "recipes" / "worktree-flow" / "gate"
        if gate.exists():
            shutil.rmtree(gate)
        self.assertFalse(gate.exists())

    def _sync_env(self, *, path: str | None = None, **extra: str) -> dict:
        env = dict(os.environ)
        if path is not None:
            env["PATH"] = path
        env["AI_SPECS_HOME"] = str(self.home)
        env["AI_SPECS_NO_NETWORK"] = "1"
        env.update(extra)
        return env

    def _run_sync(self, root: Path, *, path: str | None = None, **extra: str) -> subprocess.CompletedProcess:
        return subprocess.run(
            [str(CLI), "sync"],
            cwd=str(root),
            capture_output=True,
            text=True,
            env=self._sync_env(path=path, **extra),
        )

    # --- 3.10-3.13 acquisition + verification (local off-domain build) ----

    def test_offline_build_populates_version_keyed_cache(self):
        root = self._project()
        p = self._run_sync(root, AI_SPECS_GATE_OFFLINE="1", AI_SPECS_GATE_BUILD="1")
        self.assertEqual(p.returncode, 0, p.stderr)
        cb = self._cache_bin()
        self.assertTrue(cb.is_file(), f"no gate binary at {cb}")
        self.assertTrue(os.access(cb, os.X_OK))
        # Version-keyed + host-platform layout under the farm cache.
        self.assertEqual(cb.parent.name, "darwin-arm64")
        self.assertEqual(cb.parent.parent.name, VERSION)
        self.assertEqual(
            cb.parent.parent.parent,
            self.home / "cache" / "bin" / "worktree-gate",
        )

    def test_built_digest_matches_committed_shasums(self):
        root = self._project()
        p = self._run_sync(root, AI_SPECS_GATE_OFFLINE="1", AI_SPECS_GATE_BUILD="1")
        self.assertEqual(p.returncode, 0, p.stderr)
        committed = _committed_digest()
        self.assertIsNotNone(committed, "must find a committed darwin-arm64 digest")
        digest = hashlib.sha256(self._cache_bin().read_bytes()).hexdigest()
        self.assertEqual(digest, committed)

    def test_verification_receipt_written_after_acquire(self):
        root = self._project()
        p = self._run_sync(root, AI_SPECS_GATE_OFFLINE="1", AI_SPECS_GATE_BUILD="1")
        self.assertEqual(p.returncode, 0, p.stderr)
        receipt = self._cache_bin().with_name(self._cache_bin().name + ".verified")
        self.assertTrue(receipt.is_file(), "acquisition must write a .verified receipt")
        text = receipt.read_text(encoding="utf-8")
        self.assertIn("status=verified", text)
        self.assertIn(f"version={VERSION}", text)
        self.assertIn(f"digest={_committed_digest()}", text)
        self.assertIn("selftest=passed", text)

    def test_built_binary_version_and_selftest_ok(self):
        root = self._project()
        p = self._run_sync(root, AI_SPECS_GATE_OFFLINE="1", AI_SPECS_GATE_BUILD="1")
        self.assertEqual(p.returncode, 0, p.stderr)
        cb = self._cache_bin()
        ver = subprocess.run([str(cb), "--version"], capture_output=True, text=True)
        self.assertEqual(ver.returncode, 0)
        self.assertEqual(ver.stdout.strip(), VERSION)
        st = subprocess.run([str(cb), "--selftest"], capture_output=True, text=True)
        self.assertEqual(st.returncode, 0, st.stderr)

    def test_second_sync_reuses_verified_binary(self):
        root = self._project()
        env = dict(AI_SPECS_GATE_OFFLINE="1", AI_SPECS_GATE_BUILD="1")
        p1 = self._run_sync(root, **env)
        self.assertEqual(p1.returncode, 0, p1.stderr)
        first = hashlib.sha256(self._cache_bin().read_bytes()).hexdigest()
        p2 = self._run_sync(root, **env)
        self.assertEqual(p2.returncode, 0, p2.stderr)
        self.assertTrue(self._cache_bin().is_file())
        self.assertEqual(
            hashlib.sha256(self._cache_bin().read_bytes()).hexdigest(),
            first,
            "a verified cached binary must be revalidated and reused, not rebuilt",
        )

    def test_stale_seed_is_quarantined_and_reacquired(self):
        stale = b"#!/bin/sh\necho stale\n"
        cb = self._seed_stale(stale)
        stale_digest = hashlib.sha256(stale).hexdigest()
        root = self._project()
        p = self._run_sync(root, AI_SPECS_GATE_OFFLINE="1", AI_SPECS_GATE_BUILD="1")
        self.assertEqual(p.returncode, 0, p.stderr)
        # Rejected candidate moved to the rejected/ quarantine with its receipt.
        rejected = cb.parent / "rejected" / f"worktree-gate-{stale_digest}"
        self.assertTrue(rejected.is_file(), f"expected quarantine at {rejected}")
        self.assertEqual(hashlib.sha256(rejected.read_bytes()).hexdigest(), stale_digest)
        self.assertTrue((rejected.with_name(rejected.name + ".verified")).is_file())
        # A fresh verified binary replaces the cache path.
        self.assertTrue(cb.is_file())
        self.assertEqual(hashlib.sha256(cb.read_bytes()).hexdigest(), _committed_digest())
        self.assertTrue(cb.with_name(cb.name + ".verified").is_file())
        # Successful reacquisition clears the recorded mismatch for doctor.
        self.assertFalse(self._mismatch_record().exists())

    def test_stale_candidate_never_executed(self):
        marker = Path(self._tmp.name) / "MARKER_EXECUTED"
        seed = f"#!/bin/sh\ntouch {marker}\n"
        cb = self._seed_stale(seed.encode())
        root = self._project()
        p = self._run_sync(root, AI_SPECS_GATE_OFFLINE="1", AI_SPECS_GATE_BUILD="1")
        self.assertEqual(p.returncode, 0, p.stderr)
        # A digest-mismatched cache candidate must be quarantined and never run.
        self.assertFalse(marker.exists(), "stale gate candidate must never be executed")
        self.assertTrue(cb.is_file(), "a fresh binary must replace the rejected candidate")

    # --- 3.16 degradation matrix ------------------------------------------

    def test_offline_no_cached_binary_falls_back_bash(self):
        root = self._project()
        # go is absent from PATH_WITHOUT_GO, so offline cannot build; with no
        # cached binary the gate degrades to the legacy Bash implementation.
        p = self._run_sync(root, path=PATH_WITHOUT_GO, AI_SPECS_GATE_OFFLINE="1")
        self.assertEqual(p.returncode, 0, p.stderr)
        self.assertIn("worktree-gate: offline with no cached binary;", p.stderr)
        self.assertIn("gate_impl=auto: falling back to the Bash implementation", p.stderr)
        self.assertFalse(self._cache_bin().exists())

    def test_offline_with_go_builds_when_catalog_intact(self):
        # Default offline matrix cell: `auto` + go present + intact catalog
        # triggers a local build (no AI_SPECS_GATE_BUILD needed), reproducing
        # the committed digest. Distinct from the no-go fallback above.
        root = self._project()
        p = self._run_sync(root, AI_SPECS_GATE_OFFLINE="1")
        self.assertEqual(p.returncode, 0, p.stderr)
        self.assertNotIn("worktree-gate:", p.stderr)
        self.assertTrue(self._cache_bin().is_file())
        self.assertEqual(
            hashlib.sha256(self._cache_bin().read_bytes()).hexdigest(),
            _committed_digest(),
        )

    def test_tampered_catalog_offline_auto_falls_back_bash(self):
        root = self._project(gate_impl="auto")
        self._tamper_catalog()
        p = self._run_sync(root, AI_SPECS_GATE_OFFLINE="1")
        self.assertEqual(p.returncode, 0, "sync never fails due to gate acquisition")
        self.assertIn("worktree-gate: local build failed (", p.stderr)
        self.assertIn("gate_impl=auto: falling back to the Bash implementation", p.stderr)
        self.assertFalse(self._cache_bin().exists())

    def test_tampered_catalog_offline_go_fails_open(self):
        root = self._project(gate_impl="go")
        self._tamper_catalog()
        p = self._run_sync(root, AI_SPECS_GATE_OFFLINE="1")
        self.assertEqual(p.returncode, 0, "sync never fails due to gate acquisition")
        self.assertIn("worktree-gate: local build failed (", p.stderr)
        self.assertIn("gate_impl=go: gate is failing open (run 'ai-specs doctor')", p.stderr)
        self.assertFalse(self._cache_bin().exists())

    def test_gate_impl_bash_no_acquisition(self):
        root = self._project(gate_impl="bash")
        p = self._run_sync(root, AI_SPECS_GATE_OFFLINE="1")
        self.assertEqual(p.returncode, 0, p.stderr)
        self.assertFalse(self._cache_bin().exists(),
                         "gate_impl=bash must never acquire a binary")
        self.assertNotIn("worktree-gate:", p.stderr,
                         "gate_impl=bash must not warn about the Go gate")

    # --- release blocker: source-of-truth guards --------------------------

    def test_no_divergent_repository_owner(self):
        # Source-TEXT guard (reading file text is allowed, not a coupled ref):
        # gate_binary.py must not reference the divergent 'nnodes' owner.
        source = GATE_BINARY_PY.read_text(encoding="utf-8")
        self.assertNotIn("nnodes", source)

    def test_asset_url_uses_canonical_repository_owner(self):
        # Release blocker regression (source-TEXT guard): the download URL
        # must target the canonical owner used everywhere else (parada1104).
        # The previous hardcoded `nnodes` owner would 404 every acquisition.
        source = GATE_BINARY_PY.read_text(encoding="utf-8")
        self.assertIn("parada1104", source)
        self.assertNotIn("nnodes", source)

    # --- hermeticity / placement ------------------------------------------

    def test_gate_binary_lands_in_farm_cache_not_project(self):
        root = self._project()
        p = self._run_sync(root, AI_SPECS_GATE_OFFLINE="1", AI_SPECS_GATE_BUILD="1")
        self.assertEqual(p.returncode, 0, p.stderr)
        self.assertEqual(
            cache_project_dir(root, self.home).parent.parent,
            self.home / "cache",
            "project-cache dir must live under the observed farm cache root",
        )
        self.assertTrue((self.home / "cache" / "bin" / "worktree-gate").is_dir())
        self.assertFalse((root / "cache").exists(),
                         "gate acquisition must never write into the project")
        self.assertFalse((root / "worktree-gate").exists())

    def test_offline_build_is_hermetic_and_degrades_cleanly(self):
        root = self._project()
        p = self._run_sync(root, AI_SPECS_GATE_OFFLINE="1", AI_SPECS_GATE_BUILD="1")
        self.assertEqual(p.returncode, 0, p.stderr)
        self.assertIn("sync complete", p.stdout)
        self.assertNotIn("worktree-gate: local build failed", p.stderr)
        self.assertNotIn("offline with no cached binary", p.stderr)
        self.assertTrue(self._cache_bin().is_file())

    def test_no_worktree_recipe_skips_gate_acquisition(self):
        # A project that does not enable the worktree-flow recipe must not
        # trigger any gate acquisition (no binary, no gate warning).
        root = Path(self._tmp.name) / "norecipe"
        (root / "ai-specs" / "skills").mkdir(parents=True)
        (root / "ai-specs" / "commands").mkdir()
        (root / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'no-recipe'\nsubrepos = []\n\n"
            "[agents]\nenabled = ['claude']\n"
        )
        p = self._run_sync(root, AI_SPECS_GATE_OFFLINE="1", AI_SPECS_GATE_BUILD="1")
        self.assertEqual(p.returncode, 0, p.stderr)
        self.assertFalse(self._cache_bin().exists())
        self.assertNotIn("worktree-gate:", p.stderr)

    def test_stale_candidate_without_receipt_is_quarantined(self):
        # Digest-mismatch detection precedes any receipt check: a wrong-digest
        # executable with NO .verified receipt is still quarantined and
        # reacquired (never executed, never trusted).
        stale = b"#!/bin/sh\necho stale-no-receipt\n"
        cb = self._seed_stale(stale, receipt=False)
        stale_digest = hashlib.sha256(stale).hexdigest()
        root = self._project()
        p = self._run_sync(root, AI_SPECS_GATE_OFFLINE="1", AI_SPECS_GATE_BUILD="1")
        self.assertEqual(p.returncode, 0, p.stderr)
        self.assertTrue((cb.parent / "rejected" / f"worktree-gate-{stale_digest}").is_file())
        self.assertTrue(cb.is_file())
        self.assertEqual(hashlib.sha256(cb.read_bytes()).hexdigest(), _committed_digest())

    def test_committed_shasums_present_and_single_owner(self):
        # The trust root file must exist and carry the host (darwin-arm64)
        # entry that the build reproduces.
        self.assertTrue(SHA256SUMS.is_file())
        committed = _committed_digest()
        self.assertIsNotNone(committed)
        self.assertEqual(len(committed), 64)
        self.assertTrue(all(c in "0123456789abcdef" for c in committed))


if __name__ == "__main__":
    unittest.main()
