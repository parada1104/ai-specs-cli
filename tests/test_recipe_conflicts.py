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

    # --- V2 capability conflict tests ---------------------------------------

    def _make_cap_recipe(self, tmp: str, rid: str, caps: list[str]):
        recipe_dir = Path(tmp) / rid
        recipe_dir.mkdir()
        cap_lines = "".join(f'[[capabilities]]\nid = "{c}"\n' for c in caps)
        (recipe_dir / "recipe.toml").write_text(
            f'[recipe]\nid = "{rid}"\nname = "{rid.title()}"\ndescription = "D"\nversion = "1.0"\n'
            + cap_lines
        )

    def test_capability_ambiguity_warning(self):
        import tempfile
        with tempfile.TemporaryDirectory() as tmp:
            catalog = Path(tmp)
            self._make_cap_recipe(tmp, "recipe-a", ["tracker"])
            self._make_cap_recipe(tmp, "recipe-b", ["tracker"])
            conflicts = self.mod.check_capability_conflicts(catalog, ["recipe-a", "recipe-b"], [])
            self.assertEqual(len(conflicts), 1)
            self.assertEqual(conflicts[0].severity, "warning")
            self.assertEqual(conflicts[0].primitive_id, "tracker")

    def test_capability_explicit_binding_resolves_ambiguity(self):
        import tempfile
        with tempfile.TemporaryDirectory() as tmp:
            catalog = Path(tmp)
            self._make_cap_recipe(tmp, "recipe-a", ["tracker"])
            self._make_cap_recipe(tmp, "recipe-b", ["tracker"])
            bindings = [{"capability": "tracker", "recipe": "recipe-a"}]
            conflicts = self.mod.check_capability_conflicts(catalog, ["recipe-a", "recipe-b"], bindings)
            self.assertEqual(conflicts, [])

    def test_capability_duplicate_explicit_fatal(self):
        import tempfile
        with tempfile.TemporaryDirectory() as tmp:
            catalog = Path(tmp)
            self._make_cap_recipe(tmp, "recipe-a", ["tracker"])
            self._make_cap_recipe(tmp, "recipe-b", ["tracker"])
            bindings = [
                {"capability": "tracker", "recipe": "recipe-a"},
                {"capability": "tracker", "recipe": "recipe-b"},
            ]
            conflicts = self.mod.check_capability_conflicts(catalog, ["recipe-a", "recipe-b"], bindings)
            self.assertEqual(len(conflicts), 1)
            self.assertEqual(conflicts[0].severity, "fatal")

    def test_capability_single_provider_no_conflict(self):
        import tempfile
        with tempfile.TemporaryDirectory() as tmp:
            catalog = Path(tmp)
            self._make_cap_recipe(tmp, "recipe-a", ["tracker"])
            conflicts = self.mod.check_capability_conflicts(catalog, ["recipe-a"], [])
            self.assertEqual(conflicts, [])

    def test_capability_disabled_excluded(self):
        import tempfile
        with tempfile.TemporaryDirectory() as tmp:
            catalog = Path(tmp)
            self._make_cap_recipe(tmp, "recipe-a", ["tracker"])
            self._make_cap_recipe(tmp, "recipe-b", ["tracker"])
            conflicts = self.mod.check_capability_conflicts(catalog, ["recipe-a"], [])
            self.assertEqual(conflicts, [])


if __name__ == "__main__":
    unittest.main()
