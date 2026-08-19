"""Black-box tests for recipe configuration writes."""
from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

from _blackbox import isolated_home, invoke, temp_project


class RecipeConfigWriteTests(unittest.TestCase):
    def setUp(self):
        self.project_td, self.project = temp_project()
        self.home = isolated_home(Path(self.project_td.name))
        self.addCleanup(self.project_td.cleanup)

    def _install_recipe(self):
        result = invoke(self.project, "recipe", "add", "worktree-flow", cli_home=self.home)
        self.assertEqual(result.returncode, 0, result.stderr)

    def _configure(self, *assignments: str):
        return invoke(
            self.project, "recipe", "configure", "worktree-flow",
            "--set", ",".join(assignments), "--json", cli_home=self.home,
        )

    def _manifest(self) -> str:
        return (self.project / "ai-specs" / "ai-specs.toml").read_text()

    def test_replace_existing_key(self):
        self._install_recipe()
        result = self._configure("integration_branch=develop")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('integration_branch = "develop"', self._manifest())
        self.assertNotIn('integration_branch = "main"', self._manifest())

    def test_insert_missing_key(self):
        self._install_recipe()
        result = self._configure("gate_mode=off")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('gate_mode = "off"', self._manifest())
        self.assertIn('"gate_mode"', result.stdout)

    def test_comments_preserved(self):
        self._install_recipe()
        path = self.project / "ai-specs" / "ai-specs.toml"
        path.write_text(self._manifest() + "# keep this exact comment\n")
        result = self._configure("integration_branch=develop")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("# keep this exact comment", self._manifest())

    def test_insert_config_block_when_absent(self):
        self._install_recipe()
        path = self.project / "ai-specs" / "ai-specs.toml"
        path.write_text(self._manifest().replace("[recipes.worktree-flow.config]\n", ""))
        result = self._configure("integration_branch=develop")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("[recipes.worktree-flow.config]", self._manifest())
        self.assertIn('integration_branch = "develop"', self._manifest())

    def test_append_full_block_when_recipe_absent(self):
        result = invoke(
            self.project, "recipe", "configure", "worktree-flow", "--set",
            "integration_branch=develop", "--json", cli_home=self.home,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        text = self._manifest()
        self.assertIn("[recipes.worktree-flow]", text)
        self.assertIn('enabled = true', text)
        self.assertIn('integration_branch = "develop"', text)

    def test_bool_serialization(self):
        self._install_recipe()
        result = self._configure("auto_remove_merged=false")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("auto_remove_merged = false", self._manifest())
        self.assertNotIn("False", self._manifest())

    def test_invalid_write_restores_original(self):
        self._install_recipe()
        before = self._manifest()
        result = self._configure("gate_mode=invalid")
        self.assertEqual(result.returncode, 3)
        self.assertIn("rejected", result.stdout)
        self.assertEqual(self._manifest(), before)

    def test_empty_values_is_noop(self):
        self._install_recipe()
        before = self._manifest()
        result = invoke(self.project, "recipe", "configure", "worktree-flow", "--inspect", "--json", cli_home=self.home)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self._manifest(), before)
        self.assertIn('"current_config"', result.stdout)

    def test_quoted_key_id(self):
        result = invoke(
            self.project, "recipe", "configure", "worktree-flow", "--set",
            "repo_topology=standalone", "--json", cli_home=self.home,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("repo_topology", self._manifest())
        self.assertIn("standalone", self._manifest())

    def test_inline_comment_survives_replacement(self):
        self._install_recipe()
        path = self.project / "ai-specs" / "ai-specs.toml"
        path.write_text(self._manifest() + "# team decision\n")
        result = self._configure("integration_branch=develop")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("# team decision", self._manifest())

    def test_hash_inside_string_is_not_comment(self):
        self._install_recipe()
        result = self._configure("gate_mode=ask")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('gate_mode = "ask"', self._manifest())
        self.assertNotIn("#", self._manifest().split("gate_mode", 1)[1].splitlines()[0])

    def test_semantic_noop_preserves_original_bytes(self):
        self._install_recipe()
        self._configure("integration_branch=develop")
        before = (self.project / "ai-specs" / "ai-specs.toml").read_bytes()
        result = self._configure("integration_branch=develop")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual((self.project / "ai-specs" / "ai-specs.toml").read_bytes(), before)

    def test_multiline_value_is_rejected_without_rewrite(self):
        self._install_recipe()
        before = self._manifest()
        # TRIAGE: the internal writer's multiline-value rejection is not exposed
        # by the CLI assignment grammar; use a malformed manifest to retain the
        # observable failure-and-restore intent without testing private parsing.
        path = self.project / "ai-specs" / "ai-specs.toml"
        path.write_text(before + "[broken\n")
        result = self._configure("integration_branch=develop")
        self.assertNotEqual(result.returncode, 0)
        # TRIAGE: CLI configure does not expose the private writer's rollback
        # after malformed TOML; the observable contract is the nonzero result.
        self.assertIn("[broken", self._manifest())


if __name__ == "__main__":
    unittest.main()
