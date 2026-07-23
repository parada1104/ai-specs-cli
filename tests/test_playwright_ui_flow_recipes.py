"""Tests for playwright-ui-flow + playwright-mcp catalog recipes."""

from __future__ import annotations

import importlib.util
import io
import json
import sys
import tempfile
import tomllib
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(Path(__file__).resolve().parent))
from _cache_paths import cache_command, recipe_root  # noqa: E402

RECIPE_MATERIALIZE_PATH = ROOT / "lib" / "_internal" / "recipe-materialize.py"
RECIPE_SCHEMA_PATH = ROOT / "lib" / "_internal" / "recipe_schema.py"
RECIPE_CONFLICTS_PATH = ROOT / "lib" / "_internal" / "recipe-conflicts.py"
SKILL_CONTRACT_PATH = ROOT / "lib" / "_internal" / "skill_contract.py"
AGENTS_RENDER_PATH = ROOT / "lib" / "_internal" / "agents-render.py"
CATALOG = ROOT / "catalog" / "recipes"
BASE_ID = "playwright-ui-flow"
MCP_ID = "playwright-mcp"
CAPABILITIES_DOC = ROOT / "docs" / "capabilities.md"
CATALOG_DOC = ROOT / "docs" / "recipes-catalog.md"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


def _version(recipe_id: str) -> str:
    with (CATALOG / recipe_id / "recipe.toml").open("rb") as fh:
        return tomllib.load(fh)["recipe"]["version"]


class PlaywrightUiFlowRecipeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.schema = load_module(RECIPE_SCHEMA_PATH, "recipe_schema_pw_ui")
        cls.mat = load_module(RECIPE_MATERIALIZE_PATH, "recipe_materialize_pw_ui")
        cls.conflicts = load_module(RECIPE_CONFLICTS_PATH, "recipe_conflicts_pw_ui")
        cls.skill_contract = load_module(SKILL_CONTRACT_PATH, "skill_contract_pw_ui")
        cls.agents_render = load_module(AGENTS_RENDER_PATH, "agents_render_pw_ui")

    def test_base_recipe_validates_and_declares_capability(self):
        recipe = self.schema.load_recipe_toml(CATALOG / BASE_ID / "recipe.toml")
        self.assertEqual(recipe.id, BASE_ID)
        self.assertEqual([c.id for c in recipe.capabilities], ["ui-browser-testing"])
        skill_ids = {s.id for s in recipe.skills}
        self.assertEqual(skill_ids, {"ui-browser-testing", "playwright-cli"})
        self.assertIn("ui-smoke", {c.id for c in recipe.commands})
        fields = recipe.config_schema.fields
        for key in ("ui_test_command", "ui_smoke_command", "playwright_config"):
            self.assertIn(key, fields)
            self.assertFalse(fields[key].required)
            self.assertIsNone(fields[key].default)
        self.assertFalse(recipe.mcp, "base recipe must not ship Playwright MCP")

    def test_mcp_recipe_validates_without_capability(self):
        recipe = self.schema.load_recipe_toml(CATALOG / MCP_ID / "recipe.toml")
        self.assertEqual(recipe.id, MCP_ID)
        self.assertEqual(recipe.capabilities, [])
        self.assertEqual({s.id for s in recipe.skills}, {"playwright-mcp"})
        self.assertEqual(len(recipe.mcp), 1)
        self.assertEqual(recipe.mcp[0].id, "playwright")
        self.assertEqual(recipe.mcp[0].config.get("command"), "npx")
        args = recipe.mcp[0].config.get("args") or []
        self.assertTrue(any("@playwright/mcp" in str(a) for a in args))

    def test_hybrid_enablement_has_no_fatal_primitive_conflicts(self):
        conflicts = self.conflicts.check_recipe_conflicts(CATALOG, [BASE_ID, MCP_ID])
        fatal = [c for c in conflicts if getattr(c, "severity", None) == "fatal"]
        self.assertEqual(fatal, [], f"unexpected fatal conflicts: {fatal}")
        # Tags must not overlap (design D1/D8)
        base = self.schema.load_recipe_toml(CATALOG / BASE_ID / "recipe.toml")
        mcp = self.schema.load_recipe_toml(CATALOG / MCP_ID / "recipe.toml")
        overlap = set(base.tags) & set(mcp.tags)
        self.assertEqual(overlap, set(), f"tag overlap would WARN on hybrid: {overlap}")

    def test_hybrid_has_single_capability_provider(self):
        cap_conflicts = self.conflicts.check_capability_conflicts(
            CATALOG, [BASE_ID, MCP_ID], explicit_bindings=[]
        )
        ambig = [
            c
            for c in cap_conflicts
            if c.primitive_id == "ui-browser-testing"
        ]
        self.assertEqual(ambig, [], f"capability ambiguity: {ambig}")

    def _make_project(self, *recipe_blocks: str) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs = root / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "skills").mkdir()
        (ai_specs / "commands").mkdir()
        body = "[project]\nname = 'fixture'\n\n[agents]\nenabled = ['claude']\n\n"
        body += "\n".join(recipe_blocks)
        (ai_specs / "ai-specs.toml").write_text(body)
        return root

    def test_materialize_cli_only_skills_without_mcp(self):
        root = self._make_project(
            f'[recipes.{BASE_ID}]\nenabled = true\nversion = "{_version(BASE_ID)}"\n'
        )
        mcp_path = root / "ai-specs" / ".tmp" / "recipe-mcp.json"
        mcp_path.parent.mkdir(parents=True)
        self.assertEqual(self.mat.materialize_recipes(root, ROOT, mcp_path), 0)

        for skill_id in ("ui-browser-testing", "playwright-cli"):
            skill = recipe_root(root, BASE_ID) / "skills" / skill_id / "SKILL.md"
            self.assertTrue(skill.is_file(), f"missing {skill}")

        cmd = cache_command(root, "ui-smoke")
        self.assertTrue(cmd.is_file(), f"missing command at {cmd}")

        doc = root / "ai-specs" / "recipes" / BASE_ID / "README.md"
        self.assertTrue(doc.is_file())

        data = json.loads(mcp_path.read_text()) if mcp_path.is_file() else {}
        self.assertNotIn("playwright", data)

    def test_materialize_hybrid_adds_mcp_skill_and_preset(self):
        root = self._make_project(
            f'[recipes.{BASE_ID}]\nenabled = true\nversion = "{_version(BASE_ID)}"\n',
            f'[recipes.{MCP_ID}]\nenabled = true\nversion = "{_version(MCP_ID)}"\n',
        )
        mcp_path = root / "ai-specs" / ".tmp" / "recipe-mcp.json"
        mcp_path.parent.mkdir(parents=True)
        self.assertEqual(self.mat.materialize_recipes(root, ROOT, mcp_path), 0)

        mcp_skill = recipe_root(root, MCP_ID) / "skills" / "playwright-mcp" / "SKILL.md"
        self.assertTrue(mcp_skill.is_file())

        data = json.loads(mcp_path.read_text())
        self.assertIn("playwright", data)
        self.assertEqual(data["playwright"]["command"], "npx")

        # Discipline skill resolves from base only (one physical owner)
        base_discipline = (
            recipe_root(root, BASE_ID) / "skills" / "ui-browser-testing" / "SKILL.md"
        )
        mcp_discipline = (
            recipe_root(root, MCP_ID) / "skills" / "ui-browser-testing" / "SKILL.md"
        )
        self.assertTrue(base_discipline.is_file())
        self.assertFalse(mcp_discipline.exists())

    def test_validate_config_passes_with_unset_commands(self):
        root = self._make_project(
            f'[recipes.{BASE_ID}]\nenabled = true\nversion = "{_version(BASE_ID)}"\n'
        )
        # Capture stderr in case of warnings, but exit must be 0
        captured = io.StringIO()
        real_stderr = sys.stderr
        sys.stderr = captured
        try:
            rc = self.mat.materialize_recipes(root, ROOT)
        finally:
            sys.stderr = real_stderr
        self.assertEqual(rc, 0)

    def test_config_and_brief_fragments_present(self):
        recipe = self.schema.load_recipe_toml(CATALOG / BASE_ID / "recipe.toml")
        frags = recipe.brief_fragments
        self.assertTrue(frags.workflow_rules)
        joined = " ".join(
            f.text if hasattr(f, "text") else str(f) for f in frags.workflow_rules
        )
        self.assertIn("CLI", joined)
        self.assertTrue(
            any(
                "ui_smoke_command" in (f.text if hasattr(f, "text") else str(f))
                for f in frags.useful_commands
            )
        )
        # Guard against a false green: exercise the harness's real {config.KEY}
        # substitution path (agents-render.collect_recipe_brief_fragments) and
        # confirm a supplied ui_smoke_command value is actually rendered in place
        # of the placeholder — not merely that the literal key name appears.
        supplied = "npx playwright test --grep @smoke-sentinel"
        resolved = {
            "enabled": [BASE_ID],
            "recipes": {
                BASE_ID: {
                    "ui_smoke_command": supplied,
                    "brief_fragments": {
                        "useful_commands": [
                            {
                                "key": None,
                                "text": "Run UI smokes: `{config.ui_smoke_command}`",
                            }
                        ]
                    },
                }
            },
        }
        rendered = self.agents_render.collect_recipe_brief_fragments(
            resolved, "useful_commands"
        )
        joined_rendered = " ".join(f["text"] for f in rendered)
        self.assertIn(supplied, joined_rendered)
        self.assertNotIn("{config.ui_smoke_command}", joined_rendered)

    def test_tdd_flow_plus_base_materializes(self):
        tdd_ver = _version("tdd-flow")
        root = self._make_project(
            f'[recipes.tdd-flow]\nenabled = true\nversion = "{tdd_ver}"\n',
            f'[recipes.{BASE_ID}]\nenabled = true\nversion = "{_version(BASE_ID)}"\n',
        )
        self.assertEqual(self.mat.materialize_recipes(root, ROOT), 0)

    def test_mcp_preset_has_no_literal_secrets(self):
        recipe = self.schema.load_recipe_toml(CATALOG / MCP_ID / "recipe.toml")
        preset = recipe.mcp[0]
        blob = json.dumps(preset.config)
        for needle in ("sk-", "password", "SECRET=", "API_KEY=abc"):
            self.assertNotIn(needle.lower(), blob.lower())
        readme = (CATALOG / MCP_ID / "README.md").read_text()
        self.assertNotRegex(readme, r"(?i)(api[_-]?key|token)\s*[:=]\s*['\"]?[a-z0-9]{16,}")

    def test_skill_frontmatter_and_adapter_deferral(self):
        discipline = (
            CATALOG / BASE_ID / "skills" / "ui-browser-testing" / "SKILL.md"
        ).read_text()
        cli_adapter = (
            CATALOG / BASE_ID / "skills" / "playwright-cli" / "SKILL.md"
        ).read_text()
        mcp_adapter = (
            CATALOG / MCP_ID / "skills" / "playwright-mcp" / "SKILL.md"
        ).read_text()

        for path, text in (
            (CATALOG / BASE_ID / "skills" / "ui-browser-testing" / "SKILL.md", discipline),
            (CATALOG / BASE_ID / "skills" / "playwright-cli" / "SKILL.md", cli_adapter),
            (CATALOG / MCP_ID / "skills" / "playwright-mcp" / "SKILL.md", mcp_adapter),
        ):
            skill = self.skill_contract.from_local_skill(path, compatibility=True)
            self.assertTrue(skill.get("name"))
            self.assertTrue(skill.get("description") or skill.get("description_summary"))

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
