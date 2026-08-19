"""Black-box tests for recipe conflict handling during sync."""
from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

from _blackbox import isolated_home, invoke, snapshot, temp_project


class RecipeConflictTests(unittest.TestCase):
    def _project(self, recipes: dict[str, str]) -> tuple[Path, Path]:
        td, project = temp_project(name="conflict-fixture")
        self.addCleanup(td.cleanup)
        home = isolated_home(Path(td.name) / "cli-home-source")
        catalog_link = home / "catalog"
        catalog_link.unlink()
        catalog_link.mkdir()
        catalog = home / "catalog" / "recipes"
        catalog.mkdir()
        ids = []
        for rid, provides in recipes.items():
            ids.append(rid)
            recipe_dir = catalog / rid
            recipe_dir.mkdir()
            (recipe_dir / "recipe.toml").write_text(
                f"[recipe]\nid = '{rid}'\nname = '{rid.title()}'\n"
                "description = 'fixture'\nversion = '1.0'\n" + provides
            )
        entries = "\n".join(
            f"[recipes.{rid}]\nenabled = true\nversion = '1.0'\n" for rid in ids
        )
        (project / "ai-specs" / "ai-specs.toml").write_text(
            "[project]\nname = 'conflict-fixture'\n\n[agents]\nenabled = []\n\n" + entries
        )
        return project, home

    def _sync(self, recipes: dict[str, str]):
        project, home = self._project(recipes)
        before = snapshot(project)
        result = invoke(project, "sync", cli_home=home)
        return project, result, before

    def test_no_conflict_for_distinct_recipes(self):
        _, result, _ = self._sync({"distinct": ""})
        self.assertEqual(result.returncode, 0)
        self.assertNotIn("recipe conflict", result.stdout + result.stderr)

    def test_detects_skill_conflict(self):
        _, result, _ = self._sync({
            "conflict-a": "\n[provides]\nskills = [{ id = 'shared-skill', source = 'bundled' }]\n",
            "conflict-b": "\n[provides]\nskills = [{ id = 'shared-skill', source = 'bundled' }]\n",
        })
        self.assertEqual(result.returncode, 1)
        self.assertIn("recipe conflict", result.stderr)
        self.assertIn("shared-skill", result.stderr)

    def test_detects_command_conflict(self):
        _, result, _ = self._sync({
            "cmd-a": "\n[provides]\ncommands = [{ id = 'shared-cmd', path = 'commands/shared.md' }]\n",
            "cmd-b": "\n[provides]\ncommands = [{ id = 'shared-cmd', path = 'commands/shared.md' }]\n",
        })
        self.assertEqual(result.returncode, 1)
        self.assertIn("recipe conflict", result.stderr)
        self.assertIn("shared-cmd", result.stderr)

    def test_detects_mcp_conflict(self):
        _, result, _ = self._sync({
            "mcp-a": "\n[[provides.mcp]]\nid = 'shared-mcp'\ncommand = 'npx'\n",
            "mcp-b": "\n[[provides.mcp]]\nid = 'shared-mcp'\ncommand = 'npx'\n",
        })
        self.assertEqual(result.returncode, 1)
        self.assertIn("recipe conflict", result.stderr)
        self.assertIn("shared-mcp", result.stderr)

    def test_cli_exits_zero_when_no_conflict(self):
        project, result, before = self._sync({"single": ""})
        self.assertEqual(result.returncode, 0)
        self.assertIn("sync complete", result.stdout)
        self.assertNotEqual(snapshot(project), before)

    def test_cli_exits_one_when_conflict(self):
        project, result, before = self._sync({
            "skill-a": "\n[provides]\nskills = [{ id = 'shared-skill', source = 'bundled' }]\n",
            "skill-b": "\n[provides]\nskills = [{ id = 'shared-skill', source = 'bundled' }]\n",
        })
        self.assertEqual(result.returncode, 1)
        self.assertIn("shared-skill", result.stderr)
        self.assertIn("recipe conflict", result.stderr)
        self.assertNotEqual(snapshot(project), before)

    def test_capability_ambiguity_warning(self):
        _, result, _ = self._sync({"provider-a": "\ntags = ['tracker']\n", "provider-b": "\ntags = ['tracker']\n"})
        self.assertEqual(result.returncode, 0)
        self.assertIn("tag overlap", result.stderr)
        self.assertIn("tracker", result.stderr)

    def test_capability_explicit_binding_resolves_ambiguity(self):
        _, result, _ = self._sync({"provider-a": "\ntags = ['tracker']\n", "provider-b": "\ntags = ['tracker']\n"})
        self.assertEqual(result.returncode, 0)
        self.assertIn("sync complete", result.stdout)
        self.assertNotIn("recipe conflict", result.stderr)

    def test_capability_duplicate_explicit_fatal(self):
        _, result, _ = self._sync({
            "provider-a": "\n[provides]\nskills = [{ id = 'binding', source = 'bundled' }]\n",
            "provider-b": "\n[provides]\nskills = [{ id = 'binding', source = 'bundled' }]\n",
        })
        self.assertEqual(result.returncode, 1)
        self.assertIn("binding", result.stderr)
        self.assertIn("conflict", result.stderr)

    def test_capability_single_provider_no_conflict(self):
        _, result, _ = self._sync({"provider": "\ntags = ['tracker']\n"})
        self.assertEqual(result.returncode, 0)
        self.assertNotIn("tag overlap", result.stderr)

    def test_disabled_recipe_excluded(self):
        project, home = self._project({
            "provider-a": "\ntags = ['tracker']\n",
            "provider-b": "\ntags = ['tracker']\n",
        })
        manifest = project / "ai-specs" / "ai-specs.toml"
        manifest.write_text(manifest.read_text().replace(
            "[recipes.provider-b]\nenabled = true",
            "[recipes.provider-b]\nenabled = false",
        ))
        result = invoke(project, "sync", cli_home=home)
        self.assertEqual(result.returncode, 0)
        self.assertNotIn("tag overlap", result.stderr)
        self.assertIn("sync complete", result.stdout)

    def test_no_shared_tag_no_conflict(self):
        _, result, _ = self._sync({"a": "\ntags = ['vcs']\n", "b": "\ntags = ['tracker']\n"})
        self.assertEqual(result.returncode, 0)
        self.assertNotIn("tag overlap", result.stderr)

    def test_single_recipe_no_conflict(self):
        _, result, _ = self._sync({"a": "\ntags = ['vcs']\n"})
        self.assertEqual(result.returncode, 0)
        self.assertNotIn("tag overlap", result.stderr)

    def test_duplicate_tag_on_single_recipe_no_conflict(self):
        _, result, _ = self._sync({"a": "\ntags = ['vcs', 'vcs']\n"})
        self.assertEqual(result.returncode, 0)
        self.assertNotIn("tag overlap", result.stderr)

    def test_shared_tag_without_conflicts_with_is_warning(self):
        _, result, _ = self._sync({"a": "\ntags = ['vcs', 'github']\n", "b": "\ntags = ['vcs', 'gitlab']\n"})
        self.assertEqual(result.returncode, 0)
        self.assertIn("tag overlap", result.stderr)
        self.assertIn("vcs", result.stderr)

    def test_shared_tag_with_conflicts_with_is_fatal(self):
        _, result, _ = self._sync({"a": "\ntags = ['vcs']\nconflicts_with = ['b']\n", "b": "\ntags = ['vcs']\n"})
        self.assertEqual(result.returncode, 0)
        self.assertIn("tag conflict", result.stderr)
        self.assertIn("vcs", result.stderr)

    def test_conflicts_with_is_symmetric(self):
        _, result, _ = self._sync({"a": "\ntags = ['vcs']\n", "b": "\ntags = ['vcs']\nconflicts_with = ['a']\n"})
        self.assertEqual(result.returncode, 0)
        self.assertIn("tag conflict", result.stderr)
        self.assertIn("explicit conflicts_with", result.stderr)

    def test_to_dict_output_format(self):
        _, result, _ = self._sync({"a": "\ntags = ['vcs']\n", "b": "\ntags = ['vcs']\n"})
        self.assertEqual(result.returncode, 0)
        self.assertIn("tag overlap", result.stderr)
        self.assertIn("recipes a, b", result.stderr)

    def test_catalog_vcs_recipes_warn_when_enabled_together(self):
        _, result, _ = self._sync({"git-pr": "\ntags = ['vcs']\n", "bitbucket-pr": "\ntags = ['vcs']\n"})
        self.assertEqual(result.returncode, 0)
        self.assertIn("tag overlap", result.stderr)
        self.assertIn("vcs", result.stderr)


if __name__ == "__main__":
    unittest.main()
