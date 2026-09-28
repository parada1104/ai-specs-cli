"""Black-box session-context recipe tests: every behavioral test drives ``bin/ai-specs``.

No test may import ``lib/_internal`` modules. Assertions preserve the original
contract intents (schema declarations, tool-agnostic bootstrap skill,
materialized artifacts) through the CLI process boundary:

- Recipe schema validity and the exact capability/skill sets are observable
  via ``recipe add`` (validation + exact id + plan) and via ``tomllib`` reads
  of the catalog recipe.toml.
- The bootstrap skill's tool-agnosticism is a golden-content check on the
  catalog skill file.
- Materialization is observable via ``sync``: bundled skills under the
  per-project CLI cache and the doc under the project harness tree.
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
RECIPE_DIR = ROOT / "catalog" / "recipes" / "session-context"
RECIPE_ID = "session-context"


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
        tmp = tempfile.TemporaryDirectory(prefix="ai-specs-session-")
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


class SessionContextRecipeTests(_CliFixtureMixin, unittest.TestCase):
    def test_recipe_validates_and_declares_capabilities(self):
        """Recipe validates through the CLI and declares the foundational sets.

        ``recipe add`` runs schema validation and adds by the exact id; the
        exact capability/skill sets are read from the catalog declaration.
        """
        result = self.recipe_add(RECIPE_ID)
        self.assertIn(
            f"Recipe '{RECIPE_ID}' added to the manifest.", result.stdout,
            "session-context must pass schema validation and add by its exact id",
        )
        raw = tomllib.loads((RECIPE_DIR / "recipe.toml").read_text())
        self.assertEqual(raw["recipe"]["id"], RECIPE_ID)
        cap_ids = {c["id"] for c in raw["capabilities"]}
        # Foundational recipe: provides the bootstrap + conflict patterns.
        # canonical-store moved out to the vault-canonical-store recipe.
        self.assertEqual(cap_ids, {"session-bootstrap", "conflict-policy"})
        skill_ids = {s["id"] for s in raw["provides"]["skills"]}
        self.assertEqual(skill_ids, {"session-bootstrap", "context-precedence"})
        for skill in raw["provides"]["skills"]:
            self.assertEqual(skill["source"], "bundled")

    def test_bootstrap_skill_is_tool_agnostic(self):
        """The bootstrap skill refers to capabilities, not specific vendors."""
        text = (
            RECIPE_DIR / "skills" / "session-bootstrap" / "SKILL.md"
        ).read_text()
        # Decoupled: refers to capabilities, not specific vendors.
        for vendor in ("Engram", "Trello", "Obsidian"):
            self.assertNotIn(vendor, text, f"session-bootstrap still names {vendor}")
        for capability in ("memory", "tracker", "canonical-store"):
            self.assertIn(capability, text)

    def test_materialize_produces_bundled_skills_and_doc(self):
        """Sync materializes both bundled skills (and no vault-context) plus the doc."""
        self.recipe_add(RECIPE_ID)
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

        base = self.cache() / ".recipe" / RECIPE_ID / "skills"
        for skill_id in ("session-bootstrap", "context-precedence"):
            skill_md = base / skill_id / "SKILL.md"
            self.assertTrue(skill_md.is_file(), f"missing bundled skill {skill_md}")
        self.assertFalse(
            (base / "vault-context").exists(),
            "vault-context should no longer be bundled in session-context",
        )

        doc = self.root / "ai-specs" / "recipes" / RECIPE_ID / "README.md"
        self.assertTrue(doc.is_file(), f"missing doc at {doc}")


if __name__ == "__main__":
    unittest.main()
