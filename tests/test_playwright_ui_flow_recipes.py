"""Black-box playwright-ui-flow + playwright-mcp recipe tests: every behavioral
test drives ``bin/ai-specs``.

No test may import ``lib/_internal`` modules. Assertions preserve the original
contract intents (schema declarations, hybrid-enablement conflict surfaces,
materialized artifacts, MCP preset rendering, brief-fragment substitution,
golden skill content, docs tables) through the CLI process boundary:

- Recipe schema validity and declared primitives are observable via
  ``recipe add`` (validation + exact id + "The next sync will materialize:"
  plan) and via ``tomllib`` reads of the catalog recipe.toml.
- Conflict grading is observable via ``sync``: fatal primitive conflicts fail
  the sync ("recipe conflict: ..."), tag overlaps and capability ambiguity
  warn on stderr.
- Materialization is observable via ``sync``: skills under the per-project
  cache's ``.recipe/`` tree, commands under the cache commands dir, docs under
  the project harness tree, and rendered MCP configs per agent.
- Brief-fragment ``{config.KEY}`` substitution is observable via ``sync`` on a
  fresh-id copy of the recipe whose declared useful-command uses the
  placeholder: AGENTS.md renders the configured value in its place.
"""
from __future__ import annotations

import json
import re
import shutil
import sys
import tempfile
import tomllib
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from _blackbox import cache_project_dir, invoke, isolated_home, populate_catalog  # noqa: E402

ROOT = Path(__file__).resolve().parents[1]
CATALOG = ROOT / "catalog" / "recipes"
BASE_ID = "playwright-ui-flow"
MCP_ID = "playwright-mcp"
CAPABILITIES_DOC = ROOT / "docs" / "capabilities.md"
CATALOG_DOC = ROOT / "docs" / "recipes-catalog.md"


def _make_home(base: Path) -> Path:
    """Isolated CLI install root with a REAL lib copy.

    sync/materialize derive cache and catalog roots from their own realpath,
    so a symlinked lib would resolve back into the repository and let the CLI
    touch repo cache state. A real copy keeps every lookup and write in temp.
    """
    home = isolated_home(base)
    (home / "lib").unlink()
    shutil.copytree(
        ROOT / "lib", home / "lib", symlinks=True,
        ignore=shutil.ignore_patterns("_vendor", "__pycache__"),
    )
    return home


def _make_manifest(root: Path, name: str = "fixture") -> None:
    """Minimal initialized project (manifest + harness dirs) in temp."""
    (root / "ai-specs" / "skills").mkdir(parents=True, exist_ok=True)
    (root / "ai-specs" / "commands").mkdir(parents=True, exist_ok=True)
    (root / "ai-specs" / "ai-specs.toml").write_text(
        f"[project]\nname = {name!r}\n\n[agents]\nenabled = ['claude']\n"
    )


def _frontmatter(text: str) -> dict[str, str]:
    """Parse the simple ``key: value`` YAML frontmatter of a catalog SKILL.md."""
    match = re.match(r"^---\n(.*?)\n---\n", text, re.S)
    assert match, "catalog skill must open with YAML frontmatter"
    fields = {}
    for line in match.group(1).splitlines():
        key, _, value = line.partition(":")
        if key.strip() and value.strip():
            fields[key.strip()] = value.strip()
    return fields


class _CliFixtureMixin:
    """One shared isolated cli_home and temp project per test command sequence."""

    def setUp(self):
        tmp = tempfile.TemporaryDirectory(prefix="ai-specs-pw-")
        self.addCleanup(tmp.cleanup)
        self.base = Path(tmp.name)
        self.home = _make_home(self.base)
        self.root = self.base / "proj"
        _make_manifest(self.root)

    def recipe_add(self, recipe_id: str):
        result = invoke(self.root, "recipe", "add", recipe_id, cli_home=self.home)
        self.assertEqual(
            result.returncode, 0,
            f"recipe add {recipe_id} failed: {result.stdout}{result.stderr}",
        )
        return result

    def sync(self):
        return invoke(self.root, "sync", cli_home=self.home)

    def cache(self) -> Path:
        return cache_project_dir(self.root, self.home)

    def fresh_id_copy(self, fresh_id: str, replacements: dict[str, str]) -> Path:
        """Seed a fresh-id copy of the base recipe into the isolated catalog.

        Materializes the home catalog first (per-recipe symlinks, like
        ``populate_catalog``) so the fresh-id copy lives in temp, never in the
        repository catalog. Assets stay absolutely linked to the real recipe so
        the only behavioral delta is the replaced recipe.toml text. init.md
        must be a real copy: the sync validator rejects init prompt paths that
        leave the recipe dir.
        """
        populate_catalog(self.home, "bootstrap")
        toml = (CATALOG / BASE_ID / "recipe.toml").read_text()
        for old, new in replacements.items():
            assert old in toml, f"replacement source not found: {old!r}"
            toml = toml.replace(old, new, 1)
        fresh_dir = self.home / "catalog" / "recipes" / fresh_id
        fresh_dir.mkdir(parents=True, exist_ok=True)
        (fresh_dir / "recipe.toml").write_text(toml)
        for asset in ("skills", "commands", "README.md"):
            (fresh_dir / asset).symlink_to(CATALOG / BASE_ID / asset)
        (fresh_dir / "init.md").write_text((CATALOG / BASE_ID / "init.md").read_text())
        return fresh_dir


