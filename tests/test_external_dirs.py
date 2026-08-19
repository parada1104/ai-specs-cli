import hashlib
import importlib.util
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path



ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "tests"))
from _blackbox import cache_project_dir, invoke, isolated_home  # noqa: E402
from _cache_paths import (  # noqa: E402
    cache_command,
    deps_skill_dir,
    inproject_deps_skill_dir,
    recipe_skill_dir,
    recipe_root,
    resolved_skills_dir,
)
from _fixture_catalog import populate_catalog  # noqa: E402

CLI = ROOT / "bin" / "ai-specs"
CATALOG = ROOT / "catalog" / "recipes"


def _cli_home(register) -> Path:
    """isolated_home registered for cleanup by ``register`` (TestCase.addCleanup)."""
    tmp = tempfile.TemporaryDirectory()
    register(tmp.cleanup)
    return isolated_home(Path(tmp.name))


def _cli_home_with_recipe(register, recipe_id: str, recipe_toml: str, *files) -> Path:
    """isolated_home whose catalog merges public+fixture plus a custom recipe."""
    tmp = tempfile.TemporaryDirectory()
    register(tmp.cleanup)
    home = isolated_home(Path(tmp.name))
    catalog = home / "catalog"
    catalog.unlink()
    recipes = catalog / "recipes"
    recipes.mkdir(parents=True)
    populate_catalog(recipes)
    rid = recipes / recipe_id
    rid.mkdir(parents=True)
    (rid / "recipe.toml").write_text(recipe_toml)
    for rel, content in files:
        dest = rid / rel
        dest.parent.mkdir(parents=True, exist_ok=True)
        dest.write_text(content)
    return home


def _load_skill_resolution():
    """Load the internal skill-resolution module (TRIAGEd assertions only)."""
    path = ROOT / "lib" / "_internal" / "skill-resolution.py"
    spec = importlib.util.spec_from_file_location("skill_resolution_internal", path)
    assert spec is not None and spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module



def _git_repo(root: Path, name: str, skill_md: str) -> Path:
    """Create a local git repo under ``root`` exposing a SKILL.md (vendored dep)."""
    repo = root / name
    repo.mkdir()
    (repo / "SKILL.md").write_text(skill_md)
    subprocess.run(["git", "init", "-q", str(repo)], check=True, text=True, capture_output=True)
    subprocess.run(["git", "-C", str(repo), "config", "user.name", "Fixture"], check=True, text=True, capture_output=True)
    subprocess.run(["git", "-C", str(repo), "config", "user.email", "f@example.com"], check=True, text=True, capture_output=True)
    subprocess.run(["git", "-C", str(repo), "add", "."], check=True, text=True, capture_output=True)
    subprocess.run(["git", "-C", str(repo), "commit", "-q", "-m", "init"], check=True, text=True, capture_output=True)
    return repo


def _demo_recipe_toml(recipe_id: str) -> str:
    """A materializable demo recipe providing skill test-skill + command test-command."""
    return (
        "[recipe]\n"
        'id = "' + recipe_id + '"\n'
        'name = "Demo Fixture Recipe"\n'
        'description = "A materializable demo recipe fixture."\n'
        'version = "1.0.0"\n'
        'author = "ai-specs"\n'
        'license = "MIT"\n'
        "\n"
        "[provides]\n"
        "skills = [\n"
        '    { id = "test-skill", source = "bundled" },\n'
        "]\n"
        "\n"
        "commands = [\n"
        '    { id = "test-command", path = "commands/test-command.md" },\n'
        "]\n"
    )


def _recipe_catalog_toml(rid: str, skill_id: str) -> str:
    """Custom catalog recipe that provides skill_id as a bundled skill."""
    return (
        "[recipe]\n"
        'id = "' + rid + '"\n'
        'name = "' + rid + '"\n'
        'description = "resolution fixture"\n'
        'version = "1.0.0"\n'
        "[provides]\n"
        "skills = [\n"
        '    { id = "' + skill_id + '", source = "bundled" },\n'
        "]\n"
        "commands = []\n"
    )


def _seed_bundled(root: Path, home: Path, skill_id: str, content: str) -> None:
    d = cache_project_dir(root, home) / ".bundled" / "skills" / skill_id
    d.mkdir(parents=True)
    (d / "SKILL.md").write_text(content)


def _seed_dep_skill(root: Path, home: Path, dep_id: str, skill_id: str, content: str) -> None:
    d = deps_skill_dir(root, dep_id, skill_id, home)
    d.mkdir(parents=True)
    (d / "SKILL.md").write_text(content)


def _seed_inproject_dep_skill(root: Path, dep_id: str, skill_id: str, content: str) -> None:
    d = inproject_deps_skill_dir(root, dep_id, skill_id)
    d.mkdir(parents=True)
    (d / "SKILL.md").write_text(content)


def _seed_local_skill(root: Path, skill_id: str, content: str) -> None:
    d = root / "ai-specs" / "skills" / skill_id
    d.mkdir(parents=True)
    (d / "SKILL.md").write_text(content)


