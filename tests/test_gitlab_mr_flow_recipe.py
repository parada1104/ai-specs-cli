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
RECIPE_ID = "gitlab-mr-flow"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


class GitlabMrFlowRecipeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(RECIPE_MATERIALIZE_PATH, "recipe_materialize_gitlab")
        cls.schema = load_module(RECIPE_SCHEMA_PATH, "recipe_schema_gitlab")

    # --- Phase 1: Manifest and Binding ---

    def test_recipe_validates_and_declares_vcs_pr_flow(self):
        """Recipe is valid and declares vcs-pr-flow capability."""
        recipe_dir = CATALOG / RECIPE_ID
        recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
        self.assertEqual(recipe.id, RECIPE_ID)
        cap_ids = [c.id for c in recipe.capabilities]
        self.assertIn("vcs-pr-flow", cap_ids)

    def test_recipe_declares_gitlab_provider_default(self):
        """Config declares provider=gitlab as default."""
        recipe_dir = CATALOG / RECIPE_ID
        recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
        provider_field = recipe.config_schema.fields.get("provider")
        self.assertIsNotNone(provider_field, "provider config field must exist")
        self.assertFalse(provider_field.required)
        self.assertEqual(provider_field.default, "gitlab")

    def test_recipe_declares_development_base_branch_default(self):
        """Config declares base_branch=development as default."""
        recipe_dir = CATALOG / RECIPE_ID
        recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
        base_field = recipe.config_schema.fields.get("base_branch")
        self.assertIsNotNone(base_field, "base_branch config field must exist")
        self.assertFalse(base_field.required)
        self.assertEqual(base_field.default, "development")

    def test_recipe_declares_validate_config_hook(self):
        """Recipe declares on-sync validate-config hook."""
        recipe_dir = CATALOG / RECIPE_ID
        recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
        hook_pairs = [(h.event, h.action) for h in recipe.hooks]
        self.assertIn(("on-sync", "validate-config"), hook_pairs)

    def test_recipe_declares_bundled_skill(self):
        """Recipe declares bundled gitlab-merge-workflow skill."""
        recipe_dir = CATALOG / RECIPE_ID
        recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
        skill_ids = [(s.id, s.source) for s in recipe.skills]
        self.assertIn(("gitlab-merge-workflow", "bundled"), skill_ids)

    def test_recipe_declares_mr_create_command(self):
        """Recipe declares mr-create command."""
        recipe_dir = CATALOG / RECIPE_ID
        recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
        cmd_ids = [c.id for c in recipe.commands]
        self.assertIn("mr-create", cmd_ids)

    def test_recipe_declares_readme_doc(self):
        """Recipe declares README.md doc provision."""
        recipe_dir = CATALOG / RECIPE_ID
        recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
        doc_targets = [d.target for d in recipe.docs]
        self.assertIn("ai-specs/recipes/gitlab-mr-flow/README.md", doc_targets)

    # --- Phase 2: Materialization ---

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

    def test_materialize_produces_skill(self):
        """Sync materializes the bundled gitlab-merge-workflow skill."""
        root = self._make_project()
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)
        skill = (
            root / "ai-specs" / ".recipe" / RECIPE_ID
            / "skills" / "gitlab-merge-workflow" / "SKILL.md"
        )
        self.assertTrue(skill.is_file(), f"missing bundled skill at {skill}")

    def test_materialize_produces_command(self):
        """Sync materializes the mr-create command."""
        root = self._make_project()
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)
        cmd = root / "ai-specs" / "commands" / "mr-create.md"
        self.assertTrue(cmd.is_file(), f"missing command at {cmd}")

    def test_materialize_produces_readme(self):
        """Sync materializes the README doc."""
        root = self._make_project()
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)
        doc = root / "ai-specs" / "recipes" / RECIPE_ID / "README.md"
        self.assertTrue(doc.is_file(), f"missing doc at {doc}")

    def test_materialize_does_not_touch_github_assets(self):
        """Sync does not modify git-pr-flow recipe assets."""
        root = self._make_project()
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)
        github_skill = (
            root / "ai-specs" / ".recipe" / "git-pr-flow"
            / "skills" / "git-merge-workflow" / "SKILL.md"
        )
        self.assertFalse(
            github_skill.exists(),
            "git-pr-flow assets must not be materialized when only gitlab-mr-flow is enabled"
        )


