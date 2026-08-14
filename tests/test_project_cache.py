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

    def test_backups_root_is_cache_scoped_outside_project(self):
        """3.2 — RED: gate backups live under the CLI cache, never the project."""
        root = self._tmp_project()
        home = self._tmp_home()
        self.mod.ensure_cache(root, cli_home=home)
        backups = self.mod.backups_root(root, cli_home=home)
        self.assertEqual(
            backups, self.mod.cache_root(root, cli_home=home) / "backups"
        )
        self.assertTrue(str(backups).startswith(str(home.resolve())),
                        "backups must be cache-scoped to the CLI home")
        self.assertNotIn(str(root), str(backups),
                         "backups must never land inside the project")

    def test_gate_backup_path_content_hash_collision_safe(self):
        """3.2 — RED: backup naming is deterministic, immutable, collision-safe."""
        import hashlib
        root = self._tmp_project()
        home = self._tmp_home()
        self.mod.ensure_cache(root, cli_home=home)
        rel = "ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh"
        sha_a = hashlib.sha256(b"# custom A\n").hexdigest()
        sha_b = hashlib.sha256(b"# custom B\n").hexdigest()
        p1 = self.mod.gate_backup_path(root, rel, sha_a, cli_home=home)
        p2 = self.mod.gate_backup_path(root, rel, sha_a, cli_home=home)
        p3 = self.mod.gate_backup_path(root, rel, sha_b, cli_home=home)
        p4 = self.mod.gate_backup_path(
            root, "ai-specs/recipes/worktree-flow/hooks/other-gate.sh", sha_a, cli_home=home
        )
        self.assertEqual(p1, p2, "same content + rel must be immutable/stable")
        self.assertNotEqual(p1, p3, "different content must not collide")
        self.assertNotEqual(p1, p4, "different rel must not collide")
        self.assertEqual(p1.suffix, ".sh")
        self.assertEqual(p1.parent.parent, self.mod.backups_root(root, cli_home=home))
        self.assertEqual(p1.stem, sha_a)

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


class BundledCommandPathTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(PROJECT_CACHE_PATH, "project_cache_bundled_cmd_paths")

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

    def test_bundled_commands_root_is_bundled_skills_root_sibling(self):
        root = self._tmp_project()
        home = self._tmp_home()
        self.mod.ensure_cache(root, cli_home=home)
        commands_root = self.mod.bundled_commands_root(root, cli_home=home)
        skills_root = self.mod.bundled_skills_root(root, cli_home=home)
        self.assertEqual(commands_root, skills_root / "commands")
        self.assertEqual(commands_root, self.mod.cache_root(root, cli_home=home) / ".bundled" / "commands")

    def test_bundled_command_ids_returns_md_stems(self):
        home = self._tmp_home()
        (home / "bundled-commands").mkdir()
        (home / "bundled-commands" / "rules-audit.md").write_text("# rules audit\n")
        (home / "bundled-commands" / "skills-as-rules.md").write_text("# skills as rules\n")
        (home / "bundled-commands" / "README.txt").write_text("not a command\n")
        ids = self.mod.bundled_command_ids(cli_home=home)
        self.assertEqual(ids, ["rules-audit", "skills-as-rules"])

    def test_bundled_command_ids_empty_when_dir_missing(self):
        home = self._tmp_home()
        self.assertEqual(self.mod.bundled_command_ids(cli_home=home), [])


if __name__ == "__main__":
    unittest.main()


