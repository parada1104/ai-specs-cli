"""Focused parity matrix for recipe user entry points.

A consumer can reach a recipe through several user-facing surfaces: the
``ai-specs recipe`` dispatcher, the non-interactive ``recipe configure``
helper, ``sync``/``doctor``, the top-level CLI help, and the interactive Hub.
This module freezes the contract that those surfaces resolve the *same*
catalog, manifest state, and materialization for the standalone Jinna recipe,
and pins the two user-facing labels that had drifted from the underlying
command:

* ``ai-specs --help`` must advertise ``recipe configure`` because the recipe
  dispatcher already implements it;
* the Hub Recipes submenu "configure" entry must describe the whole-project
  ``configure-recipes`` action it actually delegates to.

Everything runs network-free against a temporary consumer whose
``AI_SPECS_HOME`` symlinks back to this worktree (reusing the Jinna consumer
fixture constants), and the repository dogfood manifest is asserted unchanged.
"""
from __future__ import annotations

import importlib.util
import json
import os
import re
import subprocess
import sys
import tempfile
import tomllib
import unittest
from pathlib import Path


sys.path.insert(0, str(Path(__file__).resolve().parent))
from _cache_paths import resolved_skills_dir
from test_jinna_consumer_recipe import (
    CLI,
    CLI_HOME_LINKS,
    DOGFOOD_MANIFEST,
    FAKE_JINNA_SCRIPT,
    JINNA_SKILL_ID,
    OPENPROJECT_ENV,
    RECIPE_ID,
    ROOT,
)

HUB_PY = ROOT / "lib" / "_internal" / "hub.py"

# ``recipe-list.sh`` prints ``[status]  id  version  name``.
_LIST_ROW_RE = re.compile(
    r"^\[(?P<status>\S+)\s*\]\s+(?P<id>\S+)\s+(?P<version>\S+)\s+(?P<name>.+?)\s*$"
)
# ``recipe init`` prints ``- Key: value`` bullets.
_INIT_FIELD_RE = re.compile(r"^- (?P<key>[A-Za-z ]+):\s*(?P<value>.*?)\s*$", re.MULTILINE)


def _parse_recipe_list(output: str) -> dict[str, dict[str, str]]:
    rows: dict[str, dict[str, str]] = {}
    for line in output.splitlines():
        match = _LIST_ROW_RE.match(line)
        if match:
            rows[match.group("id")] = match.groupdict()
    return rows


def _parse_init_brief(output: str) -> dict[str, str]:
    return {m.group("key").strip(): m.group("value") for m in _INIT_FIELD_RE.finditer(output)}


def _load_hub():
    spec = importlib.util.spec_from_file_location("hub_entrypoint_parity", HUB_PY)
    assert spec is not None and spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