class GitlabMrFlowBindingTests(unittest.TestCase):
    """Provider binding semantics: ambiguity and explicit binding."""

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(RECIPE_MATERIALIZE_PATH, "recipe_materialize_gitlab_binding")

    def _make_v2_recipe(self, tmp: str, rid: str, caps: list[str] = None):
        recipe_dir = Path(tmp) / rid
        recipe_dir.mkdir(parents=True, exist_ok=True)
        cap_lines = "".join(f'[[capabilities]]\nid = "{c}"\n' for c in (caps or []))
        (recipe_dir / "recipe.toml").write_text(
            f'[recipe]\nid = "{rid}"\nname = "{rid.title()}"\ndescription = "D"\nversion = "1.0"\n'
            + cap_lines
        )

    def test_dual_vcs_pr_flow_providers_stay_unbound_without_binding(self):
        """When both git-pr-flow and gitlab-mr-flow are enabled without bindings, vcs-pr-flow stays unbound."""
        with tempfile.TemporaryDirectory() as tmp:
            catalog = Path(tmp)
            self._make_v2_recipe(tmp, "git-pr-flow", caps=["vcs-pr-flow"])
            self._make_v2_recipe(tmp, "gitlab-mr-flow", caps=["vcs-pr-flow"])
            bindings = self.mod.resolve_bindings(
                catalog, ["git-pr-flow", "gitlab-mr-flow"], []
            )
            self.assertNotIn("vcs-pr-flow", bindings)

    def test_explicit_binding_selects_gitlab(self):
        """Explicit binding to gitlab-mr-flow selects it for vcs-pr-flow."""
        with tempfile.TemporaryDirectory() as tmp:
            catalog = Path(tmp)
            self._make_v2_recipe(tmp, "git-pr-flow", caps=["vcs-pr-flow"])
            self._make_v2_recipe(tmp, "gitlab-mr-flow", caps=["vcs-pr-flow"])
            bindings = self.mod.resolve_bindings(
                catalog,
                ["git-pr-flow", "gitlab-mr-flow"],
                [{"capability": "vcs-pr-flow", "recipe": "gitlab-mr-flow"}],
            )
            self.assertEqual(bindings.get("vcs-pr-flow"), "gitlab-mr-flow")


