"""Validation + materialization tests for the plan-build-flow catalog recipe."""

import importlib.util
import re
import sys
import tempfile
import tomllib
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
RECIPE_MATERIALIZE_PATH = ROOT / "lib" / "_internal" / "recipe-materialize.py"
RECIPE_SCHEMA_PATH = ROOT / "lib" / "_internal" / "recipe_schema.py"
CATALOG = ROOT / "catalog" / "recipes"
RECIPE_ID = "plan-build-flow"
import sys
from pathlib import Path as _P
sys.path.insert(0, str(_P(__file__).resolve().parent))
from _cache_paths import recipe_skill_dir, recipe_root, cache_command, resolved_skills_dir

FORBIDDEN_TERMS = ("sdd", "openspec", "spec-driven")
FORBIDDEN_SLASH = ("/plan", "/build", "/archive")


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def _version_of(recipe_id: str) -> str:
    text = (CATALOG / recipe_id / "recipe.toml").read_text()
    match = re.search(r'^\s*version\s*=\s*"([^"]+)"', text, re.MULTILINE)
    assert match, f"could not find version in {recipe_id}/recipe.toml"
    return match.group(1)


def _recipe_version() -> str:
    return _version_of(RECIPE_ID)


class PlanBuildFlowRecipeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(RECIPE_MATERIALIZE_PATH, "recipe_materialize_pbf")
        cls.schema = load_module(RECIPE_SCHEMA_PATH, "recipe_schema_pbf")

    def _make_project(self, extra_recipes: str = "") -> Path:
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
            f'[recipes.{RECIPE_ID}]\nenabled = true\nversion = "{_recipe_version()}"\n'
            + extra_recipes
        )
        return root

    def test_recipe_materializes_skill_only(self):
        recipe_dir = CATALOG / RECIPE_ID
        recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
        self.assertEqual(recipe.id, RECIPE_ID)
        self.assertEqual(len(recipe.commands), 0)
        skill_ids = [(s.id, s.source) for s in recipe.skills]
        self.assertIn(("plan-build-flow", "bundled"), skill_ids)

        root = self._make_project()
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)

        skill = (
            recipe_root(root, RECIPE_ID)
            / "skills" / "plan-build-flow" / "SKILL.md"
        )
        self.assertTrue(skill.is_file())
        for forbidden in ("plan.md", "build.md", "archive.md"):
            self.assertFalse(
                (root / "ai-specs" / "commands" / forbidden).exists(),
                f"unexpected command {forbidden}",
            )

    def test_recipe_adds_no_schema_surface(self):
        recipe_dir = CATALOG / RECIPE_ID
        recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
        self.assertEqual(len(recipe.config_schema.fields), 0)
        hook_pairs = [(h.event, h.action) for h in recipe.hooks]
        self.assertEqual(hook_pairs, [("on-sync", "validate-config")])

        raw = (recipe_dir / "recipe.toml").read_text().lower()
        for term in FORBIDDEN_TERMS:
            self.assertNotIn(term, raw)

    def test_skill_has_ambient_auto_invoke(self):
        skill = CATALOG / RECIPE_ID / "skills" / "plan-build-flow" / "SKILL.md"
        text = skill.read_text()
        self.assertIn("auto_invoke:", text)
        self.assertIn("substantial", text.lower())
        self.assertNotIn("/plan", text.split("auto_invoke")[0])  # frontmatter ok

    def test_brief_and_readme_vocabulary_clean(self):
        recipe_dir = CATALOG / RECIPE_ID
        recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
        brief = recipe.brief_fragments
        self.assertIsNotNone(brief)
        rules = [fragment.text for fragment in (brief.workflow_rules or [])]
        fragments = "\n".join(rules).lower()
        for term in FORBIDDEN_TERMS:
            self.assertNotIn(term, fragments)
        for slash in FORBIDDEN_SLASH:
            self.assertNotIn(slash, fragments)

        root = self._make_project()
        self.mod.materialize_recipes(root, ROOT)
        readme = (root / "ai-specs" / "recipes" / RECIPE_ID / "README.md").read_text().lower()
        for term in FORBIDDEN_TERMS:
            self.assertNotIn(term, readme)

    def test_implementation_brief_references_worktree_flow(self):
        recipe_dir = CATALOG / RECIPE_ID
        recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
        brief = recipe.brief_fragments
        self.assertIsNotNone(brief)
        rules = [fragment.text for fragment in (brief.workflow_rules or [])]
        combined = "\n".join(rules).lower()
        self.assertIn("worktree", combined)
        self.assertNotIn("/build", combined)
        self.assertNotIn("worktree-flow", recipe.conflicts_with)

    def test_classic_sdd_commands_unchanged(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "skills").mkdir()
        commands = ai_specs / "commands"
        commands.mkdir()
        legacy = commands / "legacy-sdd-cmd.md"
        legacy.write_text("# Legacy\n")
        (ai_specs / "skills" / "legacy-sdd-skill").mkdir()
        (ai_specs / "skills" / "legacy-sdd-skill" / "SKILL.md").write_text("---\nname: legacy\n---\n")

        manifest = ai_specs / "ai-specs.toml"
        manifest.write_text(
            "[project]\nname = 'fixture'\n\n[agents]\nenabled = ['claude']\n\n"
            f'[recipes.{RECIPE_ID}]\nenabled = true\nversion = "{_recipe_version()}"\n'
        )
        before = legacy.read_text()
        self.mod.materialize_recipes(root, ROOT)
        self.assertEqual(legacy.read_text(), before)

    def test_skill_has_change_depth_classifier(self):
        skill = CATALOG / RECIPE_ID / "skills" / "plan-build-flow" / "SKILL.md"
        text = skill.read_text().lower()
        self.assertIn("change depth classifier", text)
        for tier in ("full", "standard", "light"):
            self.assertIn(tier, text)

    def test_skill_has_pr_and_archive_gates(self):
        skill = CATALOG / RECIPE_ID / "skills" / "plan-build-flow" / "SKILL.md"
        raw = skill.read_text()
        text = raw.lower()
        self.assertIn("pr creation gate", text)
        self.assertIn("pre-merge archive gate", text)
        self.assertIn("pre-merge merge guardian", text)
        self.assertIn("premerge_guardian", text)
        self.assertIn("AI_SPECS_HOME", raw)
        self.assertIn("lib/_internal/premerge_guardian.py", text)
        self.assertNotIn("ai-specs/bin/premerge_guardian.py", text)
        self.assertIn("gh pr create", text)
        self.assertIn("before merge", text)

    def test_recipe_does_not_stage_premerge_guardian_into_project(self):
        recipe = self.schema.load_recipe_toml(CATALOG / RECIPE_ID / "recipe.toml")
        targets = [t.target for t in recipe.templates]
        self.assertNotIn("ai-specs/bin/premerge_guardian.py", targets)
        self.assertTrue(
            (ROOT / "lib" / "_internal" / "premerge_guardian.py").is_file()
        )

    def test_brief_mentions_depth_and_pr_gate(self):
        recipe_dir = CATALOG / RECIPE_ID
        recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
        brief = recipe.brief_fragments
        rules = [fragment.text for fragment in (brief.workflow_rules or [])]
        combined = "\n".join(rules).lower()
        self.assertIn("classify", combined)
        self.assertIn("tasks-only", combined)
        self.assertIn("do not open a pr", combined)
        self.assertIn("before merge", combined)


if __name__ == "__main__":
    unittest.main()
