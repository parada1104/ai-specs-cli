import importlib.util
import sys
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
RECIPE_CONFLICTS_PATH = ROOT / "lib" / "_internal" / "recipe-conflicts.py"
CATALOG = ROOT / "catalog" / "recipes"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


class RecipeConflictTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(RECIPE_CONFLICTS_PATH, "recipe_conflicts_internal")

    def test_no_conflict_for_distinct_recipes(self):
        conflicts = self.mod.check_recipe_conflicts(CATALOG, ["test-fixture"])
        self.assertEqual(conflicts, [])

    def test_detects_skill_conflict(self):
        conflicts = self.mod.check_recipe_conflicts(CATALOG, ["test-conflict-a", "test-conflict-b"])
        self.assertEqual(len(conflicts), 1)
        c = conflicts[0]
        self.assertEqual(c.primitive_type, "skill")
        self.assertEqual(c.primitive_id, "shared-skill")
        self.assertEqual(c.recipes, {"Test Conflict A", "Test Conflict B"})

    def test_detects_command_conflict(self):
        conflicts = self.mod.check_recipe_conflicts(CATALOG, ["test-cmd-conflict-a", "test-cmd-conflict-b"])
        self.assertEqual(len(conflicts), 1)
        c = conflicts[0]
        self.assertEqual(c.primitive_type, "command")
        self.assertEqual(c.primitive_id, "shared-cmd")

    def test_detects_mcp_conflict(self):
        conflicts = self.mod.check_recipe_conflicts(CATALOG, ["test-mcp-conflict-a", "test-mcp-conflict-b"])
        self.assertEqual(len(conflicts), 1)
        c = conflicts[0]
        self.assertEqual(c.primitive_type, "mcp")
        self.assertEqual(c.primitive_id, "shared-mcp")

    def test_cli_exits_zero_when_no_conflict(self):
        import subprocess
        proc = subprocess.run(
            ["python3", str(RECIPE_CONFLICTS_PATH), str(CATALOG), "test-fixture"],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(proc.returncode, 0)

    def test_cli_exits_one_when_conflict(self):
        import subprocess
        proc = subprocess.run(
            ["python3", str(RECIPE_CONFLICTS_PATH), str(CATALOG), "test-conflict-a", "test-conflict-b"],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(proc.returncode, 1)
        self.assertIn("shared-skill", proc.stderr)


if __name__ == "__main__":
    unittest.main()
