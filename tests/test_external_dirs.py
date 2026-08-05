import importlib.util
import io
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "tests"))
from _fixture_catalog import allow_internal_test_recipes_env, populate_catalog  # noqa: E402

CLI = ROOT / "bin" / "ai-specs"
REFRESH_BUNDLED_PATH = ROOT / "lib" / "_internal" / "refresh-bundled.py"
RECIPE_MATERIALIZE_PATH = ROOT / "lib" / "_internal" / "recipe-materialize.py"
VENDOR_SKILLS_PATH = ROOT / "lib" / "_internal" / "vendor-skills.py"
SKILL_RESOLUTION_PATH = ROOT / "lib" / "_internal" / "skill-resolution.py"
CATALOG = ROOT / "catalog" / "recipes"
_FIXTURE_HOME: Path | None = None


def _fixture_home() -> Path:
    global _FIXTURE_HOME
    if _FIXTURE_HOME is None:
        _FIXTURE_HOME = Path(tempfile.mkdtemp(prefix="ai-specs-ext-fixture-home-"))
        populate_catalog(_FIXTURE_HOME / "catalog" / "recipes")
    return _FIXTURE_HOME


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

def _pc():
    return load_module(ROOT / "lib" / "_internal" / "project-cache.py", "project_cache_ext")


def cache_recipe_skill(project_root, recipe_id, skill_id, cli_home=None):
    home = _fixture_home() if cli_home is None else cli_home
    return _pc().recipe_skills_root(project_root, cli_home=home) / recipe_id / "skills" / skill_id


def cache_dep_skill(project_root, dep_id, skill_id=None, cli_home=None):
    home = ROOT if cli_home is None else cli_home
    sid = dep_id if skill_id is None else skill_id
    return _pc().deps_skills_root(project_root, cli_home=home) / dep_id / "skills" / sid


def cache_command(project_root, cmd_id, cli_home=None):
    home = _fixture_home() if cli_home is None else cli_home
    return _pc().commands_dir(project_root, cli_home=home) / f"{cmd_id}.md"


def cache_bundled_skill(project_root, skill_id, cli_home=None):
    home = ROOT if cli_home is None else cli_home
    return _pc().bundled_skills_root(project_root, cli_home=home) / "skills" / skill_id


def inproject_dep_skill(project_root, dep_id, skill_id=None):
    sid = dep_id if skill_id is None else skill_id
    return _pc().inproject_deps_root(project_root) / dep_id / "skills" / sid


