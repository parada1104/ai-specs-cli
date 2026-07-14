import importlib.util
import sys
import tempfile
import tomllib
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
RECIPE_MATERIALIZE_PATH = ROOT / "lib" / "_internal" / "recipe-materialize.py"
RECIPE_SCHEMA_PATH = ROOT / "lib" / "_internal" / "recipe_schema.py"
CATALOG = ROOT / "catalog" / "recipes"
RECIPE_ID = "git-pr-flow"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


class GitPrFlowRecipeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(RECIPE_MATERIALIZE_PATH, "recipe_materialize_internal")
        cls.schema = load_module(RECIPE_SCHEMA_PATH, "recipe_schema_for_git_pr_flow")

    def test_recipe_has_no_provider_config(self):
        """Config must not declare provider — recipe id is the provider identity."""
        recipe_dir = CATALOG / RECIPE_ID
        recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
        self.assertNotIn(
            "provider",
            recipe.config_schema.fields,
            "provider config field must not exist on sibling VCS recipes",
        )

    def test_recipe_validates_and_declares_capability(self):
        recipe_dir = CATALOG / RECIPE_ID
        recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
        self.assertEqual(recipe.id, RECIPE_ID)
        cap_ids = [c.id for c in recipe.capabilities]
        self.assertIn("vcs-pr-flow", cap_ids)
        # Bundled skill is declared
        skill_ids = [(s.id, s.source) for s in recipe.skills]
        self.assertIn(("git-merge-workflow", "bundled"), skill_ids)
        # Command is declared
        cmd_ids = [c.id for c in recipe.commands]
        self.assertIn("pr-create", cmd_ids)

    def _make_project(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "skills").mkdir()
        (ai_specs / "commands").mkdir()
        with (CATALOG / RECIPE_ID / "recipe.toml").open("rb") as fh:
            recipe_version = tomllib.load(fh)["recipe"]["version"]
        manifest = ai_specs / "ai-specs.toml"
        manifest.write_text(
            "[project]\nname = 'fixture'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            f'[recipes.{RECIPE_ID}]\nenabled = true\nversion = "{recipe_version}"\n'
        )
        return root

    def test_materialize_produces_skill_command_and_doc(self):
        root = self._make_project()
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)

        skill = (
            root / "ai-specs" / ".recipe" / RECIPE_ID
            / "skills" / "git-merge-workflow" / "SKILL.md"
        )
        self.assertTrue(skill.is_file(), f"missing bundled skill at {skill}")

        cmd = root / "ai-specs" / "commands" / "pr-create.md"
        self.assertTrue(cmd.is_file(), f"missing command at {cmd}")

        doc = root / "ai-specs" / "recipes" / RECIPE_ID / "README.md"
        self.assertTrue(doc.is_file(), f"missing doc at {doc}")

    def test_materialize_ships_premerge_guardian_template(self):
        root = self._make_project()
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)
        helper = root / "ai-specs" / "bin" / "premerge_guardian.py"
        self.assertTrue(helper.is_file(), f"missing guardian template at {helper}")
        skill = (
            root / "ai-specs" / ".recipe" / RECIPE_ID
            / "skills" / "git-merge-workflow" / "SKILL.md"
        )
        self.assertIn("ai-specs/bin/premerge_guardian.py", skill.read_text())

    def test_template_matches_canonical_guardian(self):
        canon = (ROOT / "lib" / "_internal" / "premerge_guardian.py").read_text()
        shipped = (
            CATALOG / RECIPE_ID / "templates" / "premerge_guardian.py"
        ).read_text()
        self.assertEqual(shipped, canon)
class GitPrFlowGoldenContentTests(unittest.TestCase):
    """Golden text checks for pre-merge archive guidance."""

    @classmethod
    def setUpClass(cls):
        cls.skill_path = (
            CATALOG / RECIPE_ID / "skills" / "git-merge-workflow" / "SKILL.md"
        )
        cls.skill_text = cls.skill_path.read_text()

    def test_skill_requires_pre_merge_archive_before_merge(self):
        """Skill archives SDD/OpenSpec artifacts before gh pr merge."""
        merge_pos = self.skill_text.find("gh pr merge")
        archive_pos = self.skill_text.find("archive and record SDD/OpenSpec artifacts")
        self.assertGreater(archive_pos, 0)
        self.assertGreater(merge_pos, 0)
        self.assertLess(archive_pos, merge_pos)

    def test_skill_requires_post_merge_branch_cleanup(self):
        """Skill force-deletes local branch and removes worktree after merge."""
        self.assertIn("git branch -D", self.skill_text)
        self.assertIn("git worktree remove", self.skill_text)
        self.assertIn("git push origin --delete", self.skill_text)


if __name__ == "__main__":
    unittest.main()