class PlaywrightUiFlowRecipeTests(_CliFixtureMixin, unittest.TestCase):
    def test_base_recipe_validates_and_declares_capability(self):
        """Base recipe validates through the CLI and declares its primitives.

        ``recipe add`` runs schema validation and adds by the exact id; the
        declared capability/skill/command/config/mcp shapes are read from the
        catalog declaration.
        """
        result = self.recipe_add(BASE_ID)
        self.assertIn(
            f"Recipe '{BASE_ID}' added to the manifest.", result.stdout,
            "playwright-ui-flow must pass schema validation and add by its exact id",
        )
        self.assertIn(
            "The next sync will materialize:", result.stdout,
            "recipe add must print the declared primitives",
        )
        self.assertIn(
            "- skills: ui-browser-testing, playwright-cli", result.stdout,
            "both CLI skills must be declared by the base recipe",
        )
        self.assertIn(
            "- commands: ui-smoke", result.stdout,
            "ui-smoke command must be declared by the base recipe",
        )
        raw = tomllib.loads((CATALOG / BASE_ID / "recipe.toml").read_text())
        self.assertEqual(raw["recipe"]["id"], BASE_ID)
        self.assertEqual([c["id"] for c in raw["capabilities"]], ["ui-browser-testing"])
        skill_ids = {s["id"] for s in raw["provides"]["skills"]}
        self.assertEqual(skill_ids, {"ui-browser-testing", "playwright-cli"})
        self.assertIn("ui-smoke", {c["id"] for c in raw["provides"]["commands"]})
        fields = raw["config"]
        for key in ("ui_test_command", "ui_smoke_command", "playwright_config"):
            self.assertIn(key, fields)
            self.assertFalse(fields[key]["required"])
            self.assertNotIn(
                "default", fields[key],
                f"{key} must not carry an invented default",
            )
        self.assertNotIn(
            "mcp", raw.get("provides", {}),
            "base recipe must not ship Playwright MCP",
        )

    def test_mcp_recipe_validates_without_capability(self):
        """MCP recipe validates through the CLI and ships the playwright MCP
        preset without declaring any capability."""
        result = self.recipe_add(MCP_ID)
        self.assertIn(
            f"Recipe '{MCP_ID}' added to the manifest.", result.stdout,
            "playwright-mcp must pass schema validation and add by its exact id",
        )
        self.assertIn(
            "- mcp: playwright", result.stdout,
            "recipe add plan must declare the playwright MCP",
        )
        raw = tomllib.loads((CATALOG / MCP_ID / "recipe.toml").read_text())
        self.assertEqual(raw["recipe"]["id"], MCP_ID)
        self.assertEqual(raw.get("capabilities", []), [])
        self.assertEqual({s["id"] for s in raw["provides"]["skills"]}, {"playwright-mcp"})
        mcps = raw["provides"]["mcp"]
        self.assertEqual(len(mcps), 1)
        self.assertEqual(mcps[0]["id"], "playwright")
        self.assertEqual(mcps[0].get("command"), "npx")
        args = mcps[0].get("args") or []
        self.assertTrue(any("@playwright/mcp" in str(a) for a in args))

    def test_hybrid_enablement_has_no_fatal_primitive_conflicts(self):
        """Enabling both recipes syncs cleanly: no fatal primitive conflict and
        no tag overlap warning.

        Fatal primitive conflicts fail the sync with "recipe conflict:";
        tag overlaps would warn "tag overlap: ..." on stderr — neither may
        appear for the hybrid pair.
        """
        self.recipe_add(BASE_ID)
        self.recipe_add(MCP_ID)
        result = self.sync()
        self.assertEqual(
            result.returncode, 0,
            f"hybrid enablement must not hit a fatal conflict: {result.stdout}{result.stderr}",
        )
        self.assertNotIn(
            "recipe conflict:", result.stderr,
            "hybrid pair must not claim the same primitive",
        )
        self.assertNotIn(
            f"tag overlap: recipes {MCP_ID}, {BASE_ID}", result.stderr,
            "tag overlap between the pair would WARN on hybrid enablement",
        )
        # Tags must not overlap (design D1/D8) — declared shapes confirm it.
        base = tomllib.loads((CATALOG / BASE_ID / "recipe.toml").read_text())
        mcp = tomllib.loads((CATALOG / MCP_ID / "recipe.toml").read_text())
        overlap = set(base["recipe"]["tags"]) & set(mcp["recipe"]["tags"])
        self.assertEqual(overlap, set(), f"tag overlap would WARN on hybrid: {overlap}")

    def test_hybrid_has_single_capability_provider(self):
        """ui-browser-testing has exactly one provider: sync must not warn
        about a capability ambiguity for it."""
        self.recipe_add(BASE_ID)
        self.recipe_add(MCP_ID)
        result = self.sync()
        self.assertEqual(
            result.returncode, 0,
            f"single capability provider must sync cleanly: {result.stdout}{result.stderr}",
        )
        self.assertNotIn(
            "capability ambiguity: capability.id='ui-browser-testing'",
            result.stderr,
            "ui-browser-testing must have a single provider (no ambiguity warning)",
        )

    def test_materialize_cli_only_skills_without_mcp(self):
        """Base-only sync materializes CLI skills/command/doc and renders no
        playwright MCP surface for the agent."""
        self.recipe_add(BASE_ID)
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

        for skill_id in ("ui-browser-testing", "playwright-cli"):
            skill = self.cache() / ".recipe" / BASE_ID / "skills" / skill_id / "SKILL.md"
            self.assertTrue(skill.is_file(), f"missing {skill}")

        cmd = self.cache() / "commands" / "ui-smoke.md"
        self.assertTrue(cmd.is_file(), f"missing command at {cmd}")

        doc = self.root / "ai-specs" / "recipes" / BASE_ID / "README.md"
        self.assertTrue(doc.is_file(), f"missing doc at {doc}")

        mcp_json = self.root / ".mcp.json"
        self.assertFalse(
            mcp_json.exists() and "playwright" in mcp_json.read_text(),
            "claude must not get a playwright MCP entry without playwright-mcp",
        )

    def test_materialize_hybrid_adds_mcp_skill_and_preset(self):
        """Hybrid sync adds the playwright MCP skill + preset; the discipline
        skill still resolves from the base recipe only (one physical owner)."""
        self.recipe_add(BASE_ID)
        self.recipe_add(MCP_ID)
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

        mcp_skill = self.cache() / ".recipe" / MCP_ID / "skills" / "playwright-mcp" / "SKILL.md"
        self.assertTrue(mcp_skill.is_file(), f"missing mcp skill at {mcp_skill}")

        mcp_json = self.root / ".mcp.json"
        self.assertTrue(mcp_json.is_file(), "hybrid sync must render the MCP preset")
        data = json.loads(mcp_json.read_text())
        self.assertIn("playwright", data.get("mcpServers", {}))
        self.assertEqual(data["mcpServers"]["playwright"]["command"], "npx")

        # Discipline skill resolves from base only (one physical owner)
        base_discipline = (
            self.cache() / ".recipe" / BASE_ID / "skills" / "ui-browser-testing" / "SKILL.md"
        )
        mcp_discipline = (
            self.cache() / ".recipe" / MCP_ID / "skills" / "ui-browser-testing" / "SKILL.md"
        )
        self.assertTrue(base_discipline.is_file())
        self.assertFalse(mcp_discipline.exists())

    def test_validate_config_passes_with_unset_commands(self):
        """Sync passes with the optional command config left unset."""
        self.recipe_add(BASE_ID)
        result = self.sync()
        self.assertEqual(
            result.returncode, 0,
            f"unset optional commands must pass validate-config: {result.stdout}{result.stderr}",
        )

    def test_config_and_brief_fragments_present(self):
        """The base recipe declares brief fragments mentioning ui_smoke_command,
        and the harness's real {config.KEY} substitution renders a supplied
        value in AGENTS.md in place of the placeholder — not merely that the
        literal key name appears."""
        raw = tomllib.loads((CATALOG / BASE_ID / "recipe.toml").read_text())
        brief = raw["provides"]["brief"]
        self.assertTrue(brief["workflow_rules"])
        rules = " ".join(
            f["text"] if isinstance(f, dict) else f for f in brief["workflow_rules"]
        )
        self.assertIn("CLI", rules)
        joined = " ".join(brief["useful_commands"])
        self.assertTrue(
            any("ui_smoke_command" in f for f in brief["useful_commands"]),
            "useful_commands fragment must reference ui_smoke_command",
        )
        # Guard against a false green: exercise the harness's real {config.KEY}
        # substitution path through sync — a fresh-id copy of the recipe whose
        # useful-command uses the placeholder must render the supplied value.
        fresh_id = "pw-ui-fragcheck"
        # Give the copy a declared useful-command that uses the real
        # {config.KEY} placeholder (the catalog prose fragment references the
        # key by name only).
        prose = (
            '  "Run UI smokes via the command configured at '
            '`[recipes.playwright-ui-flow.config].ui_smoke_command` '
            '(or `ui_test_command` for the full suite); if unset, discover '
            "the project's Playwright script and propose a config value.\","
        )
        self.fresh_id_copy(fresh_id, {
            f'id = "{BASE_ID}"': f'id = "{fresh_id}"',
            prose: '  "Run UI smokes: `{config.ui_smoke_command}`",',
        })
        manifest = self.root / "ai-specs" / "ai-specs.toml"
        manifest.write_text(
            "[project]\nname = 'fixture'\n\n[agents]\nenabled = ['claude']\n\n"
            f"[recipes.{fresh_id}]\nenabled = true\n\n"
            f"[recipes.{fresh_id}.config]\n"
            "ui_smoke_command = 'npx playwright test --grep @smoke-sentinel'\n"
        )
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        agents = (self.root / "AGENTS.md").read_text()
        self.assertIn(
            "npx playwright test --grep @smoke-sentinel", agents,
            "supplied ui_smoke_command must be rendered into AGENTS.md",
        )
        self.assertNotIn(
            "{config.ui_smoke_command}", agents,
            "the placeholder must be substituted, not re-emitted verbatim",
        )

    def test_tdd_flow_plus_base_materializes(self):
        """tdd-flow and the base recipe materialize together."""
        self.recipe_add("tdd-flow")
        self.recipe_add(BASE_ID)
        result = self.sync()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue(
            (self.cache() / ".recipe" / "tdd-flow" / "skills" / "tdd-flow" / "SKILL.md").is_file(),
            "tdd-flow skill must materialize alongside the base recipe",
        )
        self.assertTrue(
            (self.cache() / ".recipe" / BASE_ID / "skills" / "ui-browser-testing" / "SKILL.md").is_file(),
            "base discipline skill must materialize alongside tdd-flow",
        )

    def test_mcp_preset_has_no_literal_secrets(self):
        """The MCP preset carries no literal secrets on any catalog surface."""
        raw = tomllib.loads((CATALOG / MCP_ID / "recipe.toml").read_text())
        preset = raw["provides"]["mcp"][0]
        blob = json.dumps(preset)
        for needle in ("sk-", "password", "SECRET=", "API_KEY=abc"):
            self.assertNotIn(needle.lower(), blob.lower())
        readme = (CATALOG / MCP_ID / "README.md").read_text()
        self.assertNotRegex(readme, r"(?i)(api[_-]?key|token)\s*[:=]\s*['\"]?[a-z0-9]{16,}")

    def test_skill_frontmatter_and_adapter_deferral(self):
        """Every skill has usable frontmatter, both adapters defer to the
        discipline skill, and the discipline skill binds evidence + TDD."""
        discipline = (CATALOG / BASE_ID / "skills" / "ui-browser-testing" / "SKILL.md").read_text()
        cli_adapter = (CATALOG / BASE_ID / "skills" / "playwright-cli" / "SKILL.md").read_text()
        mcp_adapter = (CATALOG / MCP_ID / "skills" / "playwright-mcp" / "SKILL.md").read_text()

        for name, text in (
            ("ui-browser-testing", discipline),
            ("playwright-cli", cli_adapter),
            ("playwright-mcp", mcp_adapter),
        ):
            with self.subTest(skill=name):
                fm = _frontmatter(text)
                self.assertTrue(fm.get("name"))
                self.assertTrue(
                    fm.get("description") or fm.get("description_summary")
                )

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