class RecipeEntrypointLifecycleParityTests(unittest.TestCase):
    """One isolated consumer exercises every real recipe CLI path."""

    @classmethod
    def setUpClass(cls):
        cls.dogfood_before = DOGFOOD_MANIFEST.read_bytes()
        cls.tmp = tempfile.TemporaryDirectory(prefix="recipe-entrypoint-parity-")
        cls.addClassCleanup(cls.tmp.cleanup)
        tmp = Path(cls.tmp.name)
        cls.consumer = tmp / "consumer"
        cls.consumer.mkdir()
        assert not cls.consumer.is_relative_to(ROOT), "consumer must live outside the repository"

        cls.cli_home = tmp / "cli-home"
        cls.cli_home.mkdir()
        for name in CLI_HOME_LINKS:
            os.symlink(ROOT / name, cls.cli_home / name, target_is_directory=True)

        bin_dir = tmp / "bin"
        bin_dir.mkdir()
        cls.invocation_log = tmp / "jinna-invocations.log"
        fake_jinna = bin_dir / "jinna"
        fake_jinna.write_text(FAKE_JINNA_SCRIPT, encoding="utf-8")
        fake_jinna.chmod(0o755)

        env = dict(os.environ)
        env.update(OPENPROJECT_ENV)
        env["AI_SPECS_HOME"] = str(cls.cli_home)
        env["JINNA_FAKE_LOG"] = str(cls.invocation_log)
        env["PATH"] = f"{bin_dir}{os.pathsep}{env.get('PATH', '')}"
        cls.env = env

        cls.outputs: dict[str, subprocess.CompletedProcess] = {}
        cls._run("init", ["init", "--no-tui"])
        cls._run("list_before", ["recipe", "list"])
        cls._run("init_before", ["recipe", "init", RECIPE_ID])
        cls._run("inspect_before", ["recipe", "configure", RECIPE_ID, "--inspect", "--json"])
        cls._run("add", ["recipe", "add", RECIPE_ID])
        cls._run("list_after", ["recipe", "list"])
        cls._run("init_after", ["recipe", "init", RECIPE_ID])
        cls._run("inspect_after", ["recipe", "configure", RECIPE_ID, "--inspect", "--json"])
        cls._run("sync", ["sync"])
        cls._run("doctor", ["doctor"])
        cls._run("inspect_after_sync", ["recipe", "configure", RECIPE_ID, "--inspect", "--json"])

    @classmethod
    def _run(cls, key: str, args: list[str]) -> subprocess.CompletedProcess:
        result = subprocess.run(
            [str(CLI), *args],
            cwd=cls.consumer,
            env=cls.env,
            capture_output=True,
            text=True,
            timeout=300,
        )
        cls.outputs[key] = result
        if result.returncode != 0:
            raise AssertionError(
                f"ai-specs {' '.join(args)} exited {result.returncode}\n"
                f"--- stdout ---\n{result.stdout}\n--- stderr ---\n{result.stderr}"
            )
        return result

    def _inspect(self, key: str) -> dict:
        return json.loads(self.outputs[key].stdout)

    def _rows(self, key: str) -> dict[str, dict[str, str]]:
        return _parse_recipe_list(self.outputs[key].stdout)

    def _brief(self, key: str) -> dict[str, str]:
        return _parse_init_brief(self.outputs[key].stdout)

    def test_catalog_metadata_converges_across_list_init_and_configure(self):
        row = self._rows("list_before")[RECIPE_ID]
        brief = self._brief("init_before")
        inspect = self._inspect("inspect_before")

        self.assertEqual(row["id"], RECIPE_ID)
        self.assertEqual(brief["ID"], RECIPE_ID)
        self.assertEqual(inspect["recipe"]["id"], RECIPE_ID)
        # list and init resolve the same catalog name/version.
        self.assertEqual(row["version"], brief["Version"])
        self.assertEqual(row["name"], brief["Name"])
        # configure resolves the same recipe schema/init contract from the catalog.
        self.assertTrue(inspect["grounding"]["init"]["present"])
        self.assertEqual(inspect["grounding"]["init"]["needs_mcp"], ["jinna"])
        self.assertTrue(inspect["grounding"]["mcp"]["required"])

    def test_manifest_state_converges_across_entrypoints(self):
        before_row = self._rows("list_before")[RECIPE_ID]
        after_row = self._rows("list_after")[RECIPE_ID]
        before_brief = self._brief("init_before")
        after_brief = self._brief("init_after")
        before_inspect = self._inspect("inspect_before")
        after_inspect = self._inspect("inspect_after")

        self.assertEqual(before_row["status"], "available")
        self.assertEqual(before_brief["Install state"], "available (not installed)")
        self.assertFalse(before_inspect["recipe"]["present_in_manifest"])
        self.assertFalse(before_inspect["recipe"]["enabled"])

        self.assertEqual(after_row["status"], "installed")
        self.assertEqual(after_brief["Install state"], "installed")
        self.assertTrue(after_inspect["recipe"]["present_in_manifest"])
        self.assertTrue(after_inspect["recipe"]["enabled"])

        manifest = tomllib.loads(
            (self.consumer / "ai-specs" / "ai-specs.toml").read_text(encoding="utf-8")
        )
        self.assertTrue(manifest["recipes"][RECIPE_ID]["enabled"])

    def test_add_sync_and_configure_agree_on_materialized_primitives(self):
        add_out = self.outputs["add"].stdout
        self.assertIn("skills: jinna-mcp-recipe", add_out)
        self.assertIn("mcp: jinna", add_out)
        self.assertIn("doc: README.md", add_out)
        self.assertIn(f"ai-specs/recipes/{RECIPE_ID}/README.md", add_out)

        # sync materialized the primitives ``recipe add`` announced.
        readme = self.consumer / "ai-specs" / "recipes" / RECIPE_ID / "README.md"
        self.assertTrue(readme.is_file() and readme.read_text(encoding="utf-8").strip())
        skill = (
            resolved_skills_dir(self.consumer, cli_home=self.cli_home)
            / JINNA_SKILL_ID
            / "SKILL.md"
        )
        self.assertTrue(skill.is_file())

        # configure grounding agrees with the materialized MCP environment surface.
        inspect = self._inspect("inspect_after_sync")
        mcp_env = json.loads(
            (self.consumer / ".mcp.json").read_text(encoding="utf-8")
        )["mcpServers"]["jinna"]["env"]
        self.assertEqual(sorted(mcp_env), inspect["grounding"]["mcp"]["env_vars"])
        self.assertEqual(sorted(mcp_env), sorted(OPENPROJECT_ENV))

        # inspect is read-only: manifest state is identical before and after sync.
        self.assertEqual(inspect["recipe"], self._inspect("inspect_after")["recipe"])

    def test_doctor_agrees_the_shared_lifecycle_is_healthy(self):
        doctor = self.outputs["doctor"]
        self.assertEqual(doctor.returncode, 0)
        self.assertIn("Summary:", doctor.stdout)
        self.assertIn("0 ERROR", doctor.stdout)

    def test_entrypoints_keep_the_provider_passive(self):
        logged = (
            self.invocation_log.read_text(encoding="utf-8")
            if self.invocation_log.exists()
            else ""
        )
        invocations = logged.split()
        self.assertTrue(invocations, "the fake jinna version check never ran")
        self.assertEqual(set(invocations), {"version"})

    def test_isolated_consumer_never_mutates_the_dogfood_manifest(self):
        self.assertEqual(DOGFOOD_MANIFEST.read_bytes(), self.dogfood_before)


