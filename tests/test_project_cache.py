"""Unit tests for project-recipe-cache helpers."""

from __future__ import annotations

import importlib.util
import os
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


class ProjectCacheTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(PROJECT_CACHE_PATH, "project_cache_internal")

    def _tmp_project(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name) / "my-project"
        root.mkdir()
        (root / "ai-specs").mkdir()
        return root.resolve()

    def _tmp_home(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        home = Path(tmp.name) / "cli-home"
        home.mkdir()
        return home

    def test_cache_key_stable_and_includes_basename(self):
        root = self._tmp_project()
        key1 = self.mod.cache_key(root)
        key2 = self.mod.cache_key(root)
        self.assertEqual(key1, key2)
        self.assertTrue(key1.endswith("-my-project"))
        prefix, sep, suffix = key1.partition("-")
        self.assertEqual(sep, "-")
        self.assertEqual(len(prefix), 12)
        self.assertTrue(all(c in "0123456789abcdef" for c in prefix))

    def test_cache_key_differs_for_different_roots(self):
        a = self._tmp_project()
        b = self._tmp_project()
        self.assertNotEqual(self.mod.cache_key(a), self.mod.cache_key(b))

    def test_ensure_cache_writes_meta_toml(self):
        root = self._tmp_project()
        home = self._tmp_home()
        cache = self.mod.ensure_cache(root, cli_home=home)
        self.assertTrue(cache.is_dir())
        self.assertEqual(cache, (home / "cache" / "projects" / self.mod.cache_key(root)).resolve())
        meta = cache / "meta.toml"
        self.assertTrue(meta.is_file())
        text = meta.read_text()
        self.assertIn("project_root", text)
        self.assertIn(str(root), text)
        self.assertIn("created_at", text)

    def test_path_helpers(self):
        root = self._tmp_project()
        home = self._tmp_home()
        self.mod.ensure_cache(root, cli_home=home)
        self.assertEqual(
            self.mod.recipe_skills_root(root, cli_home=home),
            self.mod.cache_root(root, cli_home=home) / ".recipe",
        )
        self.assertEqual(
            self.mod.deps_skills_root(root, cli_home=home),
            self.mod.cache_root(root, cli_home=home) / ".deps",
        )
        self.assertEqual(
            self.mod.commands_dir(root, cli_home=home),
            self.mod.cache_root(root, cli_home=home) / "commands",
        )
        self.assertEqual(
            self.mod.resolved_skills_dir(root, cli_home=home),
            self.mod.cache_root(root, cli_home=home) / "resolved-skills",
        )

    def test_remove_legacy_origin_migrates_overrides_then_deletes(self):
        root = self._tmp_project()
        legacy_overrides = (
            root / "ai-specs" / ".recipe" / "demo" / "overrides"
        )
        legacy_overrides.mkdir(parents=True)
        (legacy_overrides / "config.toml").write_text("board_id = \"x\"\n")
        (legacy_overrides / "templates").mkdir()
        (legacy_overrides / "templates" / "card.md").write_text("override\n")
        (root / "ai-specs" / ".deps" / "dep-a").mkdir(parents=True)
        (root / "ai-specs" / ".deps" / "dep-a" / "marker").write_text("d\n")

        self.mod.remove_legacy_origin(root)

        dest = root / "ai-specs" / "recipes" / "demo" / "overrides"
        self.assertTrue((dest / "config.toml").is_file())
        self.assertEqual((dest / "config.toml").read_text(), 'board_id = "x"\n')
        self.assertTrue((dest / "templates" / "card.md").is_file())
        self.assertFalse((root / "ai-specs" / ".recipe").exists())
        # ai-specs/.deps/ is now the toml-dep materialization home (gitignored) —
        # it must be preserved, not deleted as legacy origin.
        self.assertTrue((root / "ai-specs" / ".deps").exists())

    def test_remove_legacy_origin_keeps_existing_overrides(self):
        root = self._tmp_project()
        existing = root / "ai-specs" / "recipes" / "demo" / "overrides"
        existing.mkdir(parents=True)
        (existing / "config.toml").write_text("keep = true\n")
        legacy = root / "ai-specs" / ".recipe" / "demo" / "overrides"
        legacy.mkdir(parents=True)
        (legacy / "config.toml").write_text("stale = true\n")

        self.mod.remove_legacy_origin(root)

        self.assertEqual((existing / "config.toml").read_text(), "keep = true\n")
        self.assertFalse((root / "ai-specs" / ".recipe").exists())

    def test_remove_legacy_origin_skips_recipe_rmtree_if_override_migration_failed(self):
        """If a copytree fails with OSError, .recipe/ must NOT be deleted (data loss guard)."""
        root = self._tmp_project()
        legacy_overrides = root / "ai-specs" / ".recipe" / "demo" / "overrides"
        legacy_overrides.mkdir(parents=True)
        (legacy_overrides / "config.toml").write_text('board_id = "x"\n')
        # Block migration: place a FILE where the recipe dir should be so mkdir raises OSError
        recipes_dir = root / "ai-specs" / "recipes"
        recipes_dir.mkdir(parents=True, exist_ok=True)
        (recipes_dir / "demo").write_text("blocking file\n")
        # Create .deps — the toml-dep home, which must be preserved.
        (root / "ai-specs" / ".deps" / "dep-a").mkdir(parents=True)
        (root / "ai-specs" / ".deps" / "dep-a" / "marker").write_text("d\n")

        self.mod.remove_legacy_origin(root)

        # .recipe/ must survive — overrides were not migrated
        self.assertTrue((root / "ai-specs" / ".recipe").exists())
        self.assertTrue(legacy_overrides.exists())
        # .deps/ is the toml-dep materialization home — preserved, not deleted.
        self.assertTrue((root / "ai-specs" / ".deps").exists())

    def test_remove_legacy_origin_deletes_resolved_skills_and_internal(self):
        root = self._tmp_project()
        (root / "ai-specs" / ".resolved-skills" / "old").mkdir(parents=True)
        (root / "ai-specs" / ".internal" / "resolved-skills" / "old").mkdir(
            parents=True
        )
        (root / "ai-specs" / ".resolved-skills" / "old" / "SKILL.md").write_text(
            "x\n"
        )

        self.mod.remove_legacy_origin(root)

        self.assertFalse((root / "ai-specs" / ".resolved-skills").exists())
        self.assertFalse((root / "ai-specs" / ".internal").exists())

    def test_remove_legacy_origin_deletes_stale_premerge_bin(self):
        root = self._tmp_project()
        bin_dir = root / "ai-specs" / "bin"
        bin_dir.mkdir()
        (bin_dir / "premerge_guardian.py").write_text("stale\n")
        (bin_dir / "keep-me.sh").write_text("#!/bin/sh\n")

        self.mod.remove_legacy_origin(root)

        self.assertFalse((bin_dir / "premerge_guardian.py").exists())
        self.assertTrue((bin_dir / "keep-me.sh").is_file())
        self.assertTrue(bin_dir.is_dir())

    def test_remove_legacy_origin_removes_empty_bin_after_guardian(self):
        root = self._tmp_project()
        bin_dir = root / "ai-specs" / "bin"
        bin_dir.mkdir()
        (bin_dir / "premerge_guardian.py").write_text("stale\n")

        self.mod.remove_legacy_origin(root)

        self.assertFalse(bin_dir.exists())


if __name__ == "__main__":
    unittest.main()


