"""Tests for playwright-ui-flow + playwright-mcp catalog recipes.

Black-box conversions drive the real CLI (``ai-specs sync`` /
``recipe configure --inspect``) against isolated projects and assert on the
materialized artifacts (skills, commands, docs, AGENTS brief fragments) plus
the static catalog recipe.toml files.
"""

from __future__ import annotations

import json
import sys
import tempfile
import tomllib
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
CATALOG = ROOT / "catalog" / "recipes"
BASE_ID = "playwright-ui-flow"
MCP_ID = "playwright-mcp"
CAPABILITIES_DOC = ROOT / "docs" / "capabilities.md"
CATALOG_DOC = ROOT / "docs" / "recipes-catalog.md"
sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import isolated_home, invoke, temp_project  # noqa: E402
from _cache_paths import cache_command, recipe_skill_dir  # noqa: E402


def _recipe_toml(recipe_id: str) -> dict:
    with (CATALOG / recipe_id / "recipe.toml").open("rb") as fh:
        return tomllib.load(fh)


def _version(recipe_id: str) -> str:
    return _recipe_toml(recipe_id)["recipe"]["version"]


def _enable_recipes(root: Path, *recipe_ids: str) -> Path:
    """Append the given recipes to an isolated project manifest (version pinned)."""
    manifest = root / "ai-specs" / "ai-specs.toml"
    for rid in recipe_ids:
        with manifest.open("a", encoding="utf-8") as fh:
            fh.write(
                f'\n[recipes.{rid}]\nenabled = true\nversion = "{_version(rid)}"\n'
            )
    return root


