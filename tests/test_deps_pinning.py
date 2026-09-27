"""Black-box tests for deps-layer reproducibility (D16 pinning, D17 lock hashes).

Drives bin/ai-specs sync in a hermetic temp workspace with a local git fixture
repo as the dep source. No lib/_internal imports: every assertion observes CLI
exit codes, stdout/stderr, and emitted file trees.
"""

import hashlib
import os
import subprocess
import tempfile
import tomllib
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"

UPSTREAM_V1 = (
    "---\n"
    "name: upstream-demo\n"
    "description: >\n"
    "  Upstream vendored skill. Trigger: Upstream trigger.\n"
    "---\n\n"
    "# Vendored Demo\n\n"
    "V1 body.\n"
)
UPSTREAM_V2 = UPSTREAM_V1.replace("V1 body.", "V2 body.")
RUN_SCRIPT = "#!/bin/sh\necho v1\n"


def _git(repo: Path, *args: str) -> None:
    subprocess.run(
        ["git", "-c", "user.name=Fixture", "-c", "user.email=fixture@example.com", *args],
        cwd=repo, check=True, capture_output=True, text=True,
    )


class DepsReproducibilityTests(unittest.TestCase):
    """D16: [[deps]] ref/rev pinning honored by the vendor clone.
    D17: dep content hashes recorded in ai-specs/.ai-specs.lock."""

    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        self.project = root / "project"
        self.project.mkdir()
        self.repo = root / "dep-skill"

        (self.repo / "scripts").mkdir(parents=True)
        (self.repo / "SKILL.md").write_text(UPSTREAM_V1, encoding="utf-8")
        (self.repo / "scripts" / "run.sh").write_text(RUN_SCRIPT, encoding="utf-8")
        _git(self.repo, "init")
        _git(self.repo, "add", "-A")
        _git(self.repo, "commit", "-m", "v1")
        _git(self.repo, "tag", "v1")
        self.sha_v1 = subprocess.run(
            ["git", "rev-parse", "HEAD"], cwd=self.repo,
            check=True, capture_output=True, text=True,
        ).stdout.strip()
        (self.repo / "SKILL.md").write_text(UPSTREAM_V2, encoding="utf-8")
        _git(self.repo, "add", "-A")
        _git(self.repo, "commit", "-m", "v2")

        subprocess.run(
            [str(CLI), "init", str(self.project)],
            check=True, capture_output=True, text=True,
        )
        self._write_manifest("")

    def _write_manifest(self, pin: str) -> None:
        toml = self.project / "ai-specs" / "ai-specs.toml"
        toml.write_text(
            toml.read_text(encoding="utf-8")
            + "\n[[deps]]\n"
            + 'id = "vendored-demo"\n'
            + f'source = "{self.repo}"\n'
            + pin
            + 'scope = ["root"]\n',
            encoding="utf-8",
        )

    def _sync(self) -> subprocess.CompletedProcess:
        return subprocess.run(
            [str(CLI), "sync", str(self.project)],
            capture_output=True, text=True, check=False,
            env={**os.environ, "AI_SPECS_GATE_OFFLINE": "1"},
        )

    def _vendored(self) -> Path:
        return (
            self.project / "ai-specs" / ".deps" / "vendored-demo"
            / "skills" / "vendored-demo" / "SKILL.md"
        )

    # ── D16 ──

    def test_ref_tag_pins_vendored_content(self):
        self._write_manifest('ref = "v1"\n')
        proc = self._sync()
        self.assertEqual(proc.returncode, 0, proc.stderr)
        content = self._vendored().read_text(encoding="utf-8")
        self.assertIn("V1 body.", content)
        self.assertNotIn("V2 body.", content)

    def test_rev_alias_pins_vendored_content(self):
        self._write_manifest('rev = "v1"\n')
        proc = self._sync()
        self.assertEqual(proc.returncode, 0, proc.stderr)
        content = self._vendored().read_text(encoding="utf-8")
        self.assertIn("V1 body.", content)
        self.assertNotIn("V2 body.", content)

    def test_ref_sha_pins_vendored_content(self):
        self._write_manifest(f'ref = "{self.sha_v1}"\n')
        proc = self._sync()
        self.assertEqual(proc.returncode, 0, proc.stderr)
        content = self._vendored().read_text(encoding="utf-8")
        self.assertIn("V1 body.", content)
        self.assertNotIn("V2 body.", content)

    def test_no_ref_vendors_default_head(self):
        proc = self._sync()
        self.assertEqual(proc.returncode, 0, proc.stderr)
        content = self._vendored().read_text(encoding="utf-8")
        self.assertIn("V2 body.", content)

    # ── D17 ──

    def test_dep_hashes_recorded_in_lock(self):
        self._write_manifest('ref = "v1"\n')
        proc = self._sync()
        self.assertEqual(proc.returncode, 0, proc.stderr)

        vendored = self._vendored()
        skill_bytes = vendored.read_bytes()
        script_bytes = (vendored.parent / "scripts" / "run.sh").read_bytes()
        expected = {
            "SKILL.md": hashlib.sha256(skill_bytes.replace(b"\r\n", b"\n")).hexdigest(),
            "scripts/run.sh": hashlib.sha256(script_bytes.replace(b"\r\n", b"\n")).hexdigest(),
        }

        lock_path = self.project / "ai-specs" / ".ai-specs.lock"
        with open(lock_path, "rb") as f:
            data = tomllib.load(f)
        deps = data.get("deps", {})
        self.assertIn("vendored-demo", deps)
        hashes = deps["vendored-demo"]["skills"]["vendored-demo"]
        self.assertEqual(hashes.get("SKILL.md"), expected["SKILL.md"])
        self.assertEqual(hashes.get("scripts/run.sh"), expected["scripts/run.sh"])


if __name__ == "__main__":
    unittest.main()
