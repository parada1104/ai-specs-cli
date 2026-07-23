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
import sys
from pathlib import Path as _P
sys.path.insert(0, str(_P(__file__).resolve().parent))
from _cache_paths import recipe_skill_dir, recipe_root, cache_command, resolved_skills_dir


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

    def test_brief_surfaces_postmerge_sync_and_cleanup(self):
        """The always-on brief must surface both a post-merge base-sync rule
        (git pull --ff-only) and a post-merge cleanup rule."""
        recipe = self.schema.load_recipe_toml(CATALOG / RECIPE_ID / "recipe.toml")
        brief = recipe.brief_fragments
        self.assertIsNotNone(brief)
        rules = [f.text for f in (brief.workflow_rules or [])]
        self.assertTrue(
            any("ff-only" in r.lower() for r in rules),
            "post-merge base-sync workflow_rule missing (git pull --ff-only)",
        )
        self.assertTrue(
            any("worktree" in r.lower() and "merged" in r.lower() for r in rules),
            "post-merge cleanup workflow_rule missing",
        )

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
            recipe_root(root, RECIPE_ID)
            / "skills" / "git-merge-workflow" / "SKILL.md"
        )
        self.assertTrue(skill.is_file(), f"missing bundled skill at {skill}")

        cmd = cache_command(root, "pr-create")
        self.assertTrue(cmd.is_file(), f"missing command at {cmd}")

        doc = root / "ai-specs" / "recipes" / RECIPE_ID / "README.md"
        self.assertTrue(doc.is_file(), f"missing doc at {doc}")

    def test_materialize_does_not_stage_premerge_guardian_into_project(self):
        root = self._make_project()
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)
        helper = root / "ai-specs" / "bin" / "premerge_guardian.py"
        self.assertFalse(helper.exists(), f"unexpected in-project guardian at {helper}")
        skill = (
            recipe_root(root, RECIPE_ID)
            / "skills" / "git-merge-workflow" / "SKILL.md"
        )
        text = skill.read_text()
        self.assertIn("AI_SPECS_HOME", text)
        self.assertIn("lib/_internal/premerge_guardian.py", text)
        self.assertNotIn("ai-specs/bin/premerge_guardian.py", text)

    def test_canonical_guardian_lives_in_cli_home(self):
        canon = ROOT / "lib" / "_internal" / "premerge_guardian.py"
        self.assertTrue(canon.is_file())
        self.assertIn("pre-merge guardian", canon.read_text().lower())
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

    def test_skill_classifies_protected_heads(self):
        """Skill skips delete/cleanup for protected long-lived heads."""
        self.assertIn("Head branch class", self.skill_text)
        self.assertIn("development", self.skill_text)
        self.assertIn("staging", self.skill_text)
        self.assertIn("Protected head", self.skill_text)
        self.assertIn("--delete-branch", self.skill_text)
        self.assertIn("never pass --delete-branch", self.skill_text.lower())

    def test_skill_preflight_checks_delete_branch_on_merge(self):
        """Skill warns when GitHub auto-deletes heads on merge."""
        self.assertIn("delete_branch_on_merge", self.skill_text)
        self.assertIn(
            "gh api -X PATCH repos/$REPO -f delete_branch_on_merge=false",
            self.skill_text,
        )

    def test_skill_prefers_release_head_for_main(self):
        """Skill documents release/* heads for shipping to main."""
        self.assertIn("release/v", self.skill_text)


if __name__ == "__main__":
    unittest.main()
