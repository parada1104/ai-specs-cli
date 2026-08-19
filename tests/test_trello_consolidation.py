"""Tests for folding the PM contract into the trello-mcp-workflow recipe.

The legacy dogfood skill `trello-pm-workflow` is being absorbed into the
recipe's bundled skill. After consolidation:
  - the recipe's bundled SKILL.md carries the card/PM contract,
  - card templates no longer reference an external dogfood skill path,
  - a `card-decision` template exists and materializes,
  - no catalog recipe template points at `ai-specs/skills/**` (no dangling refs).
"""

import sys
import tempfile
import tomllib
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
RECIPE_DIR = ROOT / "catalog" / "recipes" / "trello-mcp-workflow"
CATALOG_RECIPES = ROOT / "catalog" / "recipes"

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import invoke, isolated_home


class TrelloConsolidationTests(unittest.TestCase):
    def test_card_feature_has_no_external_skill_path(self):
        text = (RECIPE_DIR / "templates" / "card-feature.md").read_text()
        self.assertNotIn("ai-specs/skills/trello-pm-workflow", text)

    def test_bundled_skill_carries_pm_contract(self):
        text = (
            RECIPE_DIR / "skills" / "trello-mcp-workflow" / "SKILL.md"
        ).read_text()
        # Card contract markers folded in from the legacy dogfood skill.
        self.assertIn("Card Contract", text)
        self.assertIn("Acceptance Criteria", text)
        # Card types including the previously-missing `decision` type.
        for card_type in ("feature", "bug", "spike", "epic", "handoff", "decision"):
            self.assertIn(card_type, text)

    def test_no_recipe_template_references_dogfood_skill_path(self):
        offenders = []
        for tpl in CATALOG_RECIPES.rglob("templates/*.md"):
            if "ai-specs/skills/" in tpl.read_text():
                offenders.append(str(tpl.relative_to(ROOT)))
        self.assertEqual(offenders, [], f"templates with dangling skill paths: {offenders}")

    def _cli_home(self):
        td = tempfile.TemporaryDirectory(prefix="trello-home-")
        self.addCleanup(td.cleanup)
        return isolated_home(Path(td.name))

    def _recipe_version(self) -> str:
        with open(RECIPE_DIR / "recipe.toml", "rb") as fh:
            return tomllib.load(fh)["recipe"]["version"]

    def _make_project(self) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "skills").mkdir()
        (ai_specs / "commands").mkdir()
        version = self._recipe_version()
        (ai_specs / "ai-specs.toml").write_text(
            "[project]\nname = 'fixture'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            f'[recipes.trello-mcp-workflow]\nenabled = true\nversion = "{version}"\n'
            "[recipes.trello-mcp-workflow.config]\n"
            "board_id = '69ec097f13e2d38ecd89a557'\n"
        )
        return root

    def test_card_decision_template_materializes(self):
        root = self._make_project()
        home = self._cli_home()
        result = invoke(root, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0, result.stderr)
        dest = (
            root / "ai-specs" / "recipes" / "trello-mcp-workflow"
            / "overrides" / "templates" / "card-decision.md"
        )
        self.assertTrue(dest.is_file(), "card-decision template should materialize")


if __name__ == "__main__":
    unittest.main()
