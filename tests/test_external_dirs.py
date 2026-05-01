import importlib.util
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
CLI = ROOT / "bin" / "ai-specs"
RECIPE_MATERIALIZE_PATH = ROOT / "lib" / "_internal" / "recipe-materialize.py"
VENDOR_SKILLS_PATH = ROOT / "lib" / "_internal" / "vendor-skills.py"
SKILL_RESOLUTION_PATH = ROOT / "lib" / "_internal" / "skill-resolution.py"
CATALOG = ROOT / "catalog" / "recipes"


def load_module(path: Path, name: str):
    # Ensure lib/_internal is on sys.path for sibling imports (skill_contract, etc.)
    internal_dir = str(path.parent)
    if internal_dir not in sys.path:
        sys.path.insert(0, internal_dir)
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


class InitExternalDirsTests(unittest.TestCase):
    def test_init_creates_recipe_and_deps_dirs(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            subprocess.run([str(CLI), "init", str(target)], check=True, text=True, capture_output=True)
            self.assertTrue((target / "ai-specs" / ".recipe").is_dir())
            self.assertTrue((target / "ai-specs" / ".deps").is_dir())

    def test_init_idempotent_for_existing_dirs(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            (target / "ai-specs" / ".recipe").mkdir(parents=True)
            (target / "ai-specs" / ".deps").mkdir(parents=True)
            (target / "ai-specs" / ".recipe" / "existing").write_text("keep")
            subprocess.run([str(CLI), "init", str(target)], check=True, text=True, capture_output=True)
            self.assertEqual((target / "ai-specs" / ".recipe" / "existing").read_text(), "keep")

    def test_gitignore_contains_external_dirs(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            subprocess.run([str(CLI), "init", str(target)], check=True, text=True, capture_output=True)
            gitignore = (target / ".gitignore").read_text()
            self.assertIn("ai-specs/.recipe/", gitignore)
            self.assertIn("ai-specs/.deps/", gitignore)
            self.assertIn("ai-specs/.recipe/*/overrides/", gitignore)

    def test_gitignore_idempotent_no_duplicates(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            subprocess.run([str(CLI), "init", str(target)], check=True, text=True, capture_output=True)
            subprocess.run([str(CLI), "init", str(target)], check=True, text=True, capture_output=True)
            gitignore = (target / ".gitignore").read_text()
            lines = [ln.strip() for ln in gitignore.splitlines()]
            self.assertEqual(lines.count("ai-specs/.recipe/"), 1)
            self.assertEqual(lines.count("ai-specs/.deps/"), 1)


class VendorSkillsPathTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(VENDOR_SKILLS_PATH, "vendor_skills_internal")

    def _make_dep_repo(self, tmp: Path, name: str) -> Path:
        repo = tmp / name
        repo.mkdir()
        (repo / "SKILL.md").write_text(
            "---\n"
            f"name: {name}\n"
            "description: Vendored skill.\n"
            "---\n\n"
            f"# {name}\n"
        )
        subprocess.run(["git", "init", "-q", str(repo)], check=True, text=True, capture_output=True)
        subprocess.run(["git", "-C", str(repo), "config", "user.name", "Fixture"], check=True, text=True, capture_output=True)
        subprocess.run(["git", "-C", str(repo), "config", "user.email", "f@example.com"], check=True, text=True, capture_output=True)
        subprocess.run(["git", "-C", str(repo), "add", "."], check=True, text=True, capture_output=True)
        subprocess.run(["git", "-C", str(repo), "commit", "-q", "-m", "init"], check=True, text=True, capture_output=True)
        return repo

    def test_vendor_writes_to_deps_dir(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            project = tmp_path / "project"
            project.mkdir()
            ai_specs = project / "ai-specs"
            ai_specs.mkdir()
            (ai_specs / "ai-specs.toml").write_text(
                "[project]\nname = 'fixture'\n\n"
                "[[deps]]\n"
                'id = "my-dep"\n'
                f'source = "{self._make_dep_repo(tmp_path, "my-dep")}"\n'
            )
            self.mod.sync_vendored_skills(project, self.mod.load_deps(project))
            skill = project / "ai-specs" / ".deps" / "my-dep" / "skills" / "my-dep" / "SKILL.md"
            self.assertTrue(skill.is_file())
            self.assertIn("name: my-dep", skill.read_text())

    def test_vendor_does_not_write_to_ai_specs_skills(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            project = tmp_path / "project"
            project.mkdir()
            ai_specs = project / "ai-specs"
            ai_specs.mkdir()
            (ai_specs / "ai-specs.toml").write_text(
                "[project]\nname = 'fixture'\n\n"
                "[[deps]]\n"
                'id = "my-dep"\n'
                f'source = "{self._make_dep_repo(tmp_path, "my-dep")}"\n'
            )
            self.mod.sync_vendored_skills(project, self.mod.load_deps(project))
            self.assertFalse((project / "ai-specs" / "skills" / "my-dep").exists())


class RecipeMaterializePathTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(RECIPE_MATERIALIZE_PATH, "recipe_materialize_internal")

    def _make_dep_repo(self, tmp: Path, name: str) -> Path:
        repo = tmp / name
        repo.mkdir()
        (repo / "SKILL.md").write_text(
            "---\n"
            f"name: {name}\n"
            "description: Recipe dep skill.\n"
            "---\n\n"
            f"# {name}\n"
        )
        subprocess.run(["git", "init", "-q", str(repo)], check=True, text=True, capture_output=True)
        subprocess.run(["git", "-C", str(repo), "config", "user.name", "Fixture"], check=True, text=True, capture_output=True)
        subprocess.run(["git", "-C", str(repo), "config", "user.email", "f@example.com"], check=True, text=True, capture_output=True)
        subprocess.run(["git", "-C", str(repo), "add", "."], check=True, text=True, capture_output=True)
        subprocess.run(["git", "-C", str(repo), "commit", "-q", "-m", "init"], check=True, text=True, capture_output=True)
        return repo

    def _make_project(self, recipe_section: str) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "skills").mkdir()
        (ai_specs / "commands").mkdir()
        manifest = ai_specs / "ai-specs.toml"
        manifest.write_text(
            "[project]\nname = 'fixture'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            + recipe_section
            + "\n"
        )
        return root

    def test_materializes_bundled_skill_to_recipe_dir(self):
        root = self._make_project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)
        skill_dir = root / "ai-specs" / ".recipe" / "test-fixture" / "skills" / "test-skill"
        self.assertTrue(skill_dir.is_dir())
        self.assertTrue((skill_dir / "SKILL.md").is_file())

    def test_materializes_command_to_ai_specs(self):
        root = self._make_project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)
        cmd = root / "ai-specs" / "commands" / "test-command.md"
        self.assertTrue(cmd.is_file())

    def test_warns_when_recipe_command_overwrites_existing_command(self):
        root = self._make_project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        cmd = root / "ai-specs" / "commands" / "test-command.md"
        cmd.write_text("# user command\n")
        import io
        captured = io.StringIO()
        old_stderr = sys.stderr
        sys.stderr = captured
        try:
            self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)
        finally:
            sys.stderr = old_stderr
        self.assertIn("overwrites existing command", captured.getvalue())
        self.assertNotEqual(cmd.read_text(), "# user command\n")

    def test_materializes_recipe_dep_skill_to_deps_dir(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            dep_repo = self._make_dep_repo(tmp_path, "dep-skill")
            home = tmp_path / "home"
            recipe_dir = home / "catalog" / "recipes" / "dep-fixture"
            recipe_dir.mkdir(parents=True)
            (recipe_dir / "recipe.toml").write_text(
                "[recipe]\n"
                'id = "dep-fixture"\n'
                'name = "Dep Fixture"\n'
                'description = "Recipe with dep skill."\n'
                'version = "1.0.0"\n\n'
                "[provides]\n"
                "skills = [\n"
                f'    {{ id = "dep-skill", source = "dep", url = "{dep_repo.as_posix()}" }},\n'
                "]\n"
            )
            root = self._make_project(
                '[recipes.dep-fixture]\nenabled = true\nversion = "1.0.0"\n'
            )
            self.assertEqual(self.mod.materialize_recipes(root, home), 0)
            dep_skill = root / "ai-specs" / ".deps" / "dep-skill" / "skills" / "dep-skill" / "SKILL.md"
            self.assertTrue(dep_skill.is_file())
            self.assertFalse((root / "ai-specs" / "skills" / "dep-skill").exists())

    def test_local_skills_untouched_by_materialization(self):
        root = self._make_project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        local_skill = root / "ai-specs" / "skills" / "local-only"
        local_skill.mkdir()
        (local_skill / "SKILL.md").write_text("local")
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)
        self.assertEqual((local_skill / "SKILL.md").read_text(), "local")


class SkillResolutionTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(SKILL_RESOLUTION_PATH, "skill_resolution_internal")

    def _make_project(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "skills").mkdir()
        (ai_specs / "commands").mkdir()
        (ai_specs / "ai-specs.toml").write_text("[project]\nname = 'fixture'\n")
        return root

    def _write_local_skill(self, root: Path, name: str) -> None:
        d = root / "ai-specs" / "skills" / name
        d.mkdir(parents=True)
        (d / "SKILL.md").write_text(f"# {name}")

    def _write_recipe_skill(self, root: Path, recipe: str, name: str) -> None:
        d = root / "ai-specs" / ".recipe" / recipe / "skills" / name
        d.mkdir(parents=True)
        (d / "SKILL.md").write_text(f"# {name}")

    def _write_dep_skill(self, root: Path, dep: str, name: str) -> None:
        d = root / "ai-specs" / ".deps" / dep / "skills" / name
        d.mkdir(parents=True)
        (d / "SKILL.md").write_text(f"# {name}")

    def test_local_precedence_over_recipe(self):
        root = self._make_project()
        self._write_local_skill(root, "shared")
        self._write_recipe_skill(root, "r1", "shared")
        resolved = self.mod.collect_skills(root)
        self.assertEqual(resolved["shared"][0], "local")

    def test_recipe_precedence_over_dep(self):
        root = self._make_project()
        self._write_recipe_skill(root, "r1", "shared")
        self._write_dep_skill(root, "d1", "shared")
        resolved = self.mod.collect_skills(root)
        self.assertEqual(resolved["shared"][0], "recipe")

    def test_local_precedence_over_all(self):
        root = self._make_project()
        self._write_local_skill(root, "shared")
        self._write_recipe_skill(root, "r1", "shared")
        self._write_dep_skill(root, "d1", "shared")
        resolved = self.mod.collect_skills(root)
        self.assertEqual(resolved["shared"][0], "local")

    def test_dep_fallback_when_no_other_source(self):
        root = self._make_project()
        self._write_dep_skill(root, "d1", "only-dep")
        resolved = self.mod.collect_skills(root)
        self.assertEqual(resolved["only-dep"][0], "dep")

    def test_first_seen_recipe_wins_with_warning(self):
        root = self._make_project()
        self._write_recipe_skill(root, "r1", "dup")
        self._write_recipe_skill(root, "r2", "dup")
        import io
        captured = io.StringIO()
        old_stderr = sys.stderr
        sys.stderr = captured
        try:
            resolved = self.mod.collect_skills(root)
        finally:
            sys.stderr = old_stderr
        self.assertEqual(resolved["dup"][0], "recipe")
        # Should warn about duplicate
        self.assertIn("dup", captured.getvalue())
        self.assertIn("r1", captured.getvalue())

    def test_first_seen_dep_wins_with_warning(self):
        root = self._make_project()
        self._write_dep_skill(root, "d1", "dup")
        self._write_dep_skill(root, "d2", "dup")
        import io
        captured = io.StringIO()
        old_stderr = sys.stderr
        sys.stderr = captured
        try:
            resolved = self.mod.collect_skills(root)
        finally:
            sys.stderr = old_stderr
        self.assertEqual(resolved["dup"][0], "dep")
        self.assertIn("dup", captured.getvalue())
        self.assertIn("d1", captured.getvalue())

    def test_missing_skill_raises(self):
        root = self._make_project()
        with self.assertRaises(RuntimeError) as ctx:
            self.mod.resolve_skill(root, "missing")
        self.assertIn("missing", str(ctx.exception))

    def test_local_override_silent_no_warning(self):
        root = self._make_project()
        self._write_local_skill(root, "shared")
        self._write_recipe_skill(root, "r1", "shared")
        import io
        captured = io.StringIO()
        old_stderr = sys.stderr
        sys.stderr = captured
        try:
            resolved = self.mod.collect_skills(root)
        finally:
            sys.stderr = old_stderr
        self.assertEqual(resolved["shared"][0], "local")
        # No warning should be emitted for local override
        self.assertEqual(captured.getvalue(), "")

    def test_local_precedence_does_not_backfill_files_from_recipe(self):
        root = self._make_project()
        self._write_local_skill(root, "shared")
        self._write_recipe_skill(root, "r1", "shared")
        recipe_asset = root / "ai-specs" / ".recipe" / "r1" / "skills" / "shared" / "assets" / "helper.md"
        recipe_asset.parent.mkdir(parents=True)
        recipe_asset.write_text("recipe asset")
        self.assertIsNone(self.mod.resolve_skill_template(root, "shared", "assets/helper.md"))


class OverrideLoadingTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(SKILL_RESOLUTION_PATH, "skill_resolution_internal")

    def _make_project(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "skills").mkdir()
        (ai_specs / "ai-specs.toml").write_text("[project]\nname = 'fixture'\n")
        return root

    def _write_recipe_skill(self, root: Path, recipe: str, name: str) -> None:
        d = root / "ai-specs" / ".recipe" / recipe / "skills" / name
        d.mkdir(parents=True)
        (d / "SKILL.md").write_text(f"# {name}")

    def test_override_config_merged(self):
        root = self._make_project()
        self._write_recipe_skill(root, "my-recipe", "my-skill")
        overrides = root / "ai-specs" / ".recipe" / "my-recipe" / "overrides" / "config.toml"
        overrides.parent.mkdir(parents=True)
        overrides.write_text('timeout = 99\n')
        cfg = self.mod.load_skill_config(root, "my-skill", {"timeout": 30})
        self.assertEqual(cfg["timeout"], 99)

    def test_override_config_missing_uses_defaults(self):
        root = self._make_project()
        self._write_recipe_skill(root, "my-recipe", "my-skill")
        cfg = self.mod.load_skill_config(root, "my-skill", {"timeout": 30})
        self.assertEqual(cfg["timeout"], 30)

    def test_override_config_isolated_between_recipes(self):
        root = self._make_project()
        self._write_recipe_skill(root, "recipe-a", "shared-skill")
        self._write_recipe_skill(root, "recipe-b", "shared-skill")
        overrides_a = root / "ai-specs" / ".recipe" / "recipe-a" / "overrides" / "config.toml"
        overrides_a.parent.mkdir(parents=True)
        overrides_a.write_text('timeout = 99\n')
        # For recipe-b skill, override from recipe-a should not apply
        # Since first-seen wins, recipe-a's skill is used
        cfg = self.mod.load_skill_config(root, "shared-skill", {"timeout": 30})
        self.assertEqual(cfg["timeout"], 99)
        # Now simulate using recipe-b's skill directly (not via resolution)
        # The helper uses resolved path, so it follows first-seen
        # To test isolation, we'll create a distinct skill in recipe-b
        self._write_recipe_skill(root, "recipe-b", "other-skill")
        cfg_b = self.mod.load_skill_config(root, "other-skill", {"timeout": 30})
        self.assertEqual(cfg_b["timeout"], 30)

    def test_override_template_preferred(self):
        root = self._make_project()
        self._write_recipe_skill(root, "my-recipe", "my-skill")
        bundled_tpl = root / "ai-specs" / ".recipe" / "my-recipe" / "skills" / "my-skill" / "template.md"
        bundled_tpl.write_text("bundled")
        override_tpl = root / "ai-specs" / ".recipe" / "my-recipe" / "overrides" / "templates" / "template.md"
        override_tpl.parent.mkdir(parents=True)
        override_tpl.write_text("override")
        resolved = self.mod.resolve_skill_template(root, "my-skill", "template.md")
        self.assertEqual(resolved.read_text(), "override")

    def test_override_template_fallback_to_bundled(self):
        root = self._make_project()
        self._write_recipe_skill(root, "my-recipe", "my-skill")
        bundled_tpl = root / "ai-specs" / ".recipe" / "my-recipe" / "skills" / "my-skill" / "template.md"
        bundled_tpl.write_text("bundled")
        resolved = self.mod.resolve_skill_template(root, "my-skill", "template.md")
        self.assertEqual(resolved.read_text(), "bundled")

    def test_override_template_missing_returns_none(self):
        root = self._make_project()
        self._write_recipe_skill(root, "my-recipe", "my-skill")
        resolved = self.mod.resolve_skill_template(root, "my-skill", "nonexistent.md")
        self.assertIsNone(resolved)


class OrphanCleanupTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(RECIPE_MATERIALIZE_PATH, "recipe_materialize_internal")

    def _make_project(self, recipe_section: str = "", deps_section: str = "") -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "skills").mkdir()
        (ai_specs / "commands").mkdir()
        manifest = ai_specs / "ai-specs.toml"
        manifest.write_text(
            "[project]\nname = 'fixture'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            + deps_section
            + recipe_section
            + "\n"
        )
        return root

    def test_orphan_recipe_directory_removed(self):
        root = self._make_project()
        orphan = root / "ai-specs" / ".recipe" / "old-recipe"
        orphan.mkdir(parents=True)
        (orphan / "keep.txt").write_text("stale")
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)
        self.assertFalse(orphan.exists())

    def test_orphan_dep_directory_removed(self):
        root = self._make_project()
        orphan = root / "ai-specs" / ".deps" / "old-dep"
        orphan.mkdir(parents=True)
        (orphan / "keep.txt").write_text("stale")
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)
        self.assertFalse(orphan.exists())

    def test_referenced_recipe_preserved(self):
        root = self._make_project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        recipe_dir = root / "ai-specs" / ".recipe" / "test-fixture"
        recipe_dir.mkdir(parents=True)
        (recipe_dir / "keep.txt").write_text("keep")
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)
        self.assertTrue((recipe_dir / "keep.txt").exists())


class ResyncIdempotencyTests(unittest.TestCase):
    def test_sync_is_idempotent(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            subprocess.run([str(CLI), "init", str(target)], check=True, text=True, capture_output=True)
            subprocess.run([str(CLI), "sync", str(target)], check=True, text=True, capture_output=True)
            first = self._hash_tree(target)
            subprocess.run([str(CLI), "sync", str(target)], check=True, text=True, capture_output=True)
            second = self._hash_tree(target)
            self.assertEqual(first, second)

    def _hash_tree(self, root: Path) -> str:
        import hashlib
        hashes = []
        for p in sorted(root.rglob("*")):
            if p.is_file() and ".git" not in str(p):
                hashes.append(f"{p.relative_to(root)}:{hashlib.sha1(p.read_bytes()).hexdigest()}")
        return "\n".join(hashes)


if __name__ == "__main__":
    unittest.main()