class InitExternalDirsTests(unittest.TestCase):
    def test_init_does_not_create_in_project_origin_dirs(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            subprocess.run([str(CLI), "init", "--no-tui", str(target)], check=True, text=True, capture_output=True)
            self.assertFalse((target / "ai-specs" / ".recipe").exists())
            self.assertFalse((target / "ai-specs" / ".deps").exists())

    def test_init_idempotent_without_origin_dirs(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            subprocess.run([str(CLI), "init", "--no-tui", str(target)], check=True, text=True, capture_output=True)
            subprocess.run([str(CLI), "init", "--no-tui", str(target)], check=True, text=True, capture_output=True)
            self.assertTrue((target / "ai-specs" / "ai-specs.toml").is_file())
            self.assertFalse((target / "ai-specs" / ".recipe").exists())

    def test_gitignore_omits_in_project_origin_dirs(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            subprocess.run([str(CLI), "init", "--no-tui", str(target)], check=True, text=True, capture_output=True)
            gitignore = (target / ".gitignore").read_text()
            self.assertNotIn("ai-specs/.recipe/", gitignore)
            self.assertNotIn("ai-specs/.deps/", gitignore)

    def test_gitignore_ignores_recipes_except_overrides(self):
        """recipes/ is CLI-owned; only declared overrides are committed."""
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            subprocess.run(["git", "init", "-q", str(target)], check=True, text=True, capture_output=True)
            subprocess.run([str(CLI), "init", "--no-tui", str(target)], check=True, text=True, capture_output=True)
            ai_specs = target / "ai-specs"
            # A bundled recipe doc should be ignored; a declared override committed.
            (ai_specs / "recipes" / "demo").mkdir(parents=True)
            (ai_specs / "recipes" / "demo" / "README.md").write_text("bundled doc\n")
            (ai_specs / "recipes" / "demo" / "overrides").mkdir()
            (ai_specs / "recipes" / "demo" / "overrides" / "config.toml").write_text("x = 1\n")
            (ai_specs / ".deps" / "mydep").mkdir(parents=True)
            (ai_specs / ".deps" / "mydep" / "SKILL.md").write_text("# mydep\n")

            def ignored(rel: str) -> bool:
                r = subprocess.run(
                    ["git", "check-ignore", "-q", rel],
                    cwd=target, capture_output=True,
                )
                return r.returncode == 0

            self.assertTrue(ignored("ai-specs/recipes/demo/README.md"))
            self.assertFalse(ignored("ai-specs/recipes/demo/overrides/config.toml"))
            self.assertTrue(ignored("ai-specs/.deps/mydep/SKILL.md"))

    def test_gitignore_idempotent_no_origin_entries(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            subprocess.run([str(CLI), "init", "--no-tui", str(target)], check=True, text=True, capture_output=True)
            subprocess.run([str(CLI), "init", "--no-tui", str(target)], check=True, text=True, capture_output=True)
            gitignore = (target / ".gitignore").read_text()
            lines = [ln.strip() for ln in gitignore.splitlines()]
            self.assertEqual(lines.count("ai-specs/.recipe/"), 0)
            self.assertEqual(lines.count("ai-specs/.deps/"), 0)

    def test_gitignore_committable_relocated_recipe_templates(self):
        """Relocated conditional templates/bin under overrides/ are committable."""
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            subprocess.run(["git", "init", "-q", str(target)], check=True, text=True, capture_output=True)
            subprocess.run([str(CLI), "init", "--no-tui", str(target)], check=True, text=True, capture_output=True)
            ai_specs = target / "ai-specs"

            trello_ovr = (
                ai_specs / "recipes" / "trello-mcp-workflow" / "overrides" / "templates"
            )
            trello_ovr.mkdir(parents=True)
            (trello_ovr / "card-feature.md").write_text("# feature\n")

            wt_ovr = ai_specs / "recipes" / "worktree-flow" / "overrides" / "bin"
            wt_ovr.mkdir(parents=True)
            (wt_ovr / "worktree-cleanup.sh").write_text("#!/bin/sh\n")

            trello_bare = ai_specs / "recipes" / "trello-mcp-workflow" / "templates"
            trello_bare.mkdir(parents=True)
            (trello_bare / "card-feature.md").write_text("# bare\n")

            wt_bare = ai_specs / "recipes" / "worktree-flow" / "bin"
            wt_bare.mkdir(parents=True)
            (wt_bare / "worktree-cleanup.sh").write_text("#!/bin/sh\n")

            def ignored(rel: str) -> bool:
                r = subprocess.run(
                    ["git", "check-ignore", "-q", rel],
                    cwd=target, capture_output=True,
                )
                return r.returncode == 0

            self.assertFalse(
                ignored(
                    "ai-specs/recipes/trello-mcp-workflow/overrides/templates/card-feature.md"
                )
            )
            self.assertFalse(
                ignored(
                    "ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh"
                )
            )
            self.assertTrue(
                ignored(
                    "ai-specs/recipes/trello-mcp-workflow/templates/card-feature.md"
                )
            )
            self.assertTrue(
                ignored("ai-specs/recipes/worktree-flow/bin/worktree-cleanup.sh")
            )


class CatalogConditionalTemplateTargetLintTests(unittest.TestCase):
    """Catalog-only guard: not_exists templates under ai-specs/recipes/ use overrides/."""

    EXPECTED_TARGETS = {
        "ai-specs/recipes/trello-mcp-workflow/overrides/templates/card-feature.md",
        "ai-specs/recipes/trello-mcp-workflow/overrides/templates/card-bug.md",
        "ai-specs/recipes/trello-mcp-workflow/overrides/templates/card-spike.md",
        "ai-specs/recipes/trello-mcp-workflow/overrides/templates/card-epic.md",
        "ai-specs/recipes/trello-mcp-workflow/overrides/templates/card-handoff.md",
        "ai-specs/recipes/trello-mcp-workflow/overrides/templates/card-decision.md",
        "ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh",
    }

    def test_not_exists_recipe_template_targets_use_overrides(self):
        import re

        found = []
        for recipe_toml in sorted(CATALOG.glob("*/recipe.toml")):
            recipe_id = recipe_toml.parent.name
            if recipe_id == "test-fixture" or recipe_id.startswith("test-"):
                continue
            text = recipe_toml.read_text()
            parts = re.split(r"\n\[\[provides\.templates\]\]\n", text)
            for part in parts[1:]:
                block = re.split(r"\n\[\[", part, maxsplit=1)[0]
                cond_m = re.search(r'(?m)^condition\s*=\s*"([^"]+)"\s*$', block)
                tgt_m = re.search(r'(?m)^target\s*=\s*"([^"]+)"\s*$', block)
                if not tgt_m:
                    continue
                target = tgt_m.group(1)
                condition = cond_m.group(1) if cond_m else None
                if condition != "not_exists":
                    continue
                if not target.startswith("ai-specs/recipes/"):
                    continue
                found.append(target)
                self.assertIn(
                    "/overrides/",
                    target,
                    f"{recipe_id}: not_exists target must nest under overrides/: {target}",
                )

        self.assertEqual(set(found), self.EXPECTED_TARGETS)


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
            # toml-deps ([[deps]]) are project-governed → in-project ai-specs/.deps/
            skill = inproject_dep_skill(project, "my-dep") / "SKILL.md"
            self.assertTrue(skill.is_file())
            self.assertIn("name: my-dep", skill.read_text())
            # and NOT staged under the CLI cache
            self.assertFalse((cache_dep_skill(project, "my-dep") / "SKILL.md").is_file())

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

    def setUp(self):
        self._allow = mock.patch.dict(os.environ, allow_internal_test_recipes_env())
        self._allow.start()
        self.addCleanup(self._allow.stop)

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
        home = _fixture_home()
        self.assertEqual(self.mod.materialize_recipes(root, home), 0)
        skill_dir = cache_recipe_skill(root, "test-fixture", "test-skill", cli_home=home)
        self.assertTrue(skill_dir.is_dir())
        self.assertTrue((skill_dir / "SKILL.md").is_file())

    def test_materializes_command_to_cache(self):
        root = self._make_project(
            "[recipes.test-fixture]\nenabled = true\n"
        )
        home = _fixture_home()
        self.assertEqual(self.mod.materialize_recipes(root, home), 0)
        cmd = cache_command(root, "test-command", cli_home=home)
        self.assertTrue(cmd.is_file())

    def test_warns_when_recipe_command_overwrites_existing_managed_command(self):
        root = self._make_project(
            "[recipes.test-fixture]\nenabled = true\n"
        )
        home = _fixture_home()
        cmd = cache_command(root, "test-command", cli_home=home)
        cmd.parent.mkdir(parents=True, exist_ok=True)
        cmd.write_text("# previous managed\n")
        import io
        captured = io.StringIO()
        old_stderr = sys.stderr
        sys.stderr = captured
        try:
            self.assertEqual(self.mod.materialize_recipes(root, home), 0)
        finally:
            sys.stderr = old_stderr
        self.assertIn("overwrites existing managed command", captured.getvalue())
        self.assertNotEqual(cmd.read_text(), "# previous managed\n")

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
            dep_skill = cache_dep_skill(root, "dep-skill", cli_home=home) / "SKILL.md"
            self.assertTrue(dep_skill.is_file())
            self.assertFalse((root / "ai-specs" / "skills" / "dep-skill").exists())

    def test_local_skills_untouched_by_materialization(self):
        root = self._make_project(
            '[recipes.test-fixture]\nenabled = true\nversion = "1.0.0"\n'
        )
        home = _fixture_home()
        local_skill = root / "ai-specs" / "skills" / "local-only"
        local_skill.mkdir()
        (local_skill / "SKILL.md").write_text("local")
        self.assertEqual(self.mod.materialize_recipes(root, home), 0)
        self.assertEqual((local_skill / "SKILL.md").read_text(), "local")


class BundledLeftoverCleanupTests(unittest.TestCase):
    """remove_legacy_origin deletes materialized bundled-skill copies from the
    project surface, but never genuine local skills or customized copies."""

    def _project(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        (root / "ai-specs" / "skills").mkdir(parents=True)
        return root

    def test_removes_bundled_leftover_keeps_local_and_customized(self):
        root = self._project()
        skills = root / "ai-specs" / "skills"
        bundled_src = ROOT / "bundled-skills"

        # 1. Materialized bundled copy (byte-identical to CLI source) → remove.
        leftover = skills / "harness-lifecycle"
        leftover.mkdir()
        (leftover / "SKILL.md").write_text(
            (bundled_src / "harness-lifecycle" / "SKILL.md").read_text()
        )

        # 2. Genuine local skill (no bundled counterpart) → keep.
        local = skills / "my-local-skill"
        local.mkdir()
        (local / "SKILL.md").write_text("# my-local-skill\n")

        # 3. Customized copy of a bundled skill (content differs) → keep + warn.
        customized = skills / "skill-creator"
        customized.mkdir()
        (customized / "SKILL.md").write_text("# skill-creator (locally edited)\n")

        _pc().remove_legacy_origin(root, cli_home=ROOT)

        self.assertFalse(leftover.exists(), "bundled leftover should be removed")
        self.assertTrue(local.exists(), "genuine local skill must be preserved")
        self.assertTrue(customized.exists(), "customized copy must be preserved")

    def test_removes_untouched_old_version_copy_via_lock_hash(self):
        """Migration: a copy from an older CLI (differs from current source) but
        recorded untouched in the legacy lock is safe to remove."""
        import hashlib
        root = self._project()
        old = root / "ai-specs" / "skills" / "skill-creator"
        old.mkdir()
        old_content = "# skill-creator (older CLI version, untouched)\n"
        (old / "SKILL.md").write_text(old_content)
        h = hashlib.sha256(old_content.encode()).hexdigest()
        (root / "ai-specs" / ".ai-specs.lock").write_text(
            f'[skills."skill-creator"]\n"SKILL.md" = "{h}"\n'
        )
        _pc().remove_legacy_origin(root, cli_home=ROOT)
        self.assertFalse(old.exists(), "untouched managed copy should be removed via lock hash")

    def test_keeps_edited_copy_not_matching_source_or_lock(self):
        root = self._project()
        edited = root / "ai-specs" / "skills" / "skill-creator"
        edited.mkdir()
        (edited / "SKILL.md").write_text("# genuinely edited by the user\n")
        (root / "ai-specs" / ".ai-specs.lock").write_text(
            '[skills."skill-creator"]\n"SKILL.md" = "0000000000000000"\n'
        )
        _pc().remove_legacy_origin(root, cli_home=ROOT)
        self.assertTrue(edited.exists(), "user-edited copy must be preserved")


class BundledCommandLeftoverCleanupTests(unittest.TestCase):
    """remove_bundled_command_leftovers deletes materialized bundled-command
    copies from the project surface, but never genuine local commands or
    customized copies."""

    def _project(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        (root / "ai-specs" / "commands").mkdir(parents=True)
        return root

    def test_removes_bundled_leftover_keeps_local_and_customized(self):
        root = self._project()
        commands = root / "ai-specs" / "commands"
        bundled_src = ROOT / "bundled-commands"

        # 1. Materialized bundled copy (byte-identical to CLI source) → remove.
        leftover = commands / "rules-audit.md"
        leftover.write_text((bundled_src / "rules-audit.md").read_text())

        # 2. Genuine local command (no bundled counterpart) → keep.
        local = commands / "my-local-command.md"
        local.write_text("# my-local-command\n")

        # 3. Customized copy of a bundled command (content differs) → keep + warn.
        customized = commands / "skills-as-rules.md"
        customized.write_text("# skills-as-rules (locally edited)\n")

        _pc().remove_bundled_command_leftovers(root / "ai-specs", ROOT)

        self.assertFalse(leftover.exists(), "bundled leftover should be removed")
        self.assertTrue(local.exists(), "genuine local command must be preserved")
        self.assertTrue(customized.exists(), "customized copy must be preserved")

    def test_removes_untouched_old_version_copy_via_lock_hash(self):
        """Migration: a copy from an older CLI (differs from current source) but
        recorded untouched in the legacy lock is safe to remove."""
        import hashlib
        root = self._project()
        old = root / "ai-specs" / "commands" / "rules-audit.md"
        old_content = "# rules-audit (older CLI version, untouched)\n"
        old.write_text(old_content)
        h = hashlib.sha256(old_content.encode()).hexdigest()
        _pc().remove_bundled_command_leftovers(
            root / "ai-specs", ROOT, lock_commands={"rules-audit.md": h}
        )
        self.assertFalse(old.exists(), "untouched managed copy should be removed via lock hash")

    def test_keeps_edited_copy_not_matching_source_or_lock(self):
        root = self._project()
        edited = root / "ai-specs" / "commands" / "rules-audit.md"
        edited.write_text("# genuinely edited by the user\n")
        _pc().remove_bundled_command_leftovers(
            root / "ai-specs", ROOT, lock_commands={"rules-audit.md": "0000000000000000"}
        )
        self.assertTrue(edited.exists(), "user-edited copy must be preserved")

    def test_no_bundled_counterpart_is_untouched(self):
        root = self._project()
        only_local = root / "ai-specs" / "commands" / "totally-local.md"
        only_local.write_text("# no bundled counterpart\n")
        _pc().remove_bundled_command_leftovers(root / "ai-specs", ROOT)
        self.assertTrue(only_local.exists())


class RecipeCommandLeftoverCleanupTests(unittest.TestCase):
    """Recipe-managed command copies migrate out of ai-specs/commands safely."""

    def _project(self) -> tuple[Path, Path, Path]:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name) / "project"
        (root / "ai-specs" / "commands").mkdir(parents=True)
        home = Path(tmp.name) / "home"
        home.mkdir()
        managed = _pc().commands_dir(root, cli_home=home)
        managed.mkdir(parents=True)
        return root, home, managed

    def test_removes_untouched_recipe_copy_and_merge_stays_silent(self):
        root, home, managed = self._project()
        content = "# recipe command\n"
        (managed / "pr-create.md").write_text(content)
        local = root / "ai-specs" / "commands" / "pr-create.md"
        local.write_text(content)

        _pc().remove_recipe_command_leftovers(root, cli_home=home)
        self.assertFalse(local.exists())

        dest = Path(tempfile.mkdtemp())
        self.addCleanup(lambda: __import__("shutil").rmtree(dest, ignore_errors=True))
        captured = io.StringIO()
        old = sys.stderr
        sys.stderr = captured
        try:
            self.assertEqual(_pc().merge_commands(root, dest, cli_home=home), 1)
        finally:
            sys.stderr = old
        self.assertEqual(captured.getvalue(), "")

    def test_preserves_customized_recipe_copy_with_local_warning(self):
        root, home, managed = self._project()
        (managed / "pr-create.md").write_text("# recipe command\n")
        local = root / "ai-specs" / "commands" / "pr-create.md"
        local.write_text("# customized locally\n")

        captured = io.StringIO()
        old = sys.stderr
        sys.stderr = captured
        try:
            _pc().remove_recipe_command_leftovers(root, cli_home=home)
        finally:
            sys.stderr = old

        self.assertTrue(local.exists())
        self.assertIn("local/customized", captured.getvalue())

    def test_refresh_migrates_cached_recipe_copy(self):
        root = Path(tempfile.mkdtemp()) / "project"
        self.addCleanup(lambda: shutil.rmtree(root.parent, ignore_errors=True))
        (root / "ai-specs" / "commands").mkdir(parents=True)
        (root / "ai-specs" / "ai-specs.toml").write_text(
            '[project]\nname = "refresh-recipe-leftover"\n'
        )
        managed = _pc().commands_dir(root, cli_home=ROOT)
        managed.mkdir(parents=True)
        content = "# recipe-managed command\n"
        (managed / "pr-create.md").write_text(content)
        local = root / "ai-specs" / "commands" / "pr-create.md"
        local.write_text(content)

        result = subprocess.run(
            [sys.executable, str(REFRESH_BUNDLED_PATH), str(root), str(ROOT)],
            check=False,
            capture_output=True,
            text=True,
        )
        self.assertEqual(result.returncode, 0, result.stderr or result.stdout)
        self.assertFalse(local.exists())
        self.assertNotIn("local hand-authored wins", result.stdout + result.stderr)

    def test_sync_migrates_first_recipe_copy_before_cache_exists(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name) / "project"
        root.mkdir()
        init = subprocess.run(
            [str(CLI), "init", str(root)], check=False, capture_output=True, text=True
        )
        self.assertEqual(init.returncode, 0, init.stderr or init.stdout)
        (root / "ai-specs" / "ai-specs.toml").write_text(
            '[project]\nname = "sync-recipe-leftover"\n\n'
            '[agents]\nenabled = ["cursor"]\n\n'
            '[recipes.tdd-flow]\nenabled = true\n\n'
            '[recipes.tdd-flow.config]\ntest_command = "python3 -m unittest"\n'
        )
        local = root / "ai-specs" / "commands" / "tdd.md"
        content = (ROOT / "catalog" / "recipes" / "tdd-flow" / "commands" / "tdd.md").read_text()
        local.write_text(content)
        managed = _pc().commands_dir(root, cli_home=ROOT)
        self.assertFalse((managed / "tdd.md").exists())

        result = subprocess.run(
            [str(CLI), "sync", str(root)], check=False, capture_output=True, text=True
        )
        self.assertEqual(result.returncode, 0, result.stderr or result.stdout)
        self.assertFalse(local.exists())
        self.assertNotIn("local hand-authored wins", result.stdout + result.stderr)
        self.assertEqual((managed / "tdd.md").read_text(), content)

    def test_removes_untouched_recipe_copy_via_legacy_lock_hash(self):
        import hashlib

        root, home, managed = self._project()
        local = root / "ai-specs" / "commands" / "pr-create.md"
        content = "# older recipe command\n"
        local.write_text(content)
        managed.rmdir()
        digest = hashlib.sha256(content.encode()).hexdigest()
        (root / "ai-specs" / ".ai-specs.lock").write_text(
            f'[commands]\n"pr-create.md" = "{digest}"\n'
        )

        _pc().remove_recipe_command_leftovers(root, cli_home=home)
        self.assertFalse(local.exists())


class TrackedBundledCommandLeftoverTests(unittest.TestCase):
    """tracked_bundled_command_leftovers finds git-tracked bundled-command
    copies whose working-tree file is gone; never mutates the index."""

    def _git_project(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        subprocess.run(["git", "init", "-q", str(root)], check=True)
        subprocess.run(
            ["git", "-C", str(root), "config", "user.email", "t@example.com"], check=True
        )
        subprocess.run(["git", "-C", str(root), "config", "user.name", "t"], check=True)
        return root

    def test_finds_tracked_command_with_missing_working_tree_copy(self):
        root = self._git_project()
        commands = root / "ai-specs" / "commands"
        commands.mkdir(parents=True)
        (commands / "rules-audit.md").write_text("# leftover\n")
        subprocess.run(["git", "-C", str(root), "add", "-A"], check=True)
        subprocess.run(["git", "-C", str(root), "commit", "-qm", "track"], check=True)
        (commands / "rules-audit.md").unlink()

        ids = _pc().tracked_bundled_command_leftovers(root, cli_home=ROOT)
        self.assertIn("rules-audit", ids)

    def test_empty_when_working_tree_copy_exists(self):
        root = self._git_project()
        commands = root / "ai-specs" / "commands"
        commands.mkdir(parents=True)
        (commands / "rules-audit.md").write_text("# still here\n")
        subprocess.run(["git", "-C", str(root), "add", "-A"], check=True)
        subprocess.run(["git", "-C", str(root), "commit", "-qm", "track"], check=True)

        ids = _pc().tracked_bundled_command_leftovers(root, cli_home=ROOT)
        self.assertEqual(ids, [])

    def test_empty_when_not_a_git_work_tree(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        (root / "ai-specs" / "commands").mkdir(parents=True)
        ids = _pc().tracked_bundled_command_leftovers(root, cli_home=ROOT)
        self.assertEqual(ids, [])


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
        d = cache_recipe_skill(root, recipe, name, cli_home=ROOT)
        d.mkdir(parents=True)
        (d / "SKILL.md").write_text(f"# {name}")

    def _write_dep_skill(self, root: Path, dep: str, name: str) -> None:
        d = cache_dep_skill(root, dep, skill_id=name, cli_home=ROOT)
        d.mkdir(parents=True)
        (d / "SKILL.md").write_text(f"# {name}")

    def _write_bundled_skill(self, root: Path, name: str) -> None:
        d = cache_bundled_skill(root, name, cli_home=ROOT)
        d.mkdir(parents=True)
        (d / "SKILL.md").write_text(f"# {name}")

    def test_bundled_fallback_when_no_other_source(self):
        root = self._make_project()
        self._write_bundled_skill(root, "harness-lifecycle")
        resolved = self.mod.collect_skills(root, cli_home=ROOT)
        self.assertEqual(resolved["harness-lifecycle"][0], "bundled")

    def test_dep_precedence_over_bundled(self):
        root = self._make_project()
        self._write_dep_skill(root, "d1", "shared")
        self._write_bundled_skill(root, "shared")
        resolved = self.mod.collect_skills(root, cli_home=ROOT)
        self.assertEqual(resolved["shared"][0], "dep")

    def test_local_precedence_over_bundled(self):
        root = self._make_project()
        self._write_local_skill(root, "skill-creator")
        self._write_bundled_skill(root, "skill-creator")
        resolved = self.mod.collect_skills(root, cli_home=ROOT)
        self.assertEqual(resolved["skill-creator"][0], "local")

    def test_local_precedence_over_recipe(self):
        root = self._make_project()
        self._write_local_skill(root, "shared")
        self._write_recipe_skill(root, "r1", "shared")
        resolved = self.mod.collect_skills(root, cli_home=ROOT)
        self.assertEqual(resolved["shared"][0], "local")

    def test_recipe_precedence_over_dep(self):
        root = self._make_project()
        self._write_recipe_skill(root, "r1", "shared")
        self._write_dep_skill(root, "d1", "shared")
        resolved = self.mod.collect_skills(root, cli_home=ROOT)
        self.assertEqual(resolved["shared"][0], "recipe")

    def test_local_precedence_over_all(self):
        root = self._make_project()
        self._write_local_skill(root, "shared")
        self._write_recipe_skill(root, "r1", "shared")
        self._write_dep_skill(root, "d1", "shared")
        resolved = self.mod.collect_skills(root, cli_home=ROOT)
        self.assertEqual(resolved["shared"][0], "local")

    def test_dep_fallback_when_no_other_source(self):
        root = self._make_project()
        self._write_dep_skill(root, "d1", "only-dep")
        resolved = self.mod.collect_skills(root, cli_home=ROOT)
        self.assertEqual(resolved["only-dep"][0], "dep")

    def test_inproject_toml_dep_resolves_as_dep(self):
        root = self._make_project()
        d = inproject_dep_skill(root, "d1", "only-toml-dep")
        d.mkdir(parents=True)
        (d / "SKILL.md").write_text("# only-toml-dep")
        resolved = self.mod.collect_skills(root, cli_home=ROOT)
        self.assertEqual(resolved["only-toml-dep"][0], "dep")

    def test_first_seen_recipe_wins_with_warning(self):
        root = self._make_project()
        self._write_recipe_skill(root, "r1", "dup")
        self._write_recipe_skill(root, "r2", "dup")
        import io
        captured = io.StringIO()
        old_stderr = sys.stderr
        sys.stderr = captured
        try:
            resolved = self.mod.collect_skills(root, cli_home=ROOT)
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
            resolved = self.mod.collect_skills(root, cli_home=ROOT)
        finally:
            sys.stderr = old_stderr
        self.assertEqual(resolved["dup"][0], "dep")
        self.assertIn("dup", captured.getvalue())
        self.assertIn("d1", captured.getvalue())

    def test_missing_skill_raises(self):
        root = self._make_project()
        with self.assertRaises(RuntimeError) as ctx:
            self.mod.resolve_skill(root, "missing", cli_home=ROOT)
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
            resolved = self.mod.collect_skills(root, cli_home=ROOT)
        finally:
            sys.stderr = old_stderr
        self.assertEqual(resolved["shared"][0], "local")
        # No warning should be emitted for local override
        self.assertEqual(captured.getvalue(), "")

    def test_local_precedence_does_not_backfill_files_from_recipe(self):
        root = self._make_project()
        self._write_local_skill(root, "shared")
        self._write_recipe_skill(root, "r1", "shared")
        recipe_asset = cache_recipe_skill(root, "r1", "shared", cli_home=ROOT) / "assets" / "helper.md"
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
        d = cache_recipe_skill(root, recipe, name, cli_home=ROOT)
        d.mkdir(parents=True)
        (d / "SKILL.md").write_text(f"# {name}")

    def test_override_config_merged(self):
        root = self._make_project()
        self._write_recipe_skill(root, "my-recipe", "my-skill")
        overrides = root / "ai-specs" / "recipes" / "my-recipe" / "overrides" / "config.toml"
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
        overrides_a = root / "ai-specs" / "recipes" / "recipe-a" / "overrides" / "config.toml"
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
        bundled_tpl = cache_recipe_skill(root, "my-recipe", "my-skill", cli_home=ROOT) / "template.md"
        bundled_tpl.write_text("bundled")
        override_tpl = root / "ai-specs" / "recipes" / "my-recipe" / "overrides" / "templates" / "template.md"
        override_tpl.parent.mkdir(parents=True)
        override_tpl.write_text("override")
        resolved = self.mod.resolve_skill_template(root, "my-skill", "template.md")
        self.assertEqual(resolved.read_text(), "override")

    def test_override_template_fallback_to_bundled(self):
        root = self._make_project()
        self._write_recipe_skill(root, "my-recipe", "my-skill")
        bundled_tpl = cache_recipe_skill(root, "my-recipe", "my-skill", cli_home=ROOT) / "template.md"
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

    def setUp(self):
        self._allow = mock.patch.dict(os.environ, allow_internal_test_recipes_env())
        self._allow.start()
        self.addCleanup(self._allow.stop)

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
        home = _fixture_home()
        recipe_dir = _pc().recipe_skills_root(root, cli_home=home) / "test-fixture"
        recipe_dir.mkdir(parents=True)
        (recipe_dir / "keep.txt").write_text("keep")
        self.assertEqual(self.mod.materialize_recipes(root, home), 0)
        # keep.txt is wiped when skill materialize replaces the skill tree, but recipe dir remains
        self.assertTrue(
            cache_recipe_skill(root, "test-fixture", "test-skill", cli_home=home).is_dir()
        )


class ResyncIdempotencyTests(unittest.TestCase):
    def test_sync_is_idempotent(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / "prj"
            target.mkdir()
            subprocess.run([str(CLI), "init", "--no-tui", str(target)], check=True, text=True, capture_output=True)
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


class CommandRelocationMigrationSmokeTest(unittest.TestCase):
    """End-to-end: a pre-upgrade project (bundled commands committed under
    ai-specs/commands/, legacy lock with [commands]/[opted-out]) cleanly
    migrates on the next `sync` — byte-identical bundled copies removed,
    customizations preserved with a warning, genuine local commands
    untouched, lock trimmed to [meta] (+ [agents.*]), and the merged/fan-out
    command set still includes the bundled command from the cache."""

    def test_pre_upgrade_project_migrates_cleanly_on_sync(self):
        import hashlib

        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        target = Path(tmp.name) / "prj"
        target.mkdir()
        subprocess.run(["git", "init", "-q", str(target)], check=True)
        subprocess.run(
            ["git", "-C", str(target), "config", "user.email", "t@example.com"], check=True
        )
        subprocess.run(["git", "-C", str(target), "config", "user.name", "t"], check=True)

        # `ai-specs init` on a current CLI never materializes bundled commands;
        # simulate the pre-upgrade (0.16.0-era) committed state by hand.
        subprocess.run([str(CLI), "init", str(target)], check=True, capture_output=True, text=True)
        commands_dir = target / "ai-specs" / "commands"
        bundled_src = ROOT / "bundled-commands"

        # 1. Byte-identical committed bundled copy → must be removed as a leftover.
        rules_audit_content = (bundled_src / "rules-audit.md").read_text()
        (commands_dir / "rules-audit.md").write_text(rules_audit_content)

        # 2. Customized bundled copy (content differs) → must be preserved + warned.
        (commands_dir / "skills-as-rules.md").write_text(
            "# skills-as-rules (customized by this project)\n"
        )

        # 3. Genuine local command (no bundled counterpart) → must be untouched.
        (commands_dir / "my-local-command.md").write_text("# my-local-command\n")

        # 4. Legacy lock with [commands]/[opted-out] (0.16.0-era schema).
        rules_audit_hash = hashlib.sha256(rules_audit_content.encode("utf-8")).hexdigest()
        (target / "ai-specs" / ".ai-specs.lock").write_text(
            '[meta]\ncli_version = "0.16.0"\nsynced_at = "2026-07-01T00:00:00Z"\n\n'
            "[commands]\n"
            f'"rules-audit.md" = "{rules_audit_hash}"\n'
            '"skills-as-rules.md" = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"\n\n'
            "[opted-out]\n"
            'files = ["commands/some-other-file.md"]\n'
        )

        subprocess.run(
            ["git", "-C", str(target), "add", "-A"], check=True, capture_output=True
        )
        subprocess.run(
            ["git", "-C", str(target), "commit", "-qm", "pre-upgrade snapshot"], check=True
        )

        (target / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'migration-fixture'\n\n"
            "[agents]\nenabled = ['cursor', 'opencode']\n"
        )
        result = subprocess.run(
            [str(CLI), "sync", str(target)],
            check=True, capture_output=True, text=True,
        )
        out = result.stdout + result.stderr

        # Byte-identical bundled copy removed.
        self.assertFalse((commands_dir / "rules-audit.md").exists())

        # Customized copy preserved, with a warning printed.
        self.assertTrue((commands_dir / "skills-as-rules.md").exists())
        self.assertIn("customized", out)

        # Genuine local command untouched.
        self.assertEqual(
            (commands_dir / "my-local-command.md").read_text(), "# my-local-command\n"
        )

        # Lock trimmed to [meta] (+ [agents.*]) — no [commands]/[opted-out].
        lock_text = (target / "ai-specs" / ".ai-specs.lock").read_text()
        self.assertIn("[meta]", lock_text)
        self.assertNotIn("[commands]", lock_text)
        self.assertNotIn("[opted-out]", lock_text)

        # Merged/fan-out command set still includes the bundled command (from
        # cache) alongside the customized and genuine local ones.
        for rel in (
            ".cursor/commands/rules-audit.md",
            ".cursor/commands/skills-as-rules.md",
            ".cursor/commands/my-local-command.md",
            ".opencode/commands/rules-audit.md",
            ".opencode/commands/skills-as-rules.md",
            ".opencode/commands/my-local-command.md",
        ):
            self.assertTrue((target / rel).is_file(), rel)
        self.assertEqual(
            (target / ".cursor" / "commands" / "skills-as-rules.md").read_text(),
            "# skills-as-rules (customized by this project)\n",
            "the customized local copy must win over the bundled/cache tier in fan-out",
        )


if __name__ == "__main__":
    unittest.main()