class GitlabMrFlowGoldenContentTests(unittest.TestCase):
    """Phase 3: golden text checks for skill and command content."""

    @classmethod
    def setUpClass(cls):
        cls.skill_path = (
            CATALOG / RECIPE_ID / "skills" / "gitlab-merge-workflow" / "SKILL.md"
        )
        cls.command_path = CATALOG / RECIPE_ID / "commands" / "mr-create.md"
        cls.skill_text = cls.skill_path.read_text()
        cls.command_text = cls.command_path.read_text()

    # --- Skill golden content ---

    def test_skill_checks_glab_installed(self):
        """Skill checks that glab is installed via command -v glab."""
        self.assertIn("command -v glab", self.skill_text)

    def test_skill_checks_glab_auth(self):
        """Skill checks glab authentication via glab auth status."""
        self.assertIn("glab auth status", self.skill_text)

    def test_skill_uses_explicit_push(self):
        """Skill uses explicit git push -u origin before MR creation."""
        self.assertIn("git push -u origin", self.skill_text)

    def test_skill_uses_glab_mr_create_with_required_flags(self):
        """Skill creates MR with glab mr create and required flags."""
        self.assertIn("glab mr create", self.skill_text)
        self.assertIn("--source-branch", self.skill_text)
        self.assertIn("--target-branch", self.skill_text)
        self.assertIn("--title", self.skill_text)
        self.assertIn("--description", self.skill_text)
        self.assertIn("--yes", self.skill_text)

    def test_skill_does_not_use_fill(self):
        """Skill does not use --fill (implicit push is forbidden)."""
        self.assertNotIn("--fill", self.skill_text)

    def test_skill_does_not_auto_merge(self):
        """Skill does not include auto-merge flags."""
        self.assertNotIn("--merge-when-pipeline-succeeds", self.skill_text)
        self.assertNotIn("auto-merge", self.skill_text.lower())

    # --- Command golden content ---

    def test_command_checks_glab_installed(self):
        """Command checks that glab is installed via command -v glab."""
        self.assertIn("command -v glab", self.command_text)

    def test_command_checks_glab_auth(self):
        """Command checks glab authentication via glab auth status."""
        self.assertIn("glab auth status", self.command_text)

    def test_command_uses_explicit_push(self):
        """Command uses explicit git push -u origin before MR creation."""
        self.assertIn("git push -u origin", self.command_text)

    def test_command_uses_glab_mr_create_with_required_flags(self):
        """Command creates MR with glab mr create and required flags."""
        self.assertIn("glab mr create", self.command_text)
        self.assertIn("--source-branch", self.command_text)
        self.assertIn("--target-branch", self.command_text)
        self.assertIn("--title", self.command_text)
        self.assertIn("--description", self.command_text)
        self.assertIn("--yes", self.command_text)

    def test_command_does_not_use_fill(self):
        """Command does not use --fill (implicit push is forbidden)."""
        self.assertNotIn("--fill", self.command_text)

    def test_command_does_not_auto_merge(self):
        """Command does not include auto-merge flags."""
        self.assertNotIn("--merge-when-pipeline-succeeds", self.command_text)
        self.assertNotIn("auto-merge", self.command_text.lower())

    def test_command_push_before_create_order(self):
        """Command places git push before glab mr create."""
        push_pos = self.command_text.find("git push -u origin")
        create_pos = self.command_text.find("glab mr create")
        self.assertGreater(
            create_pos, push_pos,
            "git push must appear before glab mr create in the command"
        )

    def test_skill_push_before_create_order(self):
        """Skill places git push before glab mr create."""
        push_pos = self.skill_text.find("git push -u origin")
        create_pos = self.skill_text.find("glab mr create")
        self.assertGreater(
            create_pos, push_pos,
            "git push must appear before glab mr create in the skill"
        )

    # --- Runtime blocker messages (verify-report remediation) ---

    def test_skill_install_blocker_message(self):
        """Skill contains exact install blocker message when glab is missing."""
        self.assertIn(
            "glab` is not installed",
            self.skill_text,
            "Skill must contain install blocker message"
        )
        self.assertIn(
            "https://gitlab.com/gitlab-org/cli",
            self.skill_text,
            "Skill install blocker must include installation URL"
        )

    def test_skill_auth_blocker_message(self):
        """Skill contains exact auth blocker message when glab is unauthenticated."""
        self.assertIn(
            "glab` is not authenticated",
            self.skill_text,
            "Skill must contain auth blocker message"
        )
        self.assertIn(
            "glab auth login",
            self.skill_text,
            "Skill auth blocker must include remediation command"
        )

    def test_skill_preflight_before_push_order(self):
        """Skill checks glab install and auth BEFORE git push."""
        install_check_pos = self.skill_text.find("command -v glab")
        auth_check_pos = self.skill_text.find("glab auth status")
        push_pos = self.skill_text.find("git push -u origin")
        self.assertGreater(
            push_pos, install_check_pos,
            "git push must appear AFTER command -v glab in the skill"
        )
        self.assertGreater(
            push_pos, auth_check_pos,
            "git push must appear AFTER glab auth status in the skill"
        )

    def test_skill_stops_after_mr_create_reports_url(self):
        """Skill STOPs after MR creation and reports the MR URL."""
        create_pos = self.skill_text.find("glab mr create")
        stop_pos = self.skill_text.find("STOP")
        self.assertGreater(
            stop_pos, create_pos,
            "STOP instruction must appear AFTER glab mr create in the skill"
        )
        self.assertIn(
            "Report the MR URL",
            self.skill_text,
            "Skill must instruct to report the MR URL after creation"
        )
        self.assertIn(
            "Do not merge",
            self.skill_text,
            "Skill must explicitly say not to merge after MR creation"
        )

    def test_command_install_blocker_message(self):
        """Command contains exact install blocker message when glab is missing."""
        self.assertIn(
            "glab` is not installed",
            self.command_text,
            "Command must contain install blocker message"
        )
        self.assertIn(
            "https://gitlab.com/gitlab-org/cli",
            self.command_text,
            "Command install blocker must include installation URL"
        )

    def test_command_auth_blocker_message(self):
        """Command contains exact auth blocker message when glab is unauthenticated."""
        self.assertIn(
            "glab` is not authenticated",
            self.command_text,
            "Command must contain auth blocker message"
        )
        self.assertIn(
            "glab auth login",
            self.command_text,
            "Command auth blocker must include remediation command"
        )

    def test_command_preflight_before_push_order(self):
        """Command checks glab install and auth BEFORE git push."""
        install_check_pos = self.command_text.find("command -v glab")
        auth_check_pos = self.command_text.find("glab auth status")
        push_pos = self.command_text.find("git push -u origin")
        self.assertGreater(
            push_pos, install_check_pos,
            "git push must appear AFTER command -v glab in the command"
        )
        self.assertGreater(
            push_pos, auth_check_pos,
            "git push must appear AFTER glab auth status in the command"
        )

    def test_command_stops_after_mr_create_reports_url(self):
        """Command STOPs after MR creation and reports the MR URL."""
        create_pos = self.command_text.find("glab mr create")
        stop_pos = self.command_text.find("STOP")
        self.assertGreater(
            stop_pos, create_pos,
            "STOP instruction must appear AFTER glab mr create in the command"
        )
        self.assertIn(
            "Report the MR URL",
            self.command_text,
            "Command must instruct to report the MR URL after creation"
        )
        self.assertIn(
            "Do not merge",
            self.command_text,
            "Command must explicitly say not to merge after MR creation"
        )


if __name__ == "__main__":
    unittest.main()
