import sys
import tempfile
import tomllib
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
CATALOG = ROOT / "catalog" / "recipes"
RECIPE_ID = "git-pr-flow"
sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import isolated_home, invoke, temp_project
from _cache_paths import cache_command, recipe_skill_dir


def _recipe_toml() -> dict:
    with (CATALOG / RECIPE_ID / "recipe.toml").open("rb") as fh:
        return tomllib.load(fh)


class GitPrFlowRecipeTests(unittest.TestCase):
    def _enable_recipe(self, root: Path) -> Path:
        """Append the git-pr-flow recipe to an isolated project manifest."""
        manifest = root / "ai-specs" / "ai-specs.toml"
        recipe_version = _recipe_toml()["recipe"]["version"]
        with manifest.open("a", encoding="utf-8") as fh:
            fh.write(
                f"\n[recipes.{RECIPE_ID}]\nenabled = true\nversion = \"{recipe_version}\"\n"
            )
        return root

    def _recipe_list(self, root: Path, home: Path) -> str:
        """Rendered `ai-specs recipe list` output (process-boundary anchor)."""
        result = invoke(root, "recipe", "list", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn(RECIPE_ID, result.stdout)
        return result.stdout

    def setUp(self):
        self.home_td = tempfile.TemporaryDirectory(prefix="gprf-home-")
        self.addCleanup(self.home_td.cleanup)
        self.home = isolated_home(Path(self.home_td.name))

    def test_recipe_has_no_provider_config(self):
        """Config must not declare provider — recipe id is the provider identity.

        Observable via `ai-specs recipe list` (recipe present and installable)
        plus the catalogue [config.*] content: no field keyed `provider` is
        declared, so sibling VCS recipes keep the recipe id as provider identity.
        """
        td, root = temp_project(name="fixture", agents=("claude",))
        self.addCleanup(td.cleanup)
        root = self._enable_recipe(root)
        self._recipe_list(root, self.home)

        data = _recipe_toml()
        config = data.get("config", {})
        self.assertNotIn(
            "provider",
            config,
            "provider config field must not exist on sibling VCS recipes",
        )

    def test_recipe_validates_and_declares_capability(self):
        """The packaged recipe declares vcs-pr-flow, its bundled skill, and command.

        `ai-specs recipe list` renders the recipe as available/installed with the
        catalogue version; the capability, bundled-skill, and command declarations
        come from the packaged recipe content (schema-parseable catalogue data).
        """
        td, root = temp_project(name="fixture", agents=("claude",))
        self.addCleanup(td.cleanup)
        root = self._enable_recipe(root)
        stdout = self._recipe_list(root, self.home)
        self.assertIn(_recipe_toml()["recipe"]["version"], stdout)

        data = _recipe_toml()
        cap_ids = [c["id"] for c in data.get("capabilities", [])]
        self.assertIn("vcs-pr-flow", cap_ids)
        # Bundled skill is declared
        skills = [(s["id"], s.get("source")) for s in data["provides"]["skills"]]
        self.assertIn(("git-merge-workflow", "bundled"), skills)
        # Command is declared
        cmd_ids = [c["id"] for c in data["provides"]["commands"]]
        self.assertIn("pr-create", cmd_ids)

    def test_brief_surfaces_postmerge_sync_and_cleanup(self):
        """The packaged brief (provides.brief) surfaces both a post-merge base-sync
        rule (git pull --ff-only) and a post-merge cleanup rule."""
        td, root = temp_project(name="fixture", agents=("claude",))
        self.addCleanup(td.cleanup)
        root = self._enable_recipe(root)
        self._recipe_list(root, self.home)

        data = _recipe_toml()
        brief = data["provides"].get("brief")
        self.assertIsNotNone(brief)
        rules = brief.get("workflow_rules") or []
        self.assertTrue(
            any("ff-only" in r.lower() for r in rules),
            "post-merge base-sync workflow_rule missing (git pull --ff-only)",
        )
        self.assertTrue(
            any("worktree" in r.lower() and "merged" in r.lower() for r in rules),
            "post-merge cleanup workflow_rule missing",
        )

    def test_materialize_produces_skill_command_and_doc(self):
        """`ai-specs sync` materializes the bundled skill, the pr-create command,
        and the recipe README doc into the project/cache."""
        td, root = temp_project(name="fixture", agents=("claude",))
        self.addCleanup(td.cleanup)
        root = self._enable_recipe(root)
        result = invoke(root, "sync", cli_home=self.home)
        self.assertEqual(result.returncode, 0, result.stderr)

        skill = recipe_skill_dir(root, RECIPE_ID, "git-merge-workflow", cli_home=self.home) / "SKILL.md"
        self.assertTrue(skill.is_file(), f"missing bundled skill at {skill}")

        cmd = cache_command(root, "pr-create", cli_home=self.home)
        self.assertTrue(cmd.is_file(), f"missing command at {cmd}")

        doc = root / "ai-specs" / "recipes" / RECIPE_ID / "README.md"
        self.assertTrue(doc.is_file(), f"missing doc at {doc}")

    def test_materialize_does_not_stage_premerge_guardian_into_project(self):
        """Sync materializes the guardian's location alongside the skill but never
        stages a `bin/` copy into the project."""
        td, root = temp_project(name="fixture", agents=("claude",))
        self.addCleanup(td.cleanup)
        root = self._enable_recipe(root)
        result = invoke(root, "sync", cli_home=self.home)
        self.assertEqual(result.returncode, 0, result.stderr)

        helper = root / "ai-specs" / "bin" / "premerge_guardian.py"
        self.assertFalse(helper.exists(), f"unexpected in-project guardian at {helper}")
        skill = recipe_skill_dir(root, RECIPE_ID, "git-merge-workflow", cli_home=self.home) / "SKILL.md"
        text = skill.read_text()
        self.assertIn("AI_SPECS_HOME", text)
        self.assertIn("lib/_internal/premerge_guardian.py", text)
        self.assertNotIn("ai-specs/bin/premerge_guardian.py", text)

    def test_canonical_guardian_lives_in_cli_home(self):
        """The pre-merge guardian ships inside the CLI install root (isolated home)."""
        canon = self.home / "lib" / "_internal" / "premerge_guardian.py"
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