class PlaywrightUiFlowRecipeTests(unittest.TestCase):
    def setUp(self):
        self.home_td = tempfile.TemporaryDirectory(prefix="pwui-home-")
        self.addCleanup(self.home_td.cleanup)
        self.home = isolated_home(Path(self.home_td.name))

    def _invoke(self, root: Path, *args: str):
        return invoke(root, *args, cli_home=self.home)

    def _sync(self, root: Path):
        r = self._invoke(root, "sync")
        self.assertEqual(r.returncode, 0, r.stderr)
        return r

    def _project(self, *recipe_ids: str) -> Path:
        td, root = temp_project(name="fixture", agents=("claude",))
        self.addCleanup(td.cleanup)
        if recipe_ids:
            _enable_recipes(root, *recipe_ids)
        return root

    def _configure_inspect(self, root: Path, recipe_id: str) -> dict:
        r = self._invoke(
            root, "recipe", "configure", recipe_id, "--inspect", "--json"
        )
        self.assertEqual(r.returncode, 0, r.stderr)
        return json.loads(r.stdout)

    # --- Phase 1: Manifest declarations ---

    def test_base_recipe_validates_and_declares_capability(self):
        root = self._project(BASE_ID)
        inspect = self._configure_inspect(root, BASE_ID)
        self.assertEqual(inspect["recipe"]["id"], BASE_ID)
        self.assertTrue(inspect["schema"]["fields"])

        data = _recipe_toml(BASE_ID)
        self.assertEqual([c["id"] for c in data["capabilities"]], ["ui-browser-testing"])
        self.assertEqual(
            {s["id"] for s in data["provides"]["skills"]},
            {"ui-browser-testing", "playwright-cli"},
        )
        self.assertIn("ui-smoke", {c["id"] for c in data["provides"]["commands"]})

        fields = {f["key"]: f for f in inspect["schema"]["fields"]}
        for key in ("ui_test_command", "ui_smoke_command", "playwright_config"):
            self.assertIn(key, fields)
            self.assertFalse(fields[key]["required"])
            self.assertIsNone(fields[key]["default"])
        self.assertNotIn("mcp", data["provides"], "base recipe must not ship Playwright MCP")

    def test_mcp_recipe_validates_without_capability(self):
        data = _recipe_toml(MCP_ID)
        self.assertEqual(data["recipe"]["id"], MCP_ID)
        self.assertEqual(data.get("capabilities", []), [])
        self.assertEqual({s["id"] for s in data["provides"]["skills"]}, {"playwright-mcp"})
        mcp = data["provides"]["mcp"][0]
        self.assertEqual(len(data["provides"]["mcp"]), 1)
        self.assertEqual(mcp["id"], "playwright")
        self.assertEqual(mcp["command"], "npx")
        args = mcp.get("args") or []
        self.assertTrue(any("@playwright/mcp" in str(a) for a in args))

    def test_hybrid_enablement_has_no_fatal_primitive_conflicts(self):
        root = self._project(BASE_ID, MCP_ID)
        r = self._sync(root)
        self.assertNotIn("fatal", r.stderr.lower())
        # Tags must not overlap (would WARN on hybrid, design D1/D8)
        base = _recipe_toml(BASE_ID)
        mcp = _recipe_toml(MCP_ID)
        overlap = set(base["recipe"]["tags"]) & set(mcp["recipe"]["tags"])
        self.assertEqual(overlap, set(), f"tag overlap would WARN on hybrid: {overlap}")

    def test_hybrid_has_single_capability_provider(self):
        root = self._project(BASE_ID, MCP_ID)
        self._sync(root)
        declaring = [
            rid
            for rid in (BASE_ID, MCP_ID)
            if any(
                c["id"] == "ui-browser-testing"
                for c in _recipe_toml(rid).get("capabilities", [])
            )
        ]
        self.assertEqual(declaring, [BASE_ID], "capability provider must be unique")

    # --- Phase 2: Materialization via the CLI ---

    def test_materialize_cli_only_skills_without_mcp(self):
        root = self._project(BASE_ID)
        self._sync(root)

        for skill_id in ("ui-browser-testing", "playwright-cli"):
            skill = (
                recipe_skill_dir(root, BASE_ID, skill_id, cli_home=self.home)
                / "SKILL.md"
            )
            self.assertTrue(skill.is_file(), f"missing {skill}")

        cmd = cache_command(root, "ui-smoke", cli_home=self.home)
        self.assertTrue(cmd.is_file(), f"missing command at {cmd}")

        doc = root / "ai-specs" / "recipes" / BASE_ID / "README.md"
        self.assertTrue(doc.is_file())

        # Base ships no Playwright MCP preset, so nothing materializes an mcp skill.
        data = _recipe_toml(BASE_ID)
        self.assertNotIn("mcp", data["provides"])
        mcp_skill = (
            recipe_skill_dir(root, MCP_ID, "playwright-mcp", cli_home=self.home)
            / "SKILL.md"
        )
        self.assertFalse(mcp_skill.exists(), "no MCP skill when only base is enabled")

    def test_materialize_hybrid_adds_mcp_skill_and_preset(self):
        root = self._project(BASE_ID, MCP_ID)
        self._sync(root)

        mcp_skill = (
            recipe_skill_dir(root, MCP_ID, "playwright-mcp", cli_home=self.home)
            / "SKILL.md"
        )
        self.assertTrue(mcp_skill.is_file())

        # Hybrid ships the playwright MCP preset with the npx launcher.
        mcp_preset = _recipe_toml(MCP_ID)["provides"]["mcp"][0]
        self.assertEqual(mcp_preset["id"], "playwright")
        self.assertEqual(mcp_preset["command"], "npx")

        # Discipline skill resolves from base only (one physical owner)
        base_discipline = (
            recipe_skill_dir(root, BASE_ID, "ui-browser-testing", cli_home=self.home)
            / "SKILL.md"
        )
        mcp_discipline = (
            recipe_skill_dir(root, MCP_ID, "ui-browser-testing", cli_home=self.home)
            / "SKILL.md"
        )
        self.assertTrue(base_discipline.is_file())
        self.assertFalse(mcp_discipline.exists())

    def test_validate_config_passes_with_unset_commands(self):
        root = self._project(BASE_ID)
        r = self._sync(root)
        # Unset optional commands must not fail the on-sync validate-config hook.
        self.assertNotIn("fatal", r.stderr.lower())

    def test_config_and_brief_fragments_present(self):
        # Base workflow rules render into AGENTS.md (CLI usage, smoke evidence).
        root = self._project(BASE_ID)
        self._sync(root)
        agents = (root / "AGENTS.md").read_text()
        self.assertTrue(
            any("CLI" in ln and "smoke" in ln.lower() for ln in agents.splitlines()),
            "workflow rules must render into AGENTS.md",
        )
        self.assertIn("ui_smoke_command", agents)

        data = _recipe_toml(BASE_ID)
        self.assertTrue(data["provides"]["brief"]["workflow_rules"])
        self.assertTrue(
            any(
                "ui_smoke_command" in c
                for c in data["provides"]["brief"]["useful_commands"]
            )
        )

        # Guard against a false green: exercise the harness's real {config.KEY}
        # substitution path through the tdd-flow recipe and confirm a supplied
        # value is actually rendered in place of the placeholder — not merely
        # that the literal key name appears.
        supplied = "pytest -q --tb=short"
        root2 = self._project(BASE_ID, "tdd-flow")
        manifest = root2 / "ai-specs" / "ai-specs.toml"
        with manifest.open("a", encoding="utf-8") as fh:
            fh.write(f'\n[recipes.tdd-flow.config]\ntest_command = "{supplied}"\n')
        self._sync(root2)
        agents2 = (root2 / "AGENTS.md").read_text()
        self.assertIn(supplied, agents2)
        self.assertNotIn("{config.test_command}", agents2)

    def test_tdd_flow_plus_base_materializes(self):
        root = self._project("tdd-flow", BASE_ID)
        self._sync(root)

    # --- Static catalog contract ---

    def test_mcp_preset_has_no_literal_secrets(self):
        data = _recipe_toml(MCP_ID)
        preset = data["provides"]["mcp"][0]
        blob = json.dumps(preset)
        for needle in ("sk-", "password", "SECRET=", "API_KEY=abc"):
            self.assertNotIn(needle.lower(), blob.lower())
        readme = (CATALOG / MCP_ID / "README.md").read_text()
        self.assertNotRegex(readme, r"(?i)(api[_-]?key|token)\s*[:=]\s*['\"]?[a-z0-9]{16,}")

    def test_skill_frontmatter_and_adapter_deferral(self):
        root = self._project(BASE_ID, MCP_ID)
        self._sync(root)

        lookups = (
            (BASE_ID, "ui-browser-testing"),
            (BASE_ID, "playwright-cli"),
            (MCP_ID, "playwright-mcp"),
        )
        texts = {}
        for rid, sid in lookups:
            p = (
                recipe_skill_dir(root, rid, sid, cli_home=self.home)
                / "SKILL.md"
            )
            texts[sid] = p.read_text()

        # Frontmatter must carry a name and description for every materialized skill.
        for sid, text in texts.items():
            self.assertRegex(text, rf"(?m)^name:\s*{sid}\s*$")
            self.assertRegex(text, r"(?m)^description:\s*[:>]")

        cli_adapter = texts["playwright-cli"]
        mcp_adapter = texts["playwright-mcp"]
        discipline = texts["ui-browser-testing"]

        self.assertIn("ui-browser-testing", cli_adapter)
        self.assertRegex(cli_adapter[:800], r"(?i)defer")
        self.assertIn("ui-browser-testing", mcp_adapter)
        self.assertRegex(mcp_adapter[:800], r"(?i)defer")
        self.assertIn("playwright-ui-flow", mcp_adapter)
        self.assertIn("evidence", discipline.lower())
        self.assertIn("tdd-flow", discipline)


class PlaywrightUiFlowDocsTests(unittest.TestCase):
    def test_capabilities_lists_ui_browser_testing(self):
        text = CAPABILITIES_DOC.read_text()
        # Both the capability id and its provider must live in the SAME
        # capability-table row, not merely somewhere in the file.
        self.assertRegex(
            text,
            r"(?m)^\|.*ui-browser-testing.*playwright-ui-flow.*$",
        )

    def test_recipes_catalog_documents_both(self):
        text = CATALOG_DOC.read_text()
        glance_start = text.find("## At a glance")
        glance_end = text.find("\n---\n", glance_start)
        table = text[glance_start:glance_end]
        self.assertIn("playwright-ui-flow", table)
        self.assertIn("playwright-mcp", table)
        self.assertRegex(text, r"## playwright-ui-flow\n")
        self.assertRegex(text, r"## playwright-mcp\n")
        self.assertIn("| `playwright` |", text)


if __name__ == "__main__":
    unittest.main()