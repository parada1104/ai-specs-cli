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

FORBIDDEN_TERMS = ("sdd", "openspec", "spec-driven")


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def _version_of(recipe_id: str) -> str:
    """Read a recipe's version dynamically from its recipe.toml. Never hardcode."""
    text = (CATALOG / recipe_id / "recipe.toml").read_text()
    match = re.search(r'^\s*version\s*=\s*"([^"]+)"', text, re.MULTILINE)
    assert match, f"could not find version in {recipe_id}/recipe.toml"
    return match.group(1)


def _recipe_version() -> str:
    return _version_of(RECIPE_ID)


class PlanBuildFlowRecipeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(
            RECIPE_MATERIALIZE_PATH, "recipe_materialize_for_plan_build_flow"
        )
        cls.schema = load_module(
            RECIPE_SCHEMA_PATH, "recipe_schema_for_plan_build_flow"
        )

    def _make_project(self, extra_recipes: str = "") -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "skills").mkdir()
        (ai_specs / "commands").mkdir()
        manifest = ai_specs / "ai-specs.toml"
        version = _recipe_version()
        manifest.write_text(
            "[project]\nname = 'fixture'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            f'[recipes.{RECIPE_ID}]\nenabled = true\nversion = "{version}"\n'
            + extra_recipes
        )
        return root

    # -- AC1: exactly two commands, no third ---------------------------------

    def test_recipe_materializes_two_commands(self):
        recipe_dir = CATALOG / RECIPE_ID
        recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")
        self.assertEqual(recipe.id, RECIPE_ID)

        cmd_ids = sorted(c.id for c in recipe.commands)
        self.assertEqual(cmd_ids, ["build", "plan"])

        skill_ids = [(s.id, s.source) for s in recipe.skills]
        self.assertEqual(skill_ids, [(RECIPE_ID, "bundled")])

        root = self._make_project()
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)

        plan_cmd = root / "ai-specs" / "commands" / "plan.md"
        build_cmd = root / "ai-specs" / "commands" / "build.md"
        archive_cmd = root / "ai-specs" / "commands" / "archive.md"
        self.assertTrue(plan_cmd.is_file(), f"missing {plan_cmd}")
        self.assertTrue(build_cmd.is_file(), f"missing {build_cmd}")
        self.assertFalse(
            archive_cmd.exists(),
            "no third command file (e.g. archive.md) should be generated",
        )

        skill = (
            root / "ai-specs" / ".recipe" / RECIPE_ID
            / "skills" / RECIPE_ID / "SKILL.md"
        )
        self.assertTrue(skill.is_file(), f"missing bundled skill at {skill}")
        skill_content = skill.read_text()
        self.assertTrue(skill_content.strip(), "materialized SKILL.md must not be empty")
        source_skill = (
            CATALOG / RECIPE_ID / "skills" / RECIPE_ID / "SKILL.md"
        )
        self.assertEqual(
            skill_content,
            source_skill.read_text(),
            "materialized SKILL.md must match the source skill content",
        )

    # -- AC2: no new schema/materializer surface -----------------------------

    def test_recipe_adds_no_schema_surface(self):
        recipe_dir = CATALOG / RECIPE_ID
        recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")

        self.assertEqual(recipe.config_schema.fields, {}, "no [config.*] fields expected")

        capability_ids = [cap.id for cap in recipe.capabilities]
        self.assertIn(
            RECIPE_ID,
            capability_ids,
            f"expected [[capabilities]] with id '{RECIPE_ID}' declared",
        )

        hook_pairs = [(h.event, h.action) for h in recipe.hooks]
        self.assertEqual(
            hook_pairs,
            [("on-sync", "validate-config")],
            "only the on-sync validate-config hook is expected",
        )
        self.assertEqual(recipe.runtime_hooks, [], "no runtime hooks expected")

        with (recipe_dir / "recipe.toml").open("rb") as fh:
            data = tomllib.load(fh)

        identifiers: list[str] = [
            data["recipe"].get("id", ""),
            data["recipe"].get("name", ""),
        ]
        for cap in data.get("capabilities", []):
            identifiers.append(cap.get("id", ""))
        provides = data.get("provides", {})
        for cmd in provides.get("commands", []):
            identifiers.append(cmd.get("id", ""))
        for skill in provides.get("skills", []):
            identifiers.append(skill.get("id", ""))

        for ident in identifiers:
            lowered = ident.lower()
            for term in FORBIDDEN_TERMS:
                self.assertNotIn(
                    term,
                    lowered,
                    f"identifier '{ident}' must not contain forbidden term '{term}'",
                )

    # -- AC8: brief + README vocabulary clean --------------------------------

    def test_brief_and_readme_vocabulary_clean(self):
        recipe_dir = CATALOG / RECIPE_ID
        recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")

        brief = recipe.brief_fragments
        self.assertIsNotNone(brief, "recipe must declare [provides.brief]")

        forbidden = ("SDD", "OpenSpec", "spec-driven")
        all_texts: list[str] = []
        for section in ("workflow_rules", "useful_commands"):
            fragments = getattr(brief, section) or []
            all_texts.extend(fragment.text for fragment in fragments)

        self.assertTrue(all_texts, "expected at least one brief fragment")
        for text in all_texts:
            lowered = text.lower()
            for term in forbidden:
                self.assertNotIn(
                    term.lower(),
                    lowered,
                    f"brief text contains forbidden term '{term}': {text!r}",
                )

        root = self._make_project()
        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)

        readme = root / "ai-specs" / "recipes" / RECIPE_ID / "README.md"
        self.assertTrue(readme.is_file(), f"missing materialized README at {readme}")
        content = readme.read_text().lower()
        for term in forbidden:
            self.assertNotIn(
                term.lower(), content, f"README contains forbidden term '{term}'"
            )

    # -- AC9: worktree-flow cross-reference, no hard dependency --------------

    def test_build_brief_references_worktree_flow(self):
        recipe_dir = CATALOG / RECIPE_ID
        recipe = self.schema.load_recipe_toml(recipe_dir / "recipe.toml")

        brief = recipe.brief_fragments
        self.assertIsNotNone(brief)
        rules = [fragment.text for fragment in (brief.workflow_rules or [])]

        matches = [
            rule
            for rule in rules
            if "worktree" in rule.lower() and "build" in rule.lower()
        ]
        self.assertTrue(
            matches,
            f"expected a workflow_rules entry mentioning worktree + build, got: {rules}",
        )

        # No hard dependency: schema has no `requires` primitive; the closest
        # proxy is conflicts_with, which must not reference worktree-flow either.
        self.assertNotIn("worktree-flow", recipe.conflicts_with)

    # -- AC10: classic SDD commands/skills unaffected ------------------------

    def test_classic_sdd_commands_unchanged(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "skills").mkdir()
        (ai_specs / "commands").mkdir()
        manifest = ai_specs / "ai-specs.toml"

        tdd_version = _version_of("tdd-flow")
        worktree_version = _version_of("worktree-flow")
        manifest.write_text(
            "[project]\nname = 'fixture'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            f'[recipes.tdd-flow]\nenabled = true\nversion = "{tdd_version}"\n\n'
            f'[recipes.worktree-flow]\nenabled = true\nversion = "{worktree_version}"\n'
        )

        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)

        snapshot_dirs = [
            ai_specs / "commands",
            ai_specs / ".recipe" / "tdd-flow",
            ai_specs / ".recipe" / "worktree-flow",
        ]
        snapshot: dict[Path, bytes] = {}
        for directory in snapshot_dirs:
            if not directory.exists():
                continue
            for path in directory.rglob("*"):
                if path.is_file():
                    snapshot[path.relative_to(root)] = path.read_bytes()

        self.assertTrue(snapshot, "expected pre-existing files to snapshot")

        plan_build_version = _recipe_version()
        manifest.write_text(
            manifest.read_text()
            + f'\n[recipes.{RECIPE_ID}]\nenabled = true\nversion = "{plan_build_version}"\n'
        )

        self.assertEqual(self.mod.materialize_recipes(root, ROOT), 0)

        for rel_path, content in snapshot.items():
            full_path = root / rel_path
            self.assertTrue(
                full_path.is_file(),
                f"pre-existing file removed/renamed after enabling {RECIPE_ID}: {rel_path}",
            )
            self.assertEqual(
                full_path.read_bytes(),
                content,
                f"pre-existing file content changed after enabling {RECIPE_ID}: {rel_path}",
            )


if __name__ == "__main__":
    unittest.main()
