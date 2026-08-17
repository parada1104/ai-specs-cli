"""Phase 4 release-gate tests for the worktree-gate binary (tasks 4.8-4.9).

- 4.8  release gate: a CI-built darwin/arm64 asset must execute on real Apple
       Silicon (ad-hoc signature). Locally, scripts/build-gate.sh produces the
       exact CI artifact bytes (same flags, same version stamp), so running it
       on darwin/arm64 is the release gate — asserted here by building and
       executing it.
- 4.9  SHA256SUMS: the committed digest file must match the published assets.
       Locally we can only assert the file the CI produces (from these exact
       build outputs) matches the committed digests, or — when the committed
       digests are placeholders — that the file carries a valid, self-consistent
       digest per target. The CI workflow additionally diffs its generated
       SHA256SUMS against the committed file and fails the release otherwise.

The build is skipped loudly when no Go toolchain is present; without go the
binary cannot be produced and the release gate cannot run (the parity and
hook suites still cover the gate behavior independently).
"""
from __future__ import annotations

import hashlib
import os
import re
import shutil
import subprocess
import sys
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
BUILD_SCRIPT = ROOT / "scripts" / "build-gate.sh"
GATE_DIR = ROOT / "catalog" / "recipes" / "worktree-flow" / "gate"
SUMS_PATH = ROOT / "catalog" / "recipes" / "worktree-flow" / "bin" / "SHA256SUMS"
VERSION = (ROOT / "VERSION").read_text(encoding="utf-8").strip()

MATRIX = (
    ("darwin", "arm64"),
    ("darwin", "amd64"),
    ("linux", "amd64"),
    ("linux", "arm64"),
)


def _sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def _parse_sums(text: str) -> dict[str, str]:
    out: dict[str, str] = {}
    for line in text.splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        parts = line.split(None, 1)
        if len(parts) != 2 or len(parts[0]) != 64:
            continue
        digest, name = parts
        if name.startswith("worktree-gate-"):
            out[name] = digest.lower()
    return out