def _seed_recipe_skill(root: Path, home: Path, recipe_id: str, skill_id: str, content: str) -> None:
    d = recipe_skill_dir(root, recipe_id, skill_id, home)
    d.mkdir(parents=True)
    (d / "SKILL.md").write_text(content)


def _init_project(root: Path, cli_home: Path) -> None:
    """Run CLI init against a bare project so sync-agent's fan-out guard passes."""
    subprocess.run([str(CLI), "init", "--no-tui", str(root)], check=True, capture_output=True, text=True)



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
    """CLI toml-dep sourcing: [[deps]] materialize in-project (ai-specs/.deps)."""

    def test_vendor_writes_to_deps_dir(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            project = tmp_path / "project"
            project.mkdir()
            ai_specs = project / "ai-specs"
            ai_specs.mkdir()
            dep_repo = _git_repo(
                tmp_path, "my-dep",
                "---\nname: my-dep\ndescription: Vendored skill.\n---\n\n# my-dep\n",
            )
            (ai_specs / "ai-specs.toml").write_text(
                "[project]\nname = 'fixture'\n\n"
                "[[deps]]\n"
                'id = "my-dep"\n'
                f'source = "{dep_repo}"\n'
            )
            home = _cli_home(self.addCleanup)
            result = invoke(project, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stderr)
            # toml-deps ([[deps]]) are project-governed -> in-project ai-specs/.deps/
            skill = inproject_deps_skill_dir(project, "my-dep") / "SKILL.md"
            self.assertTrue(skill.is_file())
            self.assertIn("name: my-dep", skill.read_text())
            # and NOT staged under the CLI cache
            self.assertFalse((deps_skill_dir(project, "my-dep", None, home) / "SKILL.md").is_file())

    def test_vendor_does_not_write_to_ai_specs_skills(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            project = tmp_path / "project"
            project.mkdir()
            ai_specs = project / "ai-specs"
            ai_specs.mkdir()
            dep_repo = _git_repo(
                tmp_path, "my-dep",
                "---\nname: my-dep\ndescription: Vendored skill.\n---\n\n# my-dep\n",
            )
            (ai_specs / "ai-specs.toml").write_text(
                "[project]\nname = 'fixture'\n\n"
                "[[deps]]\n"
                'id = "my-dep"\n'
                f'source = "{dep_repo}"\n'
            )
            home = _cli_home(self.addCleanup)
            result = invoke(project, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertFalse((project / "ai-specs" / "skills" / "my-dep").exists())


class RecipeMaterializePathTests(unittest.TestCase):
    """Recipe materialization observable via `ai-specs sync` (fixture-demo recipe)."""

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

    def _demo_home(self) -> Path:
        return _cli_home_with_recipe(
            self.addCleanup,
            "fixture-demo",
            _demo_recipe_toml("fixture-demo"),
            ("skills/test-skill/SKILL.md", "---\nname: test-skill\n---\n\n# test-skill\n"),
            ("commands/test-command.md", "# test-command\n"),
        )

    def test_materializes_bundled_skill_to_recipe_dir(self):
        root = self._make_project('[recipes.fixture-demo]\nenabled = true\nversion = "1.0.0"\n')
        home = self._demo_home()
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        skill_dir = recipe_skill_dir(root, "fixture-demo", "test-skill", home)
        self.assertTrue(skill_dir.is_dir())
        self.assertTrue((skill_dir / "SKILL.md").is_file())

    def test_materializes_command_to_cache(self):
        root = self._make_project('[recipes.fixture-demo]\nenabled = true\n')
        home = self._demo_home()
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        cmd = cache_command(root, "test-command", home)
        self.assertTrue(cmd.is_file())

    def test_warns_when_recipe_command_overwrites_existing_managed_command(self):
        root = self._make_project('[recipes.fixture-demo]\nenabled = true\n')
        home = self._demo_home()
        cmd = cache_command(root, "test-command", home)
        cmd.parent.mkdir(parents=True, exist_ok=True)
        cmd.write_text("# previous managed\n")
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("overwrites existing managed command", result.stderr)
        self.assertNotEqual(cmd.read_text(), "# previous managed\n")

    def test_materializes_recipe_dep_skill_to_deps_dir(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            dep_repo = _git_repo(
                tmp_path, "dep-skill",
                "---\nname: dep-skill\ndescription: Recipe dep skill.\n---\n\n# dep-skill\n",
            )
            home = _cli_home_with_recipe(
                self.addCleanup,
                "dep-fixture",
                (
                    "[recipe]\n"
                    'id = "dep-fixture"\n'
                    'name = "Dep Fixture"\n'
                    'description = "Recipe with dep skill."\n'
                    'version = "1.0.0"\n'
                    "[provides]\n"
                    "skills = [\n"
                    f'    {{ id = "dep-skill", source = "dep", url = "{dep_repo.as_posix()}" }},\n'
                    "]\n"
                ),
            )
            root = self._make_project('[recipes.dep-fixture]\nenabled = true\nversion = "1.0.0"\n')
            result = invoke(root, "sync", cli_home=home)
            self.assertEqual(result.returncode, 0, result.stderr)
            dep_skill = deps_skill_dir(root, "dep-skill", None, home) / "SKILL.md"
            self.assertTrue(dep_skill.is_file())
            self.assertFalse((root / "ai-specs" / "skills" / "dep-skill").exists())

    def test_local_skills_untouched_by_materialization(self):
        root = self._make_project('[recipes.fixture-demo]\nenabled = true\nversion = "1.0.0"\n')
        home = self._demo_home()
        local_skill = root / "ai-specs" / "skills" / "local-only"
        local_skill.mkdir()
        (local_skill / "SKILL.md").write_text("local")
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((local_skill / "SKILL.md").read_text(), "local")


class BundledLeftoverCleanupTests(unittest.TestCase):
    """a sync removes materialized bundled-skill copies; local/customized kept."""

    def _project(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        (root / "ai-specs" / "skills").mkdir(parents=True)
        (root / "ai-specs" / "commands").mkdir()
        (root / "ai-specs" / "ai-specs.toml").write_text(
            '[project]\nname = "bundled-leftover"\n\n[agents]\nenabled = ["claude"]\n'
        )
        return root

    def test_removes_bundled_leftover_keeps_local_and_customized(self):
        root = self._project()
        skills = root / "ai-specs" / "skills"
        bundled_src = ROOT / "bundled-skills"
        leftover = skills / "harness-lifecycle"
        leftover.mkdir()
        (leftover / "SKILL.md").write_text((bundled_src / "harness-lifecycle" / "SKILL.md").read_text())
        local = skills / "my-local-skill"
        local.mkdir()
        (local / "SKILL.md").write_text("# my-local-skill\n")
        customized = skills / "skill-creator"
        customized.mkdir()
        (customized / "SKILL.md").write_text("# skill-creator (locally edited)\n")
        home = _cli_home(self.addCleanup)
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(leftover.exists(), "bundled leftover should be removed")
        self.assertTrue(local.exists(), "genuine local skill must be preserved")
        self.assertTrue(customized.exists(), "customized copy must be preserved")
        self.assertIn("customized", result.stdout + result.stderr)

    def test_removes_untouched_old_version_copy_via_lock_hash(self):
        root = self._project()
        old = root / "ai-specs" / "skills" / "skill-creator"
        old.mkdir()
        old_content = "# skill-creator (older CLI version, untouched)\n"
        (old / "SKILL.md").write_text(old_content)
        h = hashlib.sha256(old_content.encode()).hexdigest()
        (root / "ai-specs" / ".ai-specs.lock").write_text(f'[skills."skill-creator"]\n"SKILL.md" = "{h}"\n')
        home = _cli_home(self.addCleanup)
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(old.exists(), "untouched managed copy should be removed via lock hash")

    def test_keeps_edited_copy_not_matching_source_or_lock(self):
        root = self._project()
        edited = root / "ai-specs" / "skills" / "skill-creator"
        edited.mkdir()
        (edited / "SKILL.md").write_text("# genuinely edited by the user\n")
        (root / "ai-specs" / ".ai-specs.lock").write_text('[skills."skill-creator"]\n"SKILL.md" = "0000000000000000"\n')
        home = _cli_home(self.addCleanup)
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(edited.exists(), "user-edited copy must be preserved")
        self.assertIn("customized", result.stdout + result.stderr)


class BundledCommandLeftoverCleanupTests(unittest.TestCase):
    """ai-specs refresh-bundled removes bundled-command leftovers; customized kept."""

    def _project(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        (root / "ai-specs" / "commands").mkdir(parents=True)
        (root / "ai-specs" / "skills").mkdir()
        (root / "ai-specs" / "ai-specs.toml").write_text(
            '[project]\nname = "bundled-cmd"\n\n[agents]\nenabled = ["claude"]\n'
        )
        return root



    def test_removes_bundled_leftover_keeps_local_and_customized(self):
        root = self._project()
        commands = root / "ai-specs" / "commands"
        bundled_src = ROOT / "bundled-commands"
        leftover = commands / "rules-audit.md"
        leftover.write_text((bundled_src / "rules-audit.md").read_text())
        local = commands / "my-local-command.md"
        local.write_text("# my-local-command\n")
        customized = commands / "skills-as-rules.md"
        customized.write_text("# skills-as-rules (locally edited)\n")
        home = _cli_home(self.addCleanup)
        result = invoke(root, "refresh-bundled", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(leftover.exists(), "bundled leftover should be removed")
        self.assertTrue(local.exists(), "genuine local command must be preserved")
        self.assertTrue(customized.exists(), "customized copy must be preserved")

    def test_removes_untouched_old_version_copy_via_lock_hash(self):
        root = self._project()
        old = root / "ai-specs" / "commands" / "rules-audit.md"
        old_content = "# rules-audit (older CLI version, untouched)\n"
        old.write_text(old_content)
        h = hashlib.sha256(old_content.encode()).hexdigest()
        (root / "ai-specs" / ".ai-specs.lock").write_text(f'[commands]\n"rules-audit.md" = "{h}"\n')
        home = _cli_home(self.addCleanup)
        result = invoke(root, "refresh-bundled", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(old.exists(), "untouched managed copy should be removed via lock hash")

    def test_keeps_edited_copy_not_matching_source_or_lock(self):
        root = self._project()
        edited = root / "ai-specs" / "commands" / "rules-audit.md"
        edited.write_text("# genuinely edited by the user\n")
        (root / "ai-specs" / ".ai-specs.lock").write_text('[commands]\n"rules-audit.md" = "0000000000000000"\n')
        home = _cli_home(self.addCleanup)
        result = invoke(root, "refresh-bundled", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(edited.exists(), "user-edited copy must be preserved")

    def test_no_bundled_counterpart_is_untouched(self):
        root = self._project()
        only_local = root / "ai-specs" / "commands" / "totally-local.md"
        only_local.write_text("# no bundled counterpart\n")
        home = _cli_home(self.addCleanup)
        result = invoke(root, "refresh-bundled", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(only_local.exists())



class RecipeCommandLeftoverCleanupTests(unittest.TestCase):
    """Recipe-managed command copies migrate out of ai-specs/commands safely (sync)."""


    def test_removes_untouched_recipe_copy_and_merge_stays_silent(self):
        root = self._make_project()
        content = "# recipe command\n"
        home = _cli_home_with_recipe(
            self.addCleanup, "prr",
            '[recipe]\nid="prr"\nname="P"\ndescription="d"\nversion="1.0.0"\n'
            '[provides]\ncommands=[{id="pr-create",path="commands/pr-create.md"}]\nskills=[]\n',
            ("commands/pr-create.md", content),
        )
        (root / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'fixture'\n\n[agents]\nenabled = ['claude']\n\n"
            "[recipes.prr]\nenabled = true\n"
        )
        managed = cache_command(root, "pr-create", home)
        managed.parent.mkdir(parents=True, exist_ok=True)
        managed.write_text(content)
        local = root / "ai-specs" / "commands" / "pr-create.md"
        local.write_text(content)
        result = invoke(root, "sync", cli_home=home)
        out = result.stdout + result.stderr
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(local.exists())
        self.assertNotIn("local hand-authored wins", out)

    def test_removes_untouched_recipe_copy_via_legacy_lock_hash(self):
        root = self._make_project()
        local = root / "ai-specs" / "commands" / "pr-create.md"
        content = "# older recipe command\n"
        local.write_text(content)
        digest = hashlib.sha256(content.encode()).hexdigest()
        (root / "ai-specs" / ".ai-specs.lock").write_text(f'[commands]\n"pr-create.md" = "{digest}"\n')
        home = _cli_home(self.addCleanup)
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(local.exists())

    def test_preserves_customized_recipe_copy_with_local_warning(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "skills").mkdir()
        (ai_specs / "commands").mkdir()
        (ai_specs / "ai-specs.toml").write_text(
            "[project]\nname = 'fixture'\n\n[agents]\nenabled = ['claude']\n\n"
            '[recipes.prr]\nenabled = true\n'
        )
        content = "# recipe command\n"
        home = _cli_home_with_recipe(
            self.addCleanup, "prr",
            '[recipe]\nid="prr"\nname="P"\ndescription="d"\nversion="1.0.0"\n'
            '[provides]\ncommands=[{id="pr-create",path="commands/pr-create.md"}]\nskills=[]\n',
            ("commands/pr-create.md", content),
        )
        managed = cache_command(root, "pr-create", home)
        managed.parent.mkdir(parents=True, exist_ok=True)
        managed.write_text(content)
        local = root / "ai-specs" / "commands" / "pr-create.md"
        local.write_text("# customized locally\n")
        result = invoke(root, "sync", cli_home=home)
        out = result.stdout + result.stderr
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(local.exists())
        self.assertIn("local/customized", out)

    def test_refresh_migrates_cached_recipe_copy(self):
        root = self._make_project()
        (root / "ai-specs" / "ai-specs.toml").write_text(
            '[project]\nname = "refresh-recipe-leftover"\n\n[agents]\nenabled = ["claude"]\n'
        )
        content = "# recipe-managed command\n"
        home = _cli_home(self.addCleanup)
        managed = cache_command(root, "pr-create", home)
        managed.parent.mkdir(parents=True, exist_ok=True)
        managed.write_text(content)
        local = root / "ai-specs" / "commands" / "pr-create.md"
        local.write_text(content)
        result = invoke(root, "refresh-bundled", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr or result.stdout)
        self.assertFalse(local.exists())
        self.assertNotIn("local hand-authored wins", result.stdout + result.stderr)

    def test_sync_migrates_first_recipe_copy_before_cache_exists(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name) / "project"
        root.mkdir()
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "skills").mkdir()
        (ai_specs / "commands").mkdir()
        (ai_specs / "ai-specs.toml").write_text(
            '[project]\nname = "sync-recipe-leftover"\n\n'
            '[agents]\nenabled = ["cursor"]\n\n'
            '[recipes.tdd-flow]\nenabled = true\n\n'
            '[recipes.tdd-flow.config]\ntest_command = "python3 -m unittest"\n'
        )
        content = (ROOT / "catalog" / "recipes" / "tdd-flow" / "commands" / "tdd.md").read_text()
        local = root / "ai-specs" / "commands" / "tdd.md"
        local.write_text(content)
        home = _cli_home(self.addCleanup)
        managed = cache_command(root, "tdd", home)
        self.assertFalse(managed.exists())
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr or result.stdout)
        self.assertFalse(local.exists())
        self.assertNotIn("local hand-authored wins", result.stdout + result.stderr)
        self.assertEqual(managed.read_text(), content)

    def _make_project(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "skills").mkdir()
        (ai_specs / "commands").mkdir()
        (ai_specs / "ai-specs.toml").write_text(
            '[project]\nname = "recipe-leftover"\n\n[agents]\nenabled = ["claude"]\n'
        )
        return root





class TrackedBundledCommandLeftoverTests(unittest.TestCase):
    """a git-tracked bundled command whose working-tree copy is gone is surfaced
    by `ai-specs sync` as a remediation (git still tracks / git rm --cached)."""

    def _git_project(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        subprocess.run(["git", "init", "-q", str(root)], check=True)
        subprocess.run(["git", "-C", str(root), "config", "user.email", "t@example.com"], check=True)
        subprocess.run(["git", "-C", str(root), "config", "user.name", "t"], check=True)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir(parents=True)
        (ai_specs / "skills").mkdir()
        (ai_specs / "ai-specs.toml").write_text('[project]\nname = "tracked"\n[agents]\nenabled = ["claude"]\n')
        return root

    def test_finds_tracked_command_with_missing_working_tree_copy(self):
        root = self._git_project()
        commands = root / "ai-specs" / "commands"
        commands.mkdir(parents=True)
        (commands / "rules-audit.md").write_text("# leftover\n")
        subprocess.run(["git", "-C", str(root), "add", "-A"], check=True)
        subprocess.run(["git", "-C", str(root), "commit", "-qm", "track"], check=True)
        (commands / "rules-audit.md").unlink()
        home = _cli_home(self.addCleanup)
        result = invoke(root, "sync", cli_home=home)
        out = result.stdout + result.stderr
        self.assertIn("git still tracks removed CLI-bundled command(s)", out)
        self.assertIn("rules-audit", out)

    def test_empty_when_working_tree_copy_exists(self):
        root = self._git_project()
        commands = root / "ai-specs" / "commands"
        commands.mkdir(parents=True)
        (commands / "rules-audit.md").write_text("# still here\n")
        subprocess.run(["git", "-C", str(root), "add", "-A"], check=True)
        subprocess.run(["git", "-C", str(root), "commit", "-qm", "track"], check=True)
        home = self._cli_home()
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn("git still tracks removed CLI-bundled command(s)", result.stdout + result.stderr)

    def test_empty_when_not_a_git_work_tree(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir(parents=True)
        (ai_specs / "skills").mkdir()
        (ai_specs / "commands").mkdir()
        (ai_specs / "ai-specs.toml").write_text('[project]\nname = "nongit"\n[agents]\nenabled = ["claude"]\n')
        home = _cli_home(self.addCleanup)
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn("git still tracks removed CLI-bundled command(s)", result.stdout + result.stderr)

    def _cli_home(self) -> Path:
        return _cli_home(self.addCleanup)


class SkillResolutionTests(unittest.TestCase):
    """ai-specs sync-agent folds skill sources into <cache>/resolved-skills/ with
    precedence (local > recipe > dep > bundled). Three internal-only assertions
    are carried coupled behind the single loader below."""

    @classmethod
    def setUpClass(cls):
        cls.mod = _load_skill_resolution()

    def _recipe_home(self, recipe_id: str, skill_id: str) -> Path:
        """isolated_home with a custom recipe providing skill_id as bundled."""
        toml = _recipe_catalog_toml(recipe_id, skill_id)
        return _cli_home_with_recipe(
            self.addCleanup, recipe_id, toml,
            ("skills/" + skill_id + "/SKILL.md", "# recipe-marker\n"),
        )

    def _sync_agent(self, root: Path, home: Path, dep_ids: tuple[str, ...], recipe_ids: tuple[str, ...] = ()) -> dict:
        """Refresh manifest with [[deps]]/enabled recipes, run sync-agent, read resolved."""
        manifest = root / "ai-specs" / "ai-specs.toml"
        text = manifest.read_text()
        for dep in dep_ids:
            text += f'[[deps]]\nid = "{dep}"\nsource = "file:///tmp/x"\n\n'
        for rid in recipe_ids:
            text += f'[recipes.{rid}]\nenabled = true\n\n'
        manifest.write_text(text)
        result = invoke(root, "sync-agent", "--all", cli_home=home)
        resolved = {}
        rsd = resolved_skills_dir(root, home)
        if rsd.is_dir():
            for sk in sorted(rsd.iterdir()):
                skill_md = sk / "SKILL.md"
                if skill_md.is_file():
                    resolved[sk.name] = skill_md.read_text().strip()
        return {"result": result, "resolved": resolved, "output": result.stdout + result.stderr}

    def _make_project(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        self.addCleanup(lambda: shutil.rmtree(root, ignore_errors=True))
        root.mkdir(parents=True, exist_ok=True)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "skills").mkdir()
        (ai_specs / "commands").mkdir()
        (ai_specs / "ai-specs.toml").write_text("[project]\nname = 'fixture'\n\n[agents]\nenabled = ['claude']\n")
        return root

    def _init_and_seed(self, root: Path, home: Path) -> None:
        """cli init creates AGENTS.md (sync-agent guard); then seed resolved cache."""
        _init_project(root, home)
        # The init finished with a default manifest; neutralize extra recipes so a
        # plain sync-agent still runs but materializes nothing extra.
        (root / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'fixture'\n\n[agents]\nenabled = ['claude']\n"
        )

    def test_bundled_fallback_when_no_other_source(self):
        root = self._make_project()
        home = _cli_home(self.addCleanup)
        self._init_and_seed(root, home)
        _seed_bundled(root, home, "only-bundled", "# bundled-only\n")
        data = self._sync_agent(root, home, ())
        self.assertEqual(data["result"].returncode, 0, data["result"].stderr)
        self.assertEqual(data["resolved"]["only-bundled"], "# bundled-only")

    def test_dep_precedence_over_bundled(self):
        root = self._make_project()
        home = _cli_home(self.addCleanup)
        self._init_and_seed(root, home)
        _seed_dep_skill(root, home, "d1", "shared", "# dep-d1\n")
        _seed_bundled(root, home, "shared", "# bundled-shared\n")
        data = self._sync_agent(root, home, ("d1",))
        self.assertEqual(data["result"].returncode, 0, data["result"].stderr)
        self.assertEqual(data["resolved"]["shared"], "# dep-d1")

    def test_local_precedence_over_bundled(self):
        root = self._make_project()
        home = _cli_home(self.addCleanup)
        self._init_and_seed(root, home)
        _seed_local_skill(root, "shared", "# local-shared\n")
        _seed_bundled(root, home, "shared", "# bundled-shared\n")
        data = self._sync_agent(root, home, ())
        self.assertEqual(data["result"].returncode, 0, data["result"].stderr)
        self.assertEqual(data["resolved"]["shared"], "# local-shared")

    def test_local_precedence_over_recipe(self):
        root = self._make_project()
        home = self._recipe_home("r1", "shared")
        self._init_and_seed(root, home)
        _seed_local_skill(root, "shared", "# local-shared\n")
        data = self._sync_agent(root, home, (), ("r1",))
        self.assertEqual(data["result"].returncode, 0, data["result"].stderr)
        self.assertEqual(data["resolved"]["shared"], "# local-shared")

    def test_recipe_precedence_over_dep(self):
        root = self._make_project()
        home = self._recipe_home("r1", "shared")
        self._init_and_seed(root, home)
        _seed_dep_skill(root, home, "d1", "shared", "# dep-d1\n")
        data = self._sync_agent(root, home, ("d1",), ("r1",))
        self.assertEqual(data["result"].returncode, 0, data["result"].stderr)
        self.assertEqual(data["resolved"]["shared"], "# recipe-marker")

    def test_local_precedence_over_all(self):
        root = self._make_project()
        home = self._recipe_home("r1", "shared")
        self._init_and_seed(root, home)
        _seed_local_skill(root, "shared", "# local-shared\n")
        _seed_dep_skill(root, home, "d1", "shared", "# dep-d1\n")
        data = self._sync_agent(root, home, ("d1",), ("r1",))
        self.assertEqual(data["result"].returncode, 0, data["result"].stderr)
        self.assertEqual(data["resolved"]["shared"], "# local-shared")

    def test_dep_fallback_when_no_other_source(self):
        root = self._make_project()
        home = _cli_home(self.addCleanup)
        self._init_and_seed(root, home)
        _seed_dep_skill(root, home, "d1", "only-dep", "# dep-d1\n")
        data = self._sync_agent(root, home, ("d1",))
        self.assertEqual(data["result"].returncode, 0, data["result"].stderr)
        self.assertEqual(data["resolved"]["only-dep"], "# dep-d1")

    def test_inproject_toml_dep_resolves_as_dep(self):
        root = self._make_project()
        home = _cli_home(self.addCleanup)
        self._init_and_seed(root, home)
        _seed_inproject_dep_skill(root, "d1", "only-toml-dep", "# dep-d1\n")
        data = self._sync_agent(root, home, ("d1",))
        self.assertEqual(data["result"].returncode, 0, data["result"].stderr)
        self.assertEqual(data["resolved"]["only-toml-dep"], "# dep-d1")

    def test_first_seen_dep_wins_with_warning(self):
        root = self._make_project()
        home = _cli_home(self.addCleanup)
        self._init_and_seed(root, home)
        _seed_dep_skill(root, home, "d1", "dup", "# dep-d1\n")
        _seed_dep_skill(root, home, "d2", "dup", "# dep-d2\n")
        data = self._sync_agent(root, home, ("d1", "d2"))
        out = data["output"]
        self.assertEqual(data["result"].returncode, 0, data["result"].stderr)
        self.assertEqual(data["resolved"]["dup"], "# dep-d1")
        self.assertIn("found in multiple deps", out)
        self.assertIn("d1", out)

    def test_local_override_silent_no_warning(self):
        root = self._make_project()
        home = self._recipe_home("r1", "shared")
        self._init_and_seed(root, home)
        _seed_local_skill(root, "shared", "# local-shared\n")
        data = self._sync_agent(root, home, (), ("r1",))
        out = data["output"]
        self.assertEqual(data["result"].returncode, 0, data["result"].stderr)
        self.assertEqual(data["resolved"]["shared"], "# local-shared")
        self.assertNotIn("found in multiple", out)

    def test_missing_skill_raises(self):
        # TRIAGE: no CLI surface inverts `resolve_skill`'s RuntimeError. The
        # nearest observable (ai-specs sync-agent flatten) silently skips a
        # missing skill — it never raises. Kept coupled to the internal module.
        root = self._make_project()
        with self.assertRaises(RuntimeError) as ctx:
            self.mod.resolve_skill(root, "missing", cli_home=ROOT)
        self.assertIn("missing", str(ctx.exception))

    def test_first_seen_recipe_wins_with_warning(self):
        # TRIAGE: two recipes providing the same skill id are rejected by the
        # `ai-specs sync` conflict gate ("skill.id='dup' claimed by r1, r2"),
        # so the internal first-seen recipe warning has no reachable CLI path.
        root = self._make_project()
        _seed_recipe_skill(root, ROOT, "r1", "dup", "# r1\n")
        _seed_recipe_skill(root, ROOT, "r2", "dup", "# r2\n")
        import io
        captured = io.StringIO()
        old_stderr = sys.stderr
        sys.stderr = captured
        try:
            resolved = self.mod.collect_skills(root, cli_home=ROOT)
        finally:
            sys.stderr = old_stderr
        self.assertEqual(resolved["dup"][0], "recipe")
        self.assertIn("dup", captured.getvalue())
        self.assertIn("r1", captured.getvalue())

    def test_local_precedence_does_not_backfill_files_from_recipe(self):
        # TRIAGE: resolve_skill_template is never invoked by the CLI; flatten
        # copies whole skill dirs, so template-not-backfilled is unobservable.
        root = self._make_project()
        _seed_local_skill(root, "shared", "# local\n")
        _seed_recipe_skill(root, ROOT, "r1", "shared", "# recipe\n")
        recipe_asset = recipe_skill_dir(root, "r1", "shared", ROOT) / "assets" / "helper.md"
        recipe_asset.parent.mkdir(parents=True)
        recipe_asset.write_text("recipe asset")
        self.assertIsNone(self.mod.resolve_skill_template(root, "shared", "assets/helper.md"))



class OverrideLoadingTests(unittest.TestCase):
    """Internal config/template override resolution has no CLI consumer. Every
    assertion below is TRIAGED: `load_skill_config`/`resolve_skill_template`
    are never emitted by any ai-specs verb surface, so each stays coupled."""

    @classmethod
    def setUpClass(cls):
        cls.mod = _load_skill_resolution()

    def _make_project(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "skills").mkdir()
        (ai_specs / "ai-specs.toml").write_text("[project]\nname = 'fixture'\n")
        return root

    def test_override_config_merged(self):
        # TRIAGE: `ai-specs sync <project>` with an override config.toml present
        # never surfaces the merged config - the materialized tree, exit code,
        # and stdout/stderr do not expose load_skill_config's result.
        root = self._make_project()
        _seed_recipe_skill(root, ROOT, "my-recipe", "my-skill", "# skill\n")
        overrides = root / "ai-specs" / "recipes" / "my-recipe" / "overrides" / "config.toml"
        overrides.parent.mkdir(parents=True)
        overrides.write_text('timeout = 99\n')
        cfg = self.mod.load_skill_config(root, "my-skill", {"timeout": 30})
        self.assertEqual(cfg["timeout"], 99)

    def test_override_config_missing_uses_defaults(self):
        # TRIAGE: same as above - no sync surface emits the merged default.
        root = self._make_project()
        _seed_recipe_skill(root, ROOT, "test", "recipe-skill", "# skill\n")
        cfg = self.mod.load_skill_config(root, "recipe-skill", {"timeout": 30})
        self.assertEqual(cfg["timeout"], 30)

    def test_override_config_isolated_between_recipes(self):
        # TRIAGE: recipe-local override isolation is internal to the module;
        # sync emits no per-recipe override list.
        root = self._make_project()
        _seed_recipe_skill(root, ROOT, "recipe-a", "shared-skill", "# a\n")
        _seed_recipe_skill(root, ROOT, "recipe-b", "shared-skill", "# b\n")
        overrides_a = root / "ai-specs" / "recipes" / "recipe-a" / "overrides" / "config.toml"
        overrides_a.parent.mkdir(parents=True)
        overrides_a.write_text('timeout = 99\n')
        cfg = self.mod.load_skill_config(root, "shared-skill", {"timeout": 30})
        self.assertEqual(cfg["timeout"], 99)
        _seed_recipe_skill(root, ROOT, "recipe-b", "other-skill", "# b2\n")
        cfg_b = self.mod.load_skill_config(root, "other-skill", {"timeout": 30})
        self.assertEqual(cfg_b["timeout"], 30)

    def test_override_template_preferred(self):
        # TRIAGE: override-template preference is only observable through
        # resolve_skill_template; no CLI verb calls it.
        root = self._make_project()
        _seed_recipe_skill(root, ROOT, "my-recipe", "my-skill", "# skill\n")
        bundled_tpl = recipe_skill_dir(root, "my-recipe", "my-skill", ROOT) / "template.md"
        bundled_tpl.write_text("bundled")
        override_tpl = root / "ai-specs" / "recipes" / "my-recipe" / "overrides" / "templates" / "template.md"
        override_tpl.parent.mkdir(parents=True)
        override_tpl.write_text("override")
        resolved = self.mod.resolve_skill_template(root, "my-skill", "template.md")
        self.assertEqual(resolved.read_text(), "override")

    def test_override_template_fallback_to_bundled(self):
        # TRIAGE: same - the bundled template fallback has no sync surface.
        root = self._make_project()
        _seed_recipe_skill(root, ROOT, "my-recipe", "my-skill", "# skill\n")
        bundled_tpl = recipe_skill_dir(root, "my-recipe", "my-skill", ROOT) / "template.md"
        bundled_tpl.write_text("bundled")
        resolved = self.mod.resolve_skill_template(root, "my-skill", "template.md")
        self.assertEqual(resolved.read_text(), "bundled")

    def test_override_template_missing_returns_none(self):
        # TRIAGE: resolve_skill_template has no CLI consumer, so None-on-missing
        # for a recipe skill is unobservable.
        root = self._make_project()
        _seed_recipe_skill(root, ROOT, "my-recipe", "my-skill", "# skill\n")
        resolved = self.mod.resolve_skill_template(root, "my-skill", "nonexistent.md")
        self.assertIsNone(resolved)


class OrphanCleanupTests(unittest.TestCase):
    """ai-specs sync prunes orphan .recipe/.deps trees; referenced recipe kept."""

    def _make_project(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "skills").mkdir()
        (ai_specs / "commands").mkdir()
        (ai_specs / "ai-specs.toml").write_text("[project]\nname = 'fixture'\n\n[agents]\nenabled = ['claude']\n")
        return root

    def test_orphan_recipe_directory_removed(self):
        root = self._make_project()
        orphan = root / "ai-specs" / ".recipe" / "old-recipe"
        orphan.mkdir(parents=True)
        (orphan / "keep.txt").write_text("stale")
        home = _cli_home(self.addCleanup)
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(orphan.exists())

    def test_orphan_dep_directory_removed(self):
        root = self._make_project()
        orphan = root / "ai-specs" / ".deps" / "old-dep"
        orphan.mkdir(parents=True)
        (orphan / "keep.txt").write_text("stale")
        home = _cli_home(self.addCleanup)
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(orphan.exists())

    def test_referenced_recipe_preserved(self):
        root = self._make_project()
        home = _cli_home_with_recipe(
            self.addCleanup,
            "fixture-demo",
            _demo_recipe_toml("fixture-demo"),
            ("skills/test-skill/SKILL.md", "---\nname: test-skill\n---\n\n# test-skill\n"),
            ("commands/test-command.md", "# test-command\n"),
        )
        (root / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'fixture'\n\n[agents]\nenabled = ['claude']\n\n"
            '[recipes.fixture-demo]\nenabled = true\nversion = "1.0.0"\n'
        )
        recipe_dir = recipe_root(root, "fixture-demo", home)
        recipe_dir.mkdir(parents=True)
        (recipe_dir / "keep.txt").write_text("keep")
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(
            recipe_skill_dir(root, "fixture-demo", "test-skill", home).is_dir()
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
