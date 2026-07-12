"""Tests for dep_check.py CLI dependency checking."""
from __future__ import annotations

import importlib.util
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
DEP_CHECK_PATH = ROOT / "lib" / "_internal" / "dep_check.py"
RECIPE_SCHEMA_PATH = ROOT / "lib" / "_internal" / "recipe_schema.py"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


class DepCheckTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.schema = load_module(RECIPE_SCHEMA_PATH, "recipe_schema_depcheck")
        cls.mod = load_module(DEP_CHECK_PATH, "dep_check_internal")

    def _recipe(self, *deps):
        return self.schema.Recipe(
            id="demo",
            name="Demo",
            description="D",
            version="1.0",
            cli_deps=list(deps),
        )

    def _dep(self, **kwargs):
        defaults = {
            "binary": "gh",
            "purpose": "PRs",
            "required": True,
            "install_url": "https://cli.github.com/",
            "version_check": "",
            "min_version": "",
        }
        defaults.update(kwargs)
        return self.schema.CliDep(**defaults)

    def test_found_binary_ok(self):
        recipe = self._recipe(self._dep())
        with patch.object(self.mod, "_which", return_value=True):
            results = self.mod.check_cli_deps(recipe)
        self.assertEqual(len(results), 1)
        self.assertTrue(results[0].found)
        self.assertTrue(results[0].ok)

    def test_missing_binary_not_ok(self):
        recipe = self._recipe(self._dep())
        with patch.object(self.mod, "_which", return_value=False):
            results = self.mod.check_cli_deps(recipe)
        self.assertFalse(results[0].found)
        self.assertFalse(results[0].ok)

    def test_version_meets_min(self):
        recipe = self._recipe(
            self._dep(version_check="gh --version", min_version="2.0.0")
        )
        with patch.object(self.mod, "_which", return_value=True), patch.object(
            self.mod, "_run_version_check", return_value="gh 2.40.0"
        ):
            results = self.mod.check_cli_deps(recipe)
        self.assertTrue(results[0].ok)
        self.assertEqual(results[0].version, "2.40.0")

    def test_version_below_min(self):
        recipe = self._recipe(
            self._dep(version_check="gh --version", min_version="2.0.0")
        )
        with patch.object(self.mod, "_which", return_value=True), patch.object(
            self.mod, "_run_version_check", return_value="gh 1.9.0"
        ):
            results = self.mod.check_cli_deps(recipe)
        self.assertFalse(results[0].ok)
        self.assertIn("1.9.0", results[0].detail)
        self.assertIn("2.0.0", results[0].detail)

    def test_unparseable_version_does_not_block(self):
        recipe = self._recipe(
            self._dep(version_check="gh --version", min_version="2.0.0")
        )
        with patch.object(self.mod, "_which", return_value=True), patch.object(
            self.mod, "_run_version_check", return_value="weird"
        ):
            results = self.mod.check_cli_deps(recipe)
        self.assertTrue(results[0].found)
        self.assertTrue(results[0].ok)
        self.assertIn("unknown", results[0].detail)

    def test_optional_missing_not_failure(self):
        recipe = self._recipe(self._dep(required=False))
        with patch.object(self.mod, "_which", return_value=False):
            results = self.mod.check_cli_deps(recipe)
        self.assertFalse(results[0].ok)
        self.assertIs(results[0].required, False)

    def test_version_check_subprocess_error_degrades(self):
        recipe = self._recipe(
            self._dep(version_check="gh --version", min_version="2.0.0")
        )

        def boom(_cmd):
            raise RuntimeError("boom")

        with patch.object(self.mod, "_which", return_value=True), patch.object(
            self.mod, "_run_version_check", side_effect=boom
        ):
            results = self.mod.check_cli_deps(recipe)
        self.assertEqual(results[0].version, "")
        self.assertTrue(results[0].ok)  # unparseable / empty does not block

    def test_check_project_deps_aggregates(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            catalog = root / "catalog" / "recipes"
            (catalog / "alpha").mkdir(parents=True)
            (catalog / "beta").mkdir(parents=True)
            (catalog / "alpha" / "recipe.toml").write_text(
                "[recipe]\n"
                'id = "alpha"\n'
                'name = "Alpha"\n'
                'description = "D"\n'
                'version = "1.0"\n'
                "\n"
                "[[deps.cli]]\n"
                'binary = "tool-a"\n'
                'purpose = "A"\n'
            )
            (catalog / "beta" / "recipe.toml").write_text(
                "[recipe]\n"
                'id = "beta"\n'
                'name = "Beta"\n'
                'description = "D"\n'
                'version = "1.0"\n'
                "\n"
                "[[deps.cli]]\n"
                'binary = "tool-b"\n'
                'purpose = "B"\n'
            )
            project = root / "project"
            (project / "ai-specs").mkdir(parents=True)
            (project / "ai-specs" / "ai-specs.toml").write_text(
                '[project]\nname = "p"\n\n'
                "[recipes.alpha]\nenabled = true\nversion = \"1.0\"\n\n"
                "[recipes.beta]\nenabled = true\nversion = \"1.0\"\n"
            )
            with patch.dict("os.environ", {"AI_SPECS_HOME": str(root)}), patch.object(
                self.mod, "_which", return_value=True
            ):
                results = self.mod.check_project_deps(project)
            binaries = sorted((r.recipe_id, r.binary) for r in results)
            self.assertEqual(binaries, [("alpha", "tool-a"), ("beta", "tool-b")])
            self.assertTrue(all(r.recipe_id for r in results))

    def test_version_ge(self):
        self.assertTrue(self.mod._version_ge((2, 0), (2, 0, 0)))
        self.assertTrue(self.mod._version_ge((10,), (9,)))
        self.assertFalse(self.mod._version_ge((2, 0), (2, 1)))

    def test_parse_version(self):
        self.assertEqual(self.mod._parse_version("nope"), ())
        self.assertEqual(self.mod._parse_version("gh 2.40.0"), (2, 40, 0))


if __name__ == "__main__":
    unittest.main()
