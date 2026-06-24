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


class TagConflictTests(unittest.TestCase):
    """Card #27 — RED: tag-based conflict detection across enabled recipes."""

    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(RECIPE_CONFLICTS_PATH, "recipe_conflicts_tags")
        cls.schema = load_module(
            ROOT / "lib" / "_internal" / "recipe_schema.py", "recipe_schema_for_tags"
        )

    def _recipe(self, rid, tags, conflicts_with=None):
        return self.schema.Recipe(
            id=rid,
            name=rid,
            description="d",
            version="1.0",
            tags=list(tags),
            conflicts_with=list(conflicts_with or []),
        )

    def test_no_shared_tag_no_conflict(self):
        recipes = [self._recipe("a", ["vcs"]), self._recipe("b", ["tracker"])]
        self.assertEqual(self.mod.check_tag_conflicts(recipes), [])

    def test_single_recipe_no_conflict(self):
        self.assertEqual(self.mod.check_tag_conflicts([self._recipe("a", ["vcs"])]), [])

    def test_duplicate_tag_on_single_recipe_no_conflict(self):
        # A recipe listing the same tag twice must not self-conflict.
        recipes = [self._recipe("a", ["vcs", "vcs"])]
        self.assertEqual(self.mod.check_tag_conflicts(recipes), [])

    def test_shared_tag_without_conflicts_with_is_warning(self):
        recipes = [self._recipe("a", ["vcs", "github"]), self._recipe("b", ["vcs", "gitlab"])]
        conflicts = self.mod.check_tag_conflicts(recipes)
        self.assertEqual(len(conflicts), 1)
        c = conflicts[0]
        self.assertEqual(c.severity, "warning")
        self.assertEqual(c.tag, "vcs")
        self.assertEqual(c.recipes, {"a", "b"})

    def test_shared_tag_with_conflicts_with_is_fatal(self):
        recipes = [
            self._recipe("a", ["vcs"], conflicts_with=["b"]),
            self._recipe("b", ["vcs"]),
        ]
        conflicts = self.mod.check_tag_conflicts(recipes)
        self.assertEqual(len(conflicts), 1)
        self.assertEqual(conflicts[0].severity, "fatal")
        self.assertEqual(conflicts[0].tag, "vcs")

    def test_conflicts_with_is_symmetric(self):
        # Only b declares the conflict, but it must still be fatal for the pair.
        recipes = [
            self._recipe("a", ["vcs"]),
            self._recipe("b", ["vcs"], conflicts_with=["a"]),
        ]
        conflicts = self.mod.check_tag_conflicts(recipes)
        self.assertEqual(len(conflicts), 1)
        self.assertEqual(conflicts[0].severity, "fatal")

    def test_to_dict_output_format(self):
        recipes = [self._recipe("a", ["vcs"]), self._recipe("b", ["vcs"])]
        c = self.mod.check_tag_conflicts(recipes)[0]
        d = c.to_dict()
        self.assertEqual(d["type"], "tag_conflict")
        self.assertEqual(d["tag"], "vcs")
        self.assertEqual(sorted(d["recipes"]), ["a", "b"])

    def test_catalog_vcs_recipes_warn_when_enabled_together(self):
        recipes = [
            self.mod.load_recipe_toml(CATALOG / rid / "recipe.toml")
            for rid in ("git-pr-flow", "bitbucket-pr-flow")
        ]
        conflicts = self.mod.check_tag_conflicts(recipes)
        vcs = [c for c in conflicts if c.tag == "vcs"]
        self.assertTrue(vcs, "expected a vcs tag conflict between two VCS recipes")


if __name__ == "__main__":
    unittest.main()
