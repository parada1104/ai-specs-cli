"""Group 8.2: `.ai-specs/run/` is gitignored after `ai-specs init`.

The init.sh script appends `templates/gitignore-root.tmpl` verbatim into
the project's root `.gitignore`. This test runs `ai-specs init` against a
tmp directory and asserts the `.ai-specs/run/` pattern is present within
the managed block. It also verifies the pattern actually causes git to
ignore files placed there (end-to-end behavioural check, not just a string
match against the template).
"""
from __future__ import annotations

import os
import subprocess
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
INIT_SH = ROOT / "lib" / "init.sh"
TEMPLATE = ROOT / "templates" / "gitignore-root.tmpl"
BEGIN = "# --- ai-specs: agent-generated files (managed by ai-specs sync-agent) ---"
END = "# --- end ai-specs ---"


class GitignoreRunDirTests(unittest.TestCase):
    def _run_init(self, target: Path) -> None:
        env = os.environ.copy()
        env.setdefault("HOME", str(target.parent))
        subprocess.run(
            ["bash", str(INIT_SH), str(target), "--name", "fixture"],
            check=True,
            capture_output=True,
            env=env,
        )

    def test_template_contains_ai_specs_run_pattern(self):
        text = TEMPLATE.read_text()
        self.assertIn(BEGIN, text)
        self.assertIn(END, text)
        # Pattern MUST live inside the managed block.
        begin_idx = text.index(BEGIN)
        end_idx = text.index(END)
        managed = text[begin_idx:end_idx]
        self.assertIn(
            ".ai-specs/run/",
            managed,
            "templates/gitignore-root.tmpl must list `.ai-specs/run/` "
            "inside the managed block so daemon state files are ignored.",
        )

    def test_init_writes_ai_specs_run_to_gitignore(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        target = Path(tmp.name) / "prj"
        target.mkdir()
        self._run_init(target)

        gitignore = target / ".gitignore"
        self.assertTrue(gitignore.is_file(), f"missing {gitignore}")
        content = gitignore.read_text()
        self.assertIn(BEGIN, content)
        self.assertIn(".ai-specs/run/", content)

    def test_git_actually_ignores_files_under_ai_specs_run(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        target = Path(tmp.name) / "prj"
        target.mkdir()
        subprocess.run(
            ["git", "init", "-q", str(target)], check=True, capture_output=True
        )
        self._run_init(target)

        run_dir = target / ".ai-specs" / "run"
        run_dir.mkdir(parents=True)
        (run_dir / "proxy.pid").write_text("12345\n")
        (run_dir / "proxy.port").write_text("8765\n")

        proc = subprocess.run(
            ["git", "-C", str(target), "status", "--porcelain", "--ignored"],
            check=True,
            capture_output=True,
            text=True,
        )
        # Untracked listing MUST NOT contain anything under .ai-specs/run/.
        # `--ignored` would surface ignored entries with a leading `!!` —
        # acceptable; the requirement is that they are not `??` (untracked).
        for line in proc.stdout.splitlines():
            if line.startswith("?? "):
                self.assertNotIn(
                    ".ai-specs/run/",
                    line,
                    f"git lists .ai-specs/run file as untracked: {line!r}",
                )


if __name__ == "__main__":
    unittest.main()
