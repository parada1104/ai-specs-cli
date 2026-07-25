"""Unit tests for root .gitignore managed agent-block refresh."""

from __future__ import annotations

import importlib.util
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
MODULE_PATH = ROOT / "lib" / "_internal" / "gitignore-root-refresh.py"
TEMPLATE = ROOT / "templates" / "gitignore-root.tmpl"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


class GitignoreRootRefreshTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(MODULE_PATH, "gitignore_root_refresh")

    def _tmp_root(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name) / "proj"
        root.mkdir()
        return root

    def test_refresh_updates_stale_agent_block_with_pi(self):
        root = self._tmp_root()
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

        action = self.mod.refresh_root_gitignore(root, TEMPLATE)
        self.assertEqual(action, "refreshed")
        text = gitignore.read_text()
        self.assertIn(".pi/", text)
        self.assertIn(".omp/", text)
        self.assertIn("node_modules/", text)
        self.assertIn("dist/", text)
        self.assertEqual(text.count("# --- end ai-specs ---"), 1)

    def test_refresh_appends_block_when_missing(self):
        root = self._tmp_root()
        gitignore = root / ".gitignore"
        gitignore.write_text("*.log\n")

        action = self.mod.refresh_root_gitignore(root, TEMPLATE)
        self.assertEqual(action, "appended")
        text = gitignore.read_text()
        self.assertTrue(text.startswith("*.log\n"))
        self.assertIn(".pi/", text)
        self.assertIn(
            "# --- ai-specs: agent-generated files (managed by ai-specs sync-agent) ---",
            text,
        )

    def test_root_template_ignores_harness_env_secrets(self):
        """JD-2: consumer root gitignore must ignore ai-specs/.env (and dotenv entrypoints)."""
        text = TEMPLATE.read_text(encoding="utf-8")
        self.assertIn("ai-specs/.env", text)
        self.assertIn(".env", text.splitlines())
        self.assertIn(".envrc", text.splitlines())

    def test_refresh_appends_ai_specs_env_ignore(self):
        root = self._tmp_root()
        action = self.mod.refresh_root_gitignore(root, TEMPLATE)
        self.assertEqual(action, "appended")
        text = (root / ".gitignore").read_text(encoding="utf-8")
        self.assertIn("ai-specs/.env", text)


if __name__ == "__main__":
    unittest.main()
