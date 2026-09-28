"""Black-box tdd-flow recipe tests: every behavioral test drives ``bin/ai-specs``.

No test may import ``lib/_internal`` modules. Assertions preserve the original
contract intents (schema declarations, materialized artifacts) through the CLI
process boundary:

- Recipe schema validity and declared primitives are observable via
  ``recipe add`` (validation + exact id + "The next sync will materialize:"
  plan) and via ``tomllib`` reads of the catalog recipe.toml.
- Materialization is observable via ``sync``: bundled skill under the
  per-project CLI cache, command in the cache commands dir, and the doc under
  the project harness tree.
"""
from __future__ import annotations

import shutil
import sys
import tempfile
import tomllib
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import cache_project_dir, invoke, isolated_home  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]
CATALOG = ROOT / "catalog" / "recipes"
RECIPE_ID = "tdd-flow"


def _make_home(base: Path) -> Path:
    """Isolated CLI install root with a REAL lib copy.

    sync/materialize derive cache and catalog roots from their own realpath,
    so a symlinked lib would resolve back into the repository and let the CLI
    touch repo cache state. A real copy keeps every lookup and write in temp.
    """
    home = isolated_home(base)
    (home / "lib").unlink()
    shutil.copytree(
        ROOT / "lib", home / "lib", symlinks=True,
        ignore=shutil.ignore_patterns("_vendor", "__pycache__"),
    )
    return home


def _make_manifest(root: Path, name: str = "fixture") -> None:
    """Minimal initialized project (manifest + harness dirs) in temp."""
    (root / "ai-specs" / "skills").mkdir(parents=True, exist_ok=True)
    (root / "ai-specs" / "commands").mkdir(parents=True, exist_ok=True)
    (root / "ai-specs" / "ai-specs.toml").write_text(
        f"[project]\nname = {name!r}\n\n[agents]\nenabled = ['claude']\n"
    )


class _CliFixtureMixin:
    """One shared isolated cli_home and temp project per test command sequence."""

    def setUp(self):
        tmp = tempfile.TemporaryDirectory(prefix="ai-specs-tdd-")
        self.addCleanup(tmp.cleanup)
        self.base = Path(tmp.name)
        self.home = _make_home(self.base)
        self.root = self.base / "proj"
        _make_manifest(self.root)

    def recipe_add(self, recipe_id: str):
        result = invoke(self.root, "recipe", "add", recipe_id, cli_home=self.home)
        self.assertEqual(
            result.returncode, 0,
            f"recipe add {recipe_id} failed: {result.stdout}{result.stderr}",
        )
        return result

    def sync(self):
        return invoke(self.root, "sync", cli_home=self.home)

    def cache(self) -> Path:
        return cache_project_dir(self.root, self.home)


class TddFlowRecipeTests(_CliFixtureMixin, unittest.TestCase):
    def test_recipe_validates_and_declares_capability(self):
        """Recipe validates through the CLI and declares its primitives.

        ``recipe add`` runs schema validation and adds by the exact id; the
        declared capability/skill/command/config shapes are read from the
        recipe's catalog declaration (recipe.toml).
        """
        result = self.recipe_add(RECIPE_ID)
        self.assertIn(
            f"Recipe '{RECIPE_ID}' added to the manifest.", result.stdout,
            "tdd-flow must pass schema validation and add by its exact id",
        )
        self.assertIn(
            "The next sync will materialize:", result.stdout,
            "recipe add must print the declared primitives",
        )
        self.assertIn(
            "- skills: tdd-flow", result.stdout,
            "bundled tdd-flow skill must be declared by the recipe",
        )
        self.assertIn(
            "- commands: tdd", result.stdout,
            "tdd command must be declared by the recipe",
        )
        raw = tomllib.loads((CATALOG / RECIPE_ID / "recipe.toml").read_text())
        self.assertEqual(raw["recipe"]["id"], RECIPE_ID)
        cap_ids = [c["id"] for c in raw["capabilities"]]
        self.assertIn("test-runner", cap_ids)
        skill_ids = [(s["id"], s["source"]) for s in raw["provides"]["skills"]]
        self.assertIn(("tdd-flow", "bundled"), skill_ids)
        cmd_ids = [c["id"] for c in raw["provides"]["commands"]]
        self.assertIn("tdd", cmd_ids)
        # test_command config is declared, optional, no default (project-specific)
        fields = raw["config"]
        self.assertIn("test_command", fields)
        self.assertFalse(fields["test_command"]["required"])
        self.assertEqual(fields["test_command"]["type"], "string")
        self.assertNotIn(
            "default", fields["test_command"],
            "test_command must not carry an invented default",
        )

    def test_materialize_produces_skill_command_and_doc(self):
        """Sync materializes the bundled skill, the command, and the doc."""
        self.recipe_add(RECIPE_ID)
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

        skill = (
            self.cache() / ".recipe" / RECIPE_ID
            / "skills" / "tdd-flow" / "SKILL.md"
        )
        self.assertTrue(skill.is_file(), f"missing bundled skill at {skill}")

        cmd = self.cache() / "commands" / "tdd.md"
        self.assertTrue(cmd.is_file(), f"missing command at {cmd}")

        doc = self.root / "ai-specs" / "recipes" / RECIPE_ID / "README.md"
        self.assertTrue(doc.is_file(), f"missing doc at {doc}")


if __name__ == "__main__":
    unittest.main()
