"""Black-box tests for root .gitignore managed agent-block refresh via `bin/ai-specs sync`."""

from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

from _blackbox import invoke, isolated_home, temp_project


class GitignoreRootRefreshTests(unittest.TestCase):
    """Drive the root .gitignore agent-block step through `bin/ai-specs sync`."""

    def setUp(self):
        # ONE shared cli_home per test: AI_SPECS_HOME is both install and cache
        # root, and sync writes the project cache beneath it.
        home_tmp = tempfile.TemporaryDirectory(prefix="ai-specs-home-")
        self.addCleanup(home_tmp.cleanup)
        self._cli_home = isolated_home(Path(home_tmp.name))

    def _sync(self, root: Path):
        """Sync a project against the shared cli_home."""
        return invoke(root, "sync", cli_home=self._cli_home)

    def test_refresh_updates_stale_agent_block_with_pi(self):
        td, root = temp_project(agents=("claude",))
        self.addCleanup(td.cleanup)
        gitignore = root / ".gitignore"
        gitignore.write_text(
            "node_modules/\n"
            "\n"
            "# --- ai-specs: agent-generated files (managed by ai-specs sync-agent) ---\n"
            ".claude/\n"
            ".cursor/\n"
            "# --- end ai-specs ---\n"
            "\n"
            "dist/\n"
        )

        result = self._sync(root)
        self.assertEqual(result.returncode, 0)
        text = gitignore.read_text()
        self.assertIn(".pi/", text)
        self.assertIn(".omp/", text)
        self.assertIn("node_modules/", text)
        self.assertIn("dist/", text)
        self.assertEqual(text.count("# --- end ai-specs ---"), 1)

    def test_refresh_appends_block_when_missing(self):
        td, root = temp_project(agents=("claude",))
        self.addCleanup(td.cleanup)
        gitignore = root / ".gitignore"
        gitignore.write_text("*.log\n")

        result = self._sync(root)
        self.assertEqual(result.returncode, 0)
        text = gitignore.read_text()
        self.assertTrue(text.startswith("*.log\n"))
        self.assertIn(".pi/", text)
        self.assertIn(
            "# --- ai-specs: agent-generated files (managed by ai-specs sync-agent) ---",
            text,
        )

    def test_root_template_ignores_harness_env_secrets(self):
        """JD-2/JD-7: consumer root gitignore must ignore harness env + migration bak."""
        td, root = temp_project(agents=("claude",))
        self.addCleanup(td.cleanup)
        gitignore = root / ".gitignore"
        gitignore.write_text("*.log\n")
        result = self._sync(root)
        self.assertEqual(result.returncode, 0)
        text = gitignore.read_text(encoding="utf-8")
        self.assertTrue(text.startswith("*.log\n"))
        self.assertIn("ai-specs.env", text)
        self.assertIn("ai-specs/.env", text)
        self.assertIn("ai-specs/.env.bak", text)
        self.assertIn("ai-specs/.envrc.bak", text)
        self.assertIn(".env", text.splitlines())
        self.assertIn(".envrc", text.splitlines())

    def test_refresh_appends_ai_specs_env_ignore(self):
        td, root = temp_project(agents=("claude",))
        self.addCleanup(td.cleanup)
        gitignore = root / ".gitignore"
        gitignore.write_text("*.log\n")

        result = self._sync(root)
        self.assertEqual(result.returncode, 0)
        text = gitignore.read_text(encoding="utf-8")
        self.assertIn("ai-specs.env", text)
        self.assertIn("ai-specs/.env", text)
        self.assertIn("ai-specs/.env.bak", text)
        self.assertIn("ai-specs/.envrc.bak", text)


if __name__ == "__main__":
    unittest.main()
