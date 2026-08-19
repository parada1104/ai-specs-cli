"""Black-box coverage for project cache behavior exposed by the CLI."""
from __future__ import annotations

import hashlib
import importlib.util
import tempfile
import unittest
from pathlib import Path

from _blackbox import cache_project_dir, cache_project_key, isolated_home, invoke, temp_project


class ProjectCacheTests(unittest.TestCase):
    def _project(self, name: str = "fixture") -> tuple[Path, Path]:
        td, root = temp_project(name=name)
        self.addCleanup(td.cleanup)
        home_td = tempfile.TemporaryDirectory()
        self.addCleanup(home_td.cleanup)
        home = isolated_home(Path(home_td.name))
        return root, home
    def _refresh(self, root: Path, home: Path):
        result = invoke(root, "refresh-bundled", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        return result

    def _sync(self, root: Path, home: Path):
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        return result

    def _cache(self, root: Path, home: Path) -> Path:
        return cache_project_dir(root, home)

    def test_cache_key_stable_and_includes_basename(self):
        root, home = self._project()
        self._refresh(root, home)
        key1 = cache_project_key(root)
        key2 = cache_project_key(root)
        self.assertEqual(key1, key2)
        self.assertTrue(key1.endswith("-" + root.name))
        prefix, sep, suffix = key1.partition("-")
        self.assertEqual(sep, "-")
        self.assertEqual(suffix, root.name)
        self.assertEqual(len(prefix), 12)
        self.assertTrue(all(c in "0123456789abcdef" for c in prefix))

    def test_cache_key_differs_for_different_roots(self):
        root_a, home = self._project("a")
        root_b, _ = self._project("b")
        self._refresh(root_a, home)
        self.assertNotEqual(cache_project_key(root_a), cache_project_key(root_b))
        self.assertNotEqual(self._cache(root_a, home), self._cache(root_b, home))
        self.assertTrue(self._cache(root_a, home).is_dir())

    def test_cache_key_matches_frozen_implementation(self):
        root, home = self._project()
        self._refresh(root, home)
        implementation = Path(__file__).resolve().parents[1] / "lib" / "_internal" / "project-cache.py"
        spec = importlib.util.spec_from_file_location("project_cache_parity", implementation)
        self.assertIsNotNone(spec)
        self.assertIsNotNone(spec.loader)
        module = importlib.util.module_from_spec(spec)
        assert spec.loader is not None
        spec.loader.exec_module(module)
        self.assertEqual(cache_project_key(root), module.cache_key(root))

    def test_ensure_cache_writes_meta_toml(self):
        root, home = self._project()
        self._sync(root, home)
        cache = self._cache(root, home)
        self.assertTrue(cache.is_dir())
        self.assertEqual(cache.resolve(), (home / "cache" / "projects" / cache_project_key(root)).resolve())
        meta = cache / "meta.toml"
        self.assertTrue(meta.is_file())
        text = meta.read_text()
        self.assertIn("project_root", text)
        self.assertIn(str(root.resolve()), text)
        self.assertIn("created_at", text)

    def test_path_helpers_materialize_expected_cache_layout(self):
        root, home = self._project()
        self._sync(root, home)
        cache = self._cache(root, home)
        self.assertTrue((cache / ".bundled").is_dir())
        self.assertTrue((cache / ".bundled" / "skills").is_dir())
        self.assertTrue((cache / ".bundled" / "commands").is_dir())
        self.assertTrue((cache / "meta.toml").is_file())
        self.assertFalse((root / "cache").exists())

    def test_backups_root_is_cache_scoped_outside_project(self):
        root, home = self._project()
        self._sync(root, home)
        backups = self._cache(root, home) / "backups"
        self.assertTrue(backups.resolve().is_relative_to(home.resolve()))
        self.assertFalse(backups.resolve().is_relative_to(root.resolve()))
        self.assertEqual(backups.parent.resolve(), self._cache(root, home).resolve())
        self.assertFalse((root / "ai-specs" / "backups").exists())

    def test_gate_backup_path_content_hash_collision_safe(self):
        root, home = self._project()
        self._sync(root, home)
        rel = "ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh"
        sha_a = hashlib.sha256(b"# custom A\n").hexdigest()
        sha_b = hashlib.sha256(b"# custom B\n").hexdigest()
        self.assertNotEqual(sha_a, sha_b)
        self.assertEqual(Path(rel).suffix, ".sh")
        self.assertEqual(len(sha_a), 64)
        backup_root = self._cache(root, home) / "backups"
        self.assertNotEqual(backup_root / sha_a, backup_root / sha_b)
        self.assertFalse((root / "ai-specs" / "backups" / sha_a).exists())


    def test_remove_legacy_origin_migrates_overrides_then_deletes(self):
        root, home = self._project()
        legacy = root / "ai-specs" / ".recipe" / "demo" / "overrides"
        legacy.mkdir(parents=True)
        (legacy / "config.toml").write_text('board_id = "x"\n')
        (legacy / "templates").mkdir()
        (legacy / "templates" / "card.md").write_text("override\n")
        (root / "ai-specs" / ".deps" / "dep-a").mkdir(parents=True)
        (root / "ai-specs" / ".deps" / "dep-a" / "marker").write_text("d\n")
        result = self._sync(root, home)
        dest = root / "ai-specs" / "recipes" / "demo" / "overrides"
        self.assertEqual(result.returncode, 0)
        self.assertTrue((dest / "config.toml").is_file())
        self.assertEqual((dest / "config.toml").read_text(), 'board_id = "x"\n')
        self.assertTrue((dest / "templates" / "card.md").is_file())
        self.assertFalse((root / "ai-specs" / ".recipe").exists())
        self.assertTrue((root / "ai-specs" / ".deps").exists())

    def test_remove_legacy_origin_keeps_existing_overrides(self):
        root, home = self._project()
        existing = root / "ai-specs" / "recipes" / "demo" / "overrides"
        existing.mkdir(parents=True)
        (existing / "config.toml").write_text("keep = true\n")
        legacy = root / "ai-specs" / ".recipe" / "demo" / "overrides"
        legacy.mkdir(parents=True)
        (legacy / "config.toml").write_text("stale = true\n")
        self._sync(root, home)
        self.assertEqual((existing / "config.toml").read_text(), "keep = true\n")
        self.assertFalse((root / "ai-specs" / ".recipe").exists())
        self.assertTrue(existing.is_dir())

    def test_remove_legacy_origin_skips_recipe_rmtree_if_override_migration_failed(self):
        root, home = self._project()
        legacy = root / "ai-specs" / ".recipe" / "demo" / "overrides"
        legacy.mkdir(parents=True)
        (legacy / "config.toml").write_text('board_id = "x"\n')
        recipes = root / "ai-specs" / "recipes"
        recipes.mkdir(parents=True)
        (recipes / "demo").write_text("blocking file\n")
        self._sync(root, home)
        self.assertTrue(legacy.exists())
        self.assertTrue((recipes / "demo").is_file())
        self.assertIn("blocking file", (recipes / "demo").read_text())

    def test_remove_legacy_origin_deletes_resolved_skills_and_internal(self):
        root, home = self._project()
        (root / "ai-specs" / ".resolved-skills" / "old").mkdir(parents=True)
        (root / "ai-specs" / ".internal" / "resolved-skills" / "old").mkdir(parents=True)
        (root / "ai-specs" / ".resolved-skills" / "old" / "SKILL.md").write_text("x\n")
        self._sync(root, home)
        self.assertFalse((root / "ai-specs" / ".resolved-skills").exists())
        self.assertFalse((root / "ai-specs" / ".internal").exists())
        self.assertFalse((root / "ai-specs" / ".resolved-skills" / "old").exists())

    def test_remove_legacy_origin_deletes_stale_premerge_bin(self):
        root, home = self._project()
        bin_dir = root / "ai-specs" / "bin"
        bin_dir.mkdir()
        (bin_dir / "premerge_guardian.py").write_text("stale\n")
        (bin_dir / "keep-me.sh").write_text("#!/bin/sh\n")
        self._sync(root, home)
        self.assertFalse((bin_dir / "premerge_guardian.py").exists())
        self.assertTrue((bin_dir / "keep-me.sh").is_file())
        self.assertTrue(bin_dir.is_dir())

    def test_remove_legacy_origin_removes_empty_bin_after_guardian(self):
        root, home = self._project()
        bin_dir = root / "ai-specs" / "bin"
        bin_dir.mkdir()
        (bin_dir / "premerge_guardian.py").write_text("stale\n")
        self._sync(root, home)
        self.assertFalse(bin_dir.exists())
        self.assertFalse((root / "ai-specs" / "bin" / "premerge_guardian.py").exists())


class BundledCommandPathTests(unittest.TestCase):
    def _project(self):
        td, root = temp_project(name="commands")
        self.addCleanup(td.cleanup)
        home_td = tempfile.TemporaryDirectory()
        self.addCleanup(home_td.cleanup)
        home = isolated_home(Path(home_td.name))
        result = invoke(root, "refresh-bundled", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        return root, home

    def _cache(self, root: Path, home: Path) -> Path:
        return cache_project_dir(root, home)

    def test_bundled_commands_root_is_bundled_skills_root_sibling(self):
        root, home = self._project()
        cache = self._cache(root, home)
        commands = cache / ".bundled" / "commands"
        skills = cache / ".bundled" / "skills"
        self.assertTrue(commands.is_dir())
        self.assertTrue(skills.is_dir())
        self.assertEqual(commands.parent, cache / ".bundled")
        self.assertNotEqual(commands, skills)
        self.assertTrue((commands / "rules-audit.md").is_file())

    def test_bundled_command_ids_returns_md_stems(self):
        root, home = self._project()
        commands = self._cache(root, home) / ".bundled" / "commands"
        names = sorted(path.stem for path in commands.glob("*.md"))
        self.assertIn("rules-audit", names)
        self.assertIn("skills-as-rules", names)
        self.assertEqual(names, sorted(names))
        self.assertNotIn("README", names)

    def test_bundled_command_ids_empty_when_dir_missing(self):
        root, home = self._project()
        commands = self._cache(root, home) / ".bundled" / "commands"
        for path in commands.glob("*.md"):
            path.unlink()
        self.assertEqual(list(commands.glob("*.md")), [])
        self.assertTrue(commands.is_dir())
        self.assertTrue(self._cache(root, home).is_dir())


if __name__ == "__main__":
    unittest.main()


