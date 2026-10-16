"""GO-20 U1b-1: the CLI's release trust root is committed and reproducible.

The installer and the native upgrade engine both verify a downloaded
`ai-specs-<goos>-<goarch>` asset against `bin/SHA256SUMS` before installing it.
That only means something if the committed digests are the bytes a canonical
build produces, so this suite rebuilds the host platform's asset locally and
compares.
"""
from __future__ import annotations

import hashlib
import os
import platform
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
TRUST_ROOT = ROOT / "bin" / "SHA256SUMS"
BUILD_SCRIPT = ROOT / "scripts" / "build-cli.sh"
VERIFY_SCRIPT = ROOT / "scripts" / "verify-cli-sums.sh"
WORKFLOW = ROOT / ".github" / "workflows" / "release-binaries.yml"
OLD_WORKFLOW = ROOT / ".github" / "workflows" / "release-worktree-gate.yml"
CANONICAL_GO = "go1.24.13"
PLATFORMS = (
    ("darwin", "arm64"),
    ("darwin", "amd64"),
    ("linux", "amd64"),
    ("linux", "arm64"),
)


def _host_platform() -> tuple[str, str]:
    os_name = {"Darwin": "darwin", "Linux": "linux"}.get(platform.system())
    arch = {
        "arm64": "arm64", "aarch64": "arm64",
        "x86_64": "amd64", "amd64": "amd64",
    }.get(platform.machine().lower())
    if os_name is None or arch is None:
        raise unittest.SkipTest(f"unsupported host platform {platform.system()}/{platform.machine()}")
    return os_name, arch


def _trust_root_entries() -> dict[str, str]:
    entries: dict[str, str] = {}
    for raw in TRUST_ROOT.read_text(encoding="utf-8").splitlines():
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        fields = line.split()
        if len(fields) == 2 and len(fields[0]) == 64:
            entries[fields[1]] = fields[0]
    return entries


class CliReleaseTrustRootTests(unittest.TestCase):
    def test_trust_root_covers_every_release_platform(self):
        self.assertTrue(TRUST_ROOT.is_file(), f"{TRUST_ROOT} must exist: it is the trust root the installer reads")
        entries = _trust_root_entries()
        self.assertEqual(len(entries), len(PLATFORMS), entries)
        for goos, goarch in PLATFORMS:
            asset = f"ai-specs-{goos}-{goarch}"
            self.assertIn(asset, entries, f"{asset} must carry a digest")
            self.assertRegex(entries[asset], r"^[0-9a-f]{64}$")

    def test_trust_root_documents_the_canonical_toolchain(self):
        header = TRUST_ROOT.read_text(encoding="utf-8")
        self.assertIn(CANONICAL_GO, header, "the header must name the toolchain the digests were built with")
        self.assertIn("build-cli.sh", header, "the header must say how to reproduce a digest")

    def test_host_platform_digest_is_reproducible_locally(self):
        go = shutil.which("go")
        if go is None:
            self.skipTest("no Go toolchain: cannot reproduce the digest")
        goos, goarch = _host_platform()
        asset = f"ai-specs-{goos}-{goarch}"
        entries = _trust_root_entries()
        self.assertIn(asset, entries, f"{asset} must be in the trust root")

        with tempfile.TemporaryDirectory(prefix="ai-specs-cli-release-") as tmp:
            out = Path(tmp) / asset
            cmd = [go, "build", "-trimpath", "-buildvcs=false", "-o", str(out), "./cmd/ai-specs"]
            env = {**os.environ, "CGO_ENABLED": "0", "GOOS": goos, "GOARCH": goarch}
            proc = subprocess.run(cmd, cwd=str(ROOT), env=env, capture_output=True, text=True, timeout=900)
            self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
            digest = hashlib.sha256(out.read_bytes()).hexdigest()

        self.assertEqual(
            digest,
            entries[asset],
            f"a canonical local build of {asset} must reproduce the committed digest; "
            f"regenerate bin/SHA256SUMS with scripts/build-cli.sh ({CANONICAL_GO}) and commit it",
        )

    def test_build_and_verify_scripts_exist_and_are_documented(self):
        self.assertTrue(BUILD_SCRIPT.is_file(), "scripts/build-cli.sh must exist")
        self.assertTrue(VERIFY_SCRIPT.is_file(), "scripts/verify-cli-sums.sh must exist")
        verify = VERIFY_SCRIPT.read_text(encoding="utf-8")
        self.assertIn("ai-specs-", verify, "the CLI verifier must canonicalize the CLI asset prefix")

    def test_release_workflow_builds_and_verifies_both_binaries(self):
        """The release channel is only real if CI builds, verifies and ships the
        CLI assets — and does not regress the gate's path while doing it."""
        self.assertTrue(WORKFLOW.is_file(), f"{WORKFLOW.name} must exist")
        self.assertFalse(
            OLD_WORKFLOW.exists(),
            "the old workflow name released only the gate; the file is release-binaries.yml now",
        )
        text = WORKFLOW.read_text(encoding="utf-8")
        # The CLI side: a matrix job, its own emitted sums, its own trust root.
        self.assertIn("build-cli:", text)
        self.assertIn("ai-specs-${{ matrix.goos }}-${{ matrix.goarch }}", text)
        self.assertIn("sha256sum ai-specs-* > CLI-SHA256SUMS", text)
        self.assertIn("verify-cli-sums.sh dist/CLI-SHA256SUMS bin/SHA256SUMS", text)
        self.assertIn("dist/ai-specs-*", text)
        # The gate side is not regressed by the addition.
        self.assertIn("verify-gate-sums.sh dist/SHA256SUMS", text)
        self.assertIn("catalog/recipes/worktree-flow/bin/SHA256SUMS", text)
        self.assertIn("dist/worktree-gate-*", text)


if __name__ == "__main__":
    unittest.main()
