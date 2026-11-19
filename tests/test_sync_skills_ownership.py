"""T6 (S1): sync never removes a real directory at an agent skills path.

Parity contract §4: skills dirs are an absolute symlink, and an existing real
directory (or file) there is a hard error, never overwritten. Before T6 the
spine ran ``rm -rf`` on it, so a user's own skills were deleted with exit 0.
"""

from __future__ import annotations

import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(Path(__file__).resolve().parent))

import _go_cli  # noqa: E402
from _blackbox import isolated_home  # noqa: E402

CLI = _go_cli.cli()
KEPANO_FIXTURE = ROOT / "tests" / "fixtures" / "kepano-obsidian-skills"


class SyncSkillsOwnershipTests(unittest.TestCase):
    def setUp(self) -> None:
        base = Path(tempfile.mkdtemp(prefix="ai-specs-skills-", dir="/private/tmp"))
        self.addCleanup(shutil.rmtree, base, ignore_errors=True)
        self.base = base
        self.home = isolated_home(base)
        self.project = base / "project"
        for name in ("skills", "commands"):
            (self.project / "ai-specs" / name).mkdir(parents=True)
        (self.project / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 't'\n\n[agents]\nenabled = ['claude']\n",
            encoding="utf-8",
        )
        self.skills = self.project / ".claude" / "skills"
        self.user_skill = self.skills / "mine" / "SKILL.md"
        self.user_skill.parent.mkdir(parents=True)
        self.user_skill.write_text("# mine\n", encoding="utf-8")

    def sync(self) -> subprocess.CompletedProcess:
        env = {
            "PATH": os.environ.get("PATH", ""),
            "HOME": str(self.base / "home"),
            "TMPDIR": str(self.base / "tmp"),
            "AI_SPECS_HOME": str(self.home),
            "AI_SPECS_GATE_OFFLINE": "1",
            "AI_SPECS_NO_NETWORK": "1",
            "AI_SPECS_VENDOR_FIXTURE_ROOT": str(KEPANO_FIXTURE),
            "LC_ALL": "C",
            "LANG": "C",
        }
        for name in ("home", "tmp"):
            (self.base / name).mkdir(exist_ok=True)
        return subprocess.run(
            [str(CLI), "sync", str(self.project)],
            cwd=ROOT,
            env=env,
            text=True,
            capture_output=True,
            check=False,
            input="",
        )

    def test_real_skills_dir_is_refused_and_left_untouched(self) -> None:
        proc = self.sync()
        self.assertEqual(proc.returncode, 1, proc.stdout + proc.stderr)
        refusal = f"    ✗ refuse to overwrite non-symlink: {self.skills}"
        self.assertIn(refusal, proc.stderr)
        self.assertIn("move or delete it so ai-specs can create the symlink", proc.stderr)
        # Nothing in the directory was touched, and the refusal happened before
        # any of this agent's other writes.
        self.assertEqual(self.user_skill.read_text(encoding="utf-8"), "# mine\n")
        self.assertFalse(self.skills.is_symlink())
        self.assertFalse((self.project / ".claude" / "commands").exists())
        self.assertFalse((self.project / "CLAUDE.md").exists())


if __name__ == "__main__":
    unittest.main()
