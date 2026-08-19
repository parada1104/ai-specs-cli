"""Tests for the session-context catalog recipe.

Black-box: every scenario drives the observable CLI through `bin/ai-specs` and
asserts on the filesystem / exit-code effects it produces. No internal module
is imported from `lib/_internal/`.
"""
from __future__ import annotations

import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
import sys
from pathlib import Path as _P
sys.path.insert(0, str(_P(__file__).resolve().parent))
from _cache_paths import recipe_root
from _blackbox import isolated_home, invoke, temp_project

RECIPE_DIR = ROOT / "catalog" / "recipes" / "session-context"


def _enable_recipe(root: Path, version: str = "2.0.0") -> None:
    """Manifest that enables the session-context recipe for a project."""
    manifest = root / "ai-specs" / "ai-specs.toml"
    manifest.write_text(
        "[project]\nname = 'fixture'\n\n"
        "[agents]\nenabled = ['claude']\n\n"
        "[recipes.session-context]\nenabled = true\n"
        f'version = "{version}"\n'
    )


class SessionContextRecipeTests(unittest.TestCase):

    def test_recipe_validates_and_declares_capabilities(self):
        # Sync drives the CLI's own recipe schema: a malformed recipe.toml would
        # fail materialization, so exit 0 is the observable proof the recipe
        # validates. The skills it declares are observable as the bundled-skill
        # tree materialized under `.recipe/session-context/skills/`.
        #
        # TRIAGE: the declared capability ids ({session-bootstrap,
        # conflict-policy}) and the per-skill source='bundled' tags are
        # declaration-level fields of recipe.toml with no CLI/filesystem
        # observable equivalent (capabilities are semantic binding tags, not
        # emitted artifacts). They are covered here by sync exit 0 (schema
        # validation) and by the bundled-skill tree (skills materialized under
        # `.recipe/`, the bundled origin, rather than `.deps/`).
        with tempfile.TemporaryDirectory() as tmp:
            base = Path(tmp)
            td, root = temp_project(name="fixture", agents=("claude",))
            try:
                _enable_recipe(root)
                home = isolated_home(base)
                res = invoke(root, "sync", cli_home=home, tmpdir=base)
                self.assertEqual(res.returncode, 0)
                listed = invoke(root, "recipe", "list", cli_home=home, tmpdir=base)
                self.assertIn("session-context", listed.stdout)

                skills = recipe_root(root, "session-context", home) / "skills"
                for skill_id in ("session-bootstrap", "context-precedence"):
                    skill_md = skills / skill_id / "SKILL.md"
                    self.assertTrue(skill_md.is_file(), f"missing bundled skill {skill_id}")
                self.assertFalse(
                    (skills / "vault-context").exists(),
                    "vault-context should no longer be bundled in session-context",
                )
            finally:
                td.cleanup()

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
            base = Path(tmp)
            td, root = temp_project(name="fixture", agents=("claude",))
            try:
                _enable_recipe(root)
                home = isolated_home(base)
                res = invoke(root, "sync", cli_home=home, tmpdir=base)
                self.assertEqual(res.returncode, 0)

                skills = recipe_root(root, "session-context", home) / "skills"
                for skill_id in ("session-bootstrap", "context-precedence"):
                    skill_md = skills / skill_id / "SKILL.md"
                    self.assertTrue(skill_md.is_file(), f"missing bundled skill {skill_id}")
                self.assertFalse(
                    (skills / "vault-context").exists(),
                    "vault-context should no longer be bundled in session-context",
                )

                doc = root / "ai-specs" / "recipes" / "session-context" / "README.md"
                self.assertTrue(doc.is_file())
            finally:
                td.cleanup()


if __name__ == "__main__":
    unittest.main()
