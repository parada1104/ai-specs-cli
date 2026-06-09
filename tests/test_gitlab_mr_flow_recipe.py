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


if __name__ == "__main__":
    unittest.main()