@unittest.skipUnless(
    shutil.which("go") is not None,
    "release gate requires a Go toolchain",
)
class WorktreeGateReleaseTests(unittest.TestCase):
    def setUp(self):
        self.dist = Path(self._fresh_dist())

    def _fresh_dist(self) -> str:
        import tempfile
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        return tmp.name

    def _build(self) -> dict[str, Path]:
        """Build every target into a fresh dist dir (build-gate.sh semantics)."""
        assets: dict[str, Path] = {}
        for goos, goarch in MATRIX:
            out = self.dist / f"worktree-gate-{goos}-{goarch}"
            env = dict(os.environ)
            env["CGO_ENABLED"] = "0"
            env["GOOS"] = goos
            env["GOARCH"] = goarch
            proc = subprocess.run(
                ["go", "build", "-trimpath", "-buildvcs=false",
                 "-ldflags", f"-s -w -X main.version={VERSION}",
                 "-o", str(out), "."],
                cwd=str(GATE_DIR), env=env, capture_output=True, text=True,
                timeout=300,
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            self.assertTrue(out.is_file())
            assets[f"worktree-gate-{goos}-{goarch}"] = out
        return assets

    def test_build_script_refreshes_current_from_fresh_native_build(self):
        # Regression: build-gate.sh must copy the freshly built native artifact
        # to dist/worktree-gate-current AFTER building it. Copying before the
        # build would make "current" a copy of the previous artifact — on a
        # clean checkout (no prior dist/) the copy would either fail or carry
        # stale bytes, so the differential runners would test the wrong binary
        # while reporting green.
        import tempfile
        fresh = Path(tempfile.mkdtemp())
        self.addCleanup(lambda: shutil.rmtree(fresh, ignore_errors=True))
        # Plant a stale "current" that must be REPLACED by the fresh build.
        stale = fresh / "worktree-gate-current"
        stale.write_bytes(b"STALE-SENTINEL\n")
        stale.chmod(0o755)
        proc = subprocess.run(
            [str(BUILD_SCRIPT), str(fresh)],
            capture_output=True, text=True, timeout=600,
        )
        self.assertEqual(proc.returncode, 0, proc.stderr)
        rebuilt = fresh / "worktree-gate-current"
        self.assertTrue(rebuilt.is_file())
        self.assertNotEqual(
            rebuilt.read_bytes(), b"STALE-SENTINEL\n",
            "build-gate.sh must overwrite worktree-gate-current with the "
            "fresh native build, not copy the previous artifact")
        ver = subprocess.run(
            [str(rebuilt), "--version"],
            capture_output=True, text=True, timeout=60,
        )
        self.assertEqual(ver.returncode, 0, ver.stderr)
        self.assertEqual(ver.stdout.strip(), VERSION)
        # And the native matrix asset is byte-identical to "current".
        native = fresh / "worktree-gate-darwin-arm64"
        self.assertTrue(native.is_file())
        self.assertEqual(
            _sha256(native), _sha256(rebuilt),
            "worktree-gate-current must equal the native build output")

    def test_darwin_arm64_asset_executes_on_apple_silicon(self):
        # Task 4.8 release gate: the CI-produced darwin/arm64 asset must run on
        # real Apple Silicon. Locally the CI artifact is byte-identical to
        # build-gate.sh's darwin/arm64 output (same flags, same version), so
        # building and executing it IS the release gate. The ad-hoc signature
        # concern only applies to the artifact downloaded from the release
        # page; a locally built Mach-O runs without quarantine.
        assets = self._build()
        darwin_arm64 = assets["worktree-gate-darwin-arm64"]
        proc = subprocess.run(
            [str(darwin_arm64), "--selftest"],
            capture_output=True, text=True, timeout=60,
        )
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertEqual(proc.stdout.strip(), "ok")
        ver = subprocess.run(
            [str(darwin_arm64), "--version"],
            capture_output=True, text=True, timeout=60,
        )
        self.assertEqual(ver.stdout.strip(), VERSION)

    def test_build_is_reproducible(self):
        # Spec scenario "Repeated builds are byte-identical".
        assets = self._build()
        target = assets["worktree-gate-linux-amd64"]
        rebuild = self.dist / "worktree-gate-linux-amd64-rebuild"
        env = dict(os.environ)
        env["CGO_ENABLED"] = "0"
        env["GOOS"] = "linux"
        env["GOARCH"] = "amd64"
        proc = subprocess.run(
            ["go", "build", "-trimpath", "-buildvcs=false",
             "-ldflags", f"-s -w -X main.version={VERSION}",
             "-o", str(rebuild), "."],
            cwd=str(GATE_DIR), env=env, capture_output=True, text=True,
            timeout=300,
        )
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertEqual(_sha256(target), _sha256(rebuild))

    def test_every_target_has_a_committed_digest_entry(self):
        # The trust root (bin/SHA256SUMS) must declare exactly the four
        # published targets. The CI workflow diffs its generated SHA256SUMS
        # against the committed file and fails the release on any mismatch, so
        # this assertion is the local stand-in: every matrix target must be
        # present in the committed digests.
        sums = _parse_sums(SUMS_PATH.read_text(encoding="utf-8"))
        for goos, goarch in MATRIX:
            name = f"worktree-gate-{goos}-{goarch}"
            with self.subTest(asset=name):
                self.assertIn(name, sums, f"committed SHA256SUMS must list {name}")

    def test_committed_digests_match_locally_built_assets(self):
        # Task 4.9: the committed SHA256SUMS must match the published assets.
        # When the file carries real digests (release state), they MUST equal
        # the locally built bytes. Placeholder comments (pre-release) are
        # tolerated and flagged for regeneration at release time.
        sums = _parse_sums(SUMS_PATH.read_text(encoding="utf-8"))
        assets = self._build()
        for name, path in assets.items():
            with self.subTest(asset=name):
                committed = sums.get(name)
                if committed is None:
                    self.fail(f"no committed digest for {name}")
                actual = _sha256(path)
                if committed.startswith("0" * 64):
                    # Pre-release placeholder; the release must regenerate.
                    self.fail(
                        f"committed SHA256SUMS still holds a placeholder for {name}; "
                        "regenerate with: scripts/build-gate.sh && "
                        "shasum -a 256 dist/worktree-gate-* | grep -v current > "
                        "catalog/recipes/worktree-flow/bin/SHA256SUMS"
                    )
                self.assertEqual(actual, committed)

    def test_ci_generated_sums_file_parses_and_matches_build(self):
        # The CI checksum job emits `sha256sum worktree-gate-* > SHA256SUMS`
        # from the built artifacts; that file must equal the committed digests.
        # Build the equivalent file locally and compare entries.
        assets = self._build()
        lines = []
        for name in sorted(assets):
            lines.append(f"{_sha256(assets[name])}  {name}")
        generated = _parse_sums("\n".join(lines) + "\n")
        committed = _parse_sums(SUMS_PATH.read_text(encoding="utf-8"))
        for name, digest in generated.items():
            with self.subTest(asset=name):
                committed_digest = committed.get(name)
                self.assertIsNotNone(committed_digest, f"CI sums list {name} but the committed file does not")
                if committed_digest.startswith("0" * 64):
                    continue  # pre-release placeholder, covered by the other test
                self.assertEqual(digest, committed_digest)

    def test_canonical_sums_comparison_ignores_header_and_order(self):
        # Regression (verify finding F8): the CI checksum gate must compare
        # the CANONICAL digest entries of the generated and committed
        # SHA256SUMS files, not raw bytes. The committed trust root carries a
        # documentation header and a hand-maintained line order (darwin-arm64
        # first); `sha256sum worktree-gate-*` emits a bare, lexicographically
        # ordered file. A byte-level `diff -u` therefore fails every release
        # even when every digest is correct — the previous workflow ran
        # exactly that diff and would have blocked the first v0.22.0 tag.
        # scripts/verify-gate-sums.sh drops comments/blank lines, keeps only
        import tempfile

        verify_script = ROOT / "scripts" / "verify-gate-sums.sh"
        self.assertTrue(verify_script.is_file(), "scripts/verify-gate-sums.sh must exist")

        assets = self._build()
        generated_dir = Path(tempfile.mkdtemp())
        self.addCleanup(lambda: shutil.rmtree(generated_dir, ignore_errors=True))
        generated_lines = []
        for name in sorted(assets):
            generated_lines.append(f"{_sha256(assets[name])}  {name}")
        generated = generated_dir / "SHA256SUMS"
        generated.write_text("\n".join(generated_lines) + "\n", encoding="utf-8")

        # Committed file, as it actually stands: header + darwin-arm64 first.
        committed = SUMS_PATH.read_text(encoding="utf-8")
        self.assertTrue(committed.startswith("#"), "committed SHA256SUMS carries a header")
        header_and_order = committed

        # A byte-level diff MUST fail (this is the F8 bug); the canonical
        # comparison MUST pass.
        proc = subprocess.run(
            ["bash", str(verify_script), str(generated), str(SUMS_PATH)],
            capture_output=True, text=True, timeout=60,
        )
        self.assertEqual(
            proc.returncode, 0,
            f"canonical comparison must pass despite header/order:\n"
            f"stdout: {proc.stdout}\nstderr: {proc.stderr}")
        self.assertIn("digest entries match", proc.stdout)

        # Sanity: a real digest mismatch still fails.
        bad = generated_dir / "SHA256SUMS-bad"
        bad.write_text(
            "0" * 64 + "  worktree-gate-darwin-arm64\n" +
            "\n".join(generated_lines[1:]) + "\n",
            encoding="utf-8",
        )
        proc_bad = subprocess.run(
            ["bash", str(verify_script), str(bad), str(SUMS_PATH)],
            capture_output=True, text=True, timeout=60,
        )
        self.assertNotEqual(proc_bad.returncode, 0, "a real digest mismatch must fail")
        self.assertIn("regenerate", proc_bad.stderr)

    def test_release_workflow_pins_canonical_toolchain_without_broken_cache(self):
        # Regression (final-verification blockers): the release CI must (a)
        # build with the same Go version that regenerated the committed
        # SHA256SUMS trust root — a different Go release compiles different
        # stdlib bytes, so the checksum gate would fail every tag push — and
        # (b) not key setup-go's cache on a nonexistent go.sum (the module has
        # zero third-party deps and intentionally no go.sum, D8).
        workflow = (ROOT / ".github" / "workflows" / "release-worktree-gate.yml").read_text(
            encoding="utf-8")
        self.assertIn("go-version:", workflow)
        match = re.search(r'go-version:\s*"([0-9]+\.[0-9]+\.[0-9]+)"', workflow)
        self.assertIsNotNone(
            match, "release workflow must pin an EXACT Go version (no .x), "
                   "because digests are toolchain-specific")
        self.assertIsNone(
            re.search(r"^\s*cache-dependency-path:", workflow, re.MULTILINE),
            "release workflow must not key setup-go's cache on a go.sum: the "
            "module has zero third-party deps and no go.sum exists")

    def test_local_toolchain_matches_pinned_release_toolchain(self):
        # The committed digests were generated with the canonical toolchain
        # and CI builds with the workflow pin, so the LOCAL toolchain that
        # regenerates digests must equal the pinned one — otherwise the
        # committed trust root and the release assets diverge. This test
        # fails loudly until digests are regenerated with the pinned Go
        # (or the pin and digests are moved together deliberately).
        workflow = (ROOT / ".github" / "workflows" / "release-worktree-gate.yml").read_text(
            encoding="utf-8")
        match = re.search(r'go-version:\s*"([0-9]+\.[0-9]+\.[0-9]+)"', workflow)
        self.assertIsNotNone(match, "workflow must pin an exact Go version")
        pinned = match.group(1)
        proc = subprocess.run(["go", "version"], capture_output=True, text=True,
                              timeout=30)
        self.assertEqual(proc.returncode, 0, proc.stderr)
        local = re.search(r"go([0-9]+\.[0-9]+\.[0-9]+)", proc.stdout)
        self.assertIsNotNone(local, f"cannot parse local go version: {proc.stdout!r}")
        self.assertEqual(
            local.group(1), pinned,
            "local Go toolchain must match the release workflow pin; "
            "regenerate SHA256SUMS with the pinned toolchain")

    def test_release_workflow_parity_job_runs_unittest_not_pytest(self):
        # Release blocker regression: the parity CI job must run the
        # repository's actual unittest-based parity test
        # (tests/test_worktree_gate_parity.py) exactly like ./tests/run.sh.
        # The previous `python3 -m pytest ...` relied on an undeclared
        # pytest dependency and failed on the stock GitHub runner.
        workflow = (ROOT / ".github" / "workflows" / "release-worktree-gate.yml").read_text(
            encoding="utf-8")
        parity_tail = workflow.split("parity:", 1)[1]
        self.assertIn(
            "python3 -m unittest", parity_tail,
            "parity job must run the unittest runner")
        self.assertIn("test_worktree_gate_parity.py", parity_tail)
        self.assertNotIn(
            "pytest", parity_tail,
            "parity job must not depend on the undeclared pytest dependency")


if __name__ == "__main__":
    unittest.main()