class RecipeEntrypointSurfaceContractTests(unittest.TestCase):
    """Help, dispatch, and label contract for the non-interactive surfaces."""

    def test_top_level_help_advertises_recipe_configure(self):
        result = subprocess.run(
            [str(CLI), "--help"], capture_output=True, text=True, check=False
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("recipe configure", result.stdout)

    def test_recipe_dispatcher_help_advertises_configure(self):
        result = subprocess.run(
            [str(CLI), "recipe", "--help"], capture_output=True, text=True, check=False
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("configure", result.stdout)

    def test_configure_recipes_help_and_dispatch_are_non_interactive(self):
        help_result = subprocess.run(
            [str(CLI), "configure-recipes", "--help"],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(help_result.returncode, 0, help_result.stderr)
        self.assertIn("Usage: ai-specs configure-recipes", help_result.stdout)

        bad_flag = subprocess.run(
            [str(CLI), "configure-recipes", "--not-a-flag"],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(bad_flag.returncode, 2)
        self.assertIn("unknown flag", bad_flag.stderr)

    def test_hub_help_and_dispatch_are_non_interactive(self):
        help_result = subprocess.run(
            [str(CLI), "hub", "--help"], capture_output=True, text=True, check=False
        )
        self.assertEqual(help_result.returncode, 0, help_result.stderr)
        self.assertIn("Usage: ai-specs hub", help_result.stdout)

        bad_flag = subprocess.run(
            [str(CLI), "hub", "--not-a-flag"],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(bad_flag.returncode, 2)
        self.assertIn("unknown flag", bad_flag.stderr)

    def test_hub_recipes_submenu_configure_label_names_the_whole_project_action(self):
        hub = _load_hub()
        choices = {value: label for label, value in hub.recipes_submenu_choices()}
        self.assertIn("configure", choices)
        label = choices["configure"]
        self.assertIn("configure-recipes", label)
        self.assertIn("whole project", label.lower())

        # The Recipes menu must not advertise a per-catalog-recipe configure it
        # does not offer; that action lives in its own whole-project entry.
        recipes_menu = next(
            desc for action, _title, desc in hub._MENU if action is hub.Action.RECIPES
        )
        self.assertNotIn("configure", recipes_menu.lower())


if __name__ == "__main__":
    unittest.main()
