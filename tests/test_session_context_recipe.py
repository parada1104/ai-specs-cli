"""Tests for the session-context catalog recipe."""
from __future__ import annotations

import importlib.util
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
RECIPE_SCHEMA_PATH = ROOT / "lib" / "_internal" / "recipe_schema.py"
RECIPE_MATERIALIZE_PATH = ROOT / "lib" / "_internal" / "recipe-materialize.py"
RECIPE_DIR = ROOT / "catalog" / "recipes" / "session-context"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


class SessionContextRecipeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.schema = load_module(RECIPE_SCHEMA_PATH, "recipe_schema_internal")
        cls.mat = load_module(RECIPE_MATERIALIZE_PATH, "recipe_materialize_internal")

    def _recipe_version(self) -> str:
        import tomllib

        with open(RECIPE_DIR / "recipe.toml", "rb") as fh:
            return tomllib.load(fh)["recipe"]["version"]

    def test_recipe_validates_and_declares_capabilities(self):
        recipe = self.schema.load_recipe_toml(RECIPE_DIR / "recipe.toml")
        self.assertEqual(recipe.id, "session-context")
        cap_ids = {c.id for c in recipe.capabilities}
        # Foundational recipe: provides the bootstrap + conflict patterns.
        # canonical-store moved out to the vault-canonical-store recipe.
        self.assertEqual(cap_ids, {"session-bootstrap", "conflict-policy"})
        skill_ids = {s.id for s in recipe.skills}
        self.assertEqual(skill_ids, {"session-bootstrap", "context-precedence"})
        for skill in recipe.skills:
            self.assertEqual(skill.source, "bundled")

    def test_bootstrap_skill_is_tool_agnostic(self):
        text = (
            RECIPE_DIR / "skills" / "session-bootstrap" / "SKILL.md"
        ).read_text()
        # Decoupled: refers to capabilities, not specific vendors.
        for vendor in ("Engram", "Trello", "Obsidian"):
            self.assertNotIn(vendor, text, f"session-bootstrap still names {vendor}")
        for capability in ("memory", "tracker", "canonical-store"):
            self.assertIn(capability, text)

    def test_materialize_produces_bundled_skills_and_doc(self):
        with tempfile.TemporaryDirectory() as tmp:
            project_root = Path(tmp) / "project"
            ai_specs = project_root / "ai-specs"
            ai_specs.mkdir(parents=True)
            (ai_specs / "skills").mkdir()
            (ai_specs / "commands").mkdir()
            version = self._recipe_version()
            (ai_specs / "ai-specs.toml").write_text(
                "[project]\nname = 'fixture'\n\n"
                "[agents]\nenabled = ['claude']\n\n"
                f"[recipes.session-context]\nenabled = true\nversion = \"{version}\"\n"
            )
            self.assertEqual(self.mat.materialize_recipes(project_root, ROOT), 0)

            base = project_root / "ai-specs" / ".recipe" / "session-context" / "skills"
            for skill_id in ("session-bootstrap", "context-precedence"):
                skill_md = base / skill_id / "SKILL.md"
                self.assertTrue(skill_md.is_file(), f"missing bundled skill {skill_id}")
            self.assertFalse(
                (base / "vault-context").exists(),
                "vault-context should no longer be bundled in session-context",
            )

            doc = project_root / "ai-specs" / "recipes" / "session-context" / "README.md"
            self.assertTrue(doc.is_file())


if __name__ == "__main__":
    unittest.main()
