"""Unit tests for command merge (cache managed + local hand-authored)."""

from __future__ import annotations

import importlib.util
import io
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
PROJECT_CACHE_PATH = ROOT / "lib" / "_internal" / "project-cache.py"


def load_module(path: Path, name: str):
    internal_dir = str(path.parent)
    if internal_dir not in sys.path:
        sys.path.insert(0, internal_dir)
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


class CommandMergeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(PROJECT_CACHE_PATH, "project_cache_cmd_merge")

    def _project(self) -> tuple[Path, Path]:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name) / "proj"
        root.mkdir()
        (root / "ai-specs" / "commands").mkdir(parents=True)
        home = Path(tmp.name) / "home"
        home.mkdir()
        self.mod.ensure_cache(root, cli_home=home)
        return root, home

    def test_local_wins_over_managed(self):
        root, home = self._project()
        managed = self.mod.commands_dir(root, cli_home=home)
        managed.mkdir(parents=True, exist_ok=True)
        (managed / "shared.md").write_text("managed\n")
        (managed / "only-managed.md").write_text("m\n")
        (root / "ai-specs" / "commands" / "shared.md").write_text("local\n")
        (root / "ai-specs" / "commands" / "only-local.md").write_text("l\n")

        dest = Path(tempfile.mkdtemp())
        self.addCleanup(lambda: __import__("shutil").rmtree(dest, ignore_errors=True))
        captured = io.StringIO()
        old = sys.stderr
        sys.stderr = captured
        try:
            n = self.mod.merge_commands(root, dest, cli_home=home)
        finally:
            sys.stderr = old

        self.assertEqual((dest / "shared.md").read_text(), "local\n")
        self.assertEqual((dest / "only-managed.md").read_text(), "m\n")
        self.assertEqual((dest / "only-local.md").read_text(), "l\n")
        self.assertEqual(n, 3)
        self.assertIn("local hand-authored wins", captured.getvalue())

    def test_managed_only(self):
        root, home = self._project()
        managed = self.mod.commands_dir(root, cli_home=home)
        managed.mkdir(parents=True, exist_ok=True)
        (managed / "a.md").write_text("a\n")
        dest = Path(tempfile.mkdtemp())
        self.addCleanup(lambda: __import__("shutil").rmtree(dest, ignore_errors=True))
        n = self.mod.merge_commands(root, dest, cli_home=home)
        self.assertEqual(n, 1)
        self.assertEqual((dest / "a.md").read_text(), "a\n")


if __name__ == "__main__":
    unittest.main()
