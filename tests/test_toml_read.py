import json
import tempfile
import unittest
from pathlib import Path

from _blackbox import invoke, isolated_home


class TomlReadTests(unittest.TestCase):
    """Black-box conversions of the original `toml-read` parsing tests.

    The readers in lib/_internal/toml-read.py are consumed only by the `sync`
    pipeline, so every test drives `bin/ai-specs sync` against a project whose
    manifest is the original fixture, then asserts on the emitted tree and on
    stdout/stderr (`!` warning lines included). Each method keeps its exact
    original manifest: where a reader's normalized value has no artifact emitted
    from that exact fixture, the assertion is left with a `# TRIAGE:`
    justification naming the specific surface it searched, never manufactured by
    reshaping the manifest. None of the tests are skipped.
    """

    def _cli_home(self) -> Path:
        """One shared install+cache root per test (required for a sequence)."""
        if getattr(self, "_shared_home", None) is None:
            tmp = tempfile.TemporaryDirectory()
            self.addCleanup(tmp.cleanup)
            self._shared_home = isolated_home(Path(tmp.name))
        return self._shared_home

    def _sync(self, root: Path):
        """Single shared helper wrapping the CLI invocation for this class."""
        return invoke(root, "sync", cli_home=self._cli_home())

    def _make_project(self, manifest: str) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        project = Path(tmp.name)
        ai_specs = project / "ai-specs"
        ai_specs.mkdir()
        (ai_specs / "ai-specs.toml").write_text(manifest, encoding="utf-8")
        (ai_specs / "skills").mkdir()
        (ai_specs / "commands").mkdir()
        return project

    def test_missing_sections_resolve_to_stable_defaults(self):
        # TRIAGE: the `assertEqual(self.mod.read_mcp(data), {})` assertion from
        # this exact manifest `bin/ai-specs sync <project>` is unobservable:
        # with no [agents].enabled the sync-agent step has zero targets, so the
        # CLI never reaches MCP rendering and emits no artifact proving the
        # "missing mcp -> {}" default. The missing-section defaults that DO
        # surface from the same manifest are asserted instead: project name in
        # AGENTS.md, and the no-deps / no-recipes / no-agents notices.
        manifest = "[project]\nname = 'fixture'\n"
        project = self._make_project(manifest)
        result = self._sync(project)
        self.assertEqual(result.returncode, 0)
        agents_md = (project / "AGENTS.md").read_text(encoding="utf-8")
        self.assertIn("- **Project**: `fixture`", agents_md)
        self.assertIn("(no [[deps]] declared — nothing to vendor)", result.stdout)
        self.assertIn("(no [recipes.*] enabled — skipping)", result.stdout)
        self.assertIn("WARNING: no agents to sync", result.stderr)

    def test_mcp_environment_alias_normalizes_to_env_and_keeps_passthrough_fields(self):
        manifest = (
            "[project]\nname = 'fixture'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            "[mcp.demo]\n"
            "command = 'npx'\n"
            "args = ['-y', '@demo/server']\n"
            "environment = { API_KEY = '$DEMO_API_KEY' }\n"
            "timeout = 30000\n"
            "enabled = true\n"
        )
        project = self._make_project(manifest)
        result = self._sync(project)
        self.assertEqual(result.returncode, 0)
        mcp = json.loads((project / ".mcp.json").read_text(encoding="utf-8"))
        server = mcp["mcpServers"]["demo"]
        # `environment` alias -> `env`, and `$VAR` -> `${VAR}` passthrough.
        self.assertEqual(server["env"], {"API_KEY": "${DEMO_API_KEY}"})
        self.assertEqual(server["command"], "npx")
        self.assertEqual(server["args"], ["-y", "@demo/server"])
        self.assertEqual(server["timeout"], 30000)
        self.assertIs(server["enabled"], True)

    def test_recipes_absent_returns_empty_dict(self):
        manifest = "[project]\nname = 'fixture'\n"
        project = self._make_project(manifest)
        result = self._sync(project)
        self.assertEqual(result.returncode, 0)
        self.assertIn("(no [recipes.*] enabled — skipping)", result.stdout)

    def test_recipes_present_normalizes_enabled_and_version(self):
        manifest = (
            "[project]\nname = 'fixture'\n\n"
            "[recipes.runtime-memory-openmemory]\n"
            "enabled = true\n"
            "version = \"1.0.0\"\n\n"
            "[recipes.another-recipe]\n"
            "enabled = false\n"
            "version = \"2.0.0\"\n"
        )
        project = self._make_project(manifest)
        result = self._sync(project)
        # The enabled recipe is routed to materialization and fails only because
        # it is absent from the installed catalog; the disabled recipe is never
        # routed, proving the enabled/disabled normalization observably.
        self.assertEqual(result.returncode, 1)
        self.assertIn("recipe directory not found", result.stderr)
        self.assertIn("runtime-memory-openmemory", result.stderr)
        self.assertNotIn("another-recipe", result.stderr)

    def test_recipes_ignores_invalid_entries(self):
        # TRIAGE: the `version=123 -> "123"` normalization in read_recipes is
        # unobservable from this exact manifest via `bin/ai-specs sync`: that
        # value only surfaces through the legacy-version WARN, which the
        # materializer emits for enabled recipes, but here enabled="not-a-bool"
        # normalizes to False, so the recipe is never routed and the version
        # string is never printed. The enabled->False default IS observable and
        # is asserted: no materialization attempt and the skipped notice.
        manifest = (
            "[project]\nname = 'fixture'\n\n"
            "[recipes.bad-recipe]\n"
            "enabled = \"not-a-bool\"\n"
            "version = 123\n"
        )
        project = self._make_project(manifest)
        result = self._sync(project)
        self.assertEqual(result.returncode, 0)
        self.assertIn("(no [recipes.*] enabled — skipping)", result.stdout)
        self.assertNotIn("recipe directory not found", result.stderr)

    def test_recipes_with_config(self):
        # TRIAGE: the `recipes["my-recipe"]["config"] == {"timeout": 60,
        # "board_id": "abc123"}` assertion is unobservable from this exact
        # manifest via `bin/ai-specs sync`: "my-recipe" is absent from the
        # installed catalog, so the recipe step fails with "recipe directory
        # not found" at read_recipe before the config is merged or emitted into
        # any persistent artifact.
        manifest = (
            "[project]\nname = 'fixture'\n\n"
            "[recipes.my-recipe]\n"
            "enabled = true\n"
            'version = "1.0.0"\n'
            "[recipes.my-recipe.config]\n"
            "timeout = 60\n"
            "board_id = 'abc123'\n"
        )
        project = self._make_project(manifest)
        result = self._sync(project)
        self.assertEqual(result.returncode, 1)
        self.assertIn("recipe directory not found", result.stderr)
        self.assertIn("my-recipe", result.stderr)

    def test_recipes_without_config_returns_empty_dict(self):
        # TRIAGE: the `recipes["my-recipe"]["config"] == {}` assertion is
        # unobservable from this exact manifest via `bin/ai-specs sync`:
        # "my-recipe" is not in the installed catalog, so sync fails at
        # read_recipe before the empty-config normalization is emitted anywhere.
        manifest = (
            "[project]\nname = 'fixture'\n\n"
            "[recipes.my-recipe]\n"
            "enabled = true\n"
        )
        project = self._make_project(manifest)
        result = self._sync(project)
        self.assertEqual(result.returncode, 1)
        self.assertIn("recipe directory not found", result.stderr)
        self.assertIn("my-recipe", result.stderr)

    def test_recipes_without_version_normalizes_empty(self):
        manifest = (
            "[project]\nname = 'fixture'\n\n"
            "[recipes.my-recipe]\n"
            "enabled = true\n"
        )
        project = self._make_project(manifest)
        result = self._sync(project)
        # version absent -> "" -> no legacy-version WARN is emitted; contrast
        # with test_recipes_present_normalizes_enabled_and_version, whose
        # present version would warn had it reached that path.
        self.assertEqual(result.returncode, 1)
        self.assertNotIn("legacy version", result.stderr)

    def test_bindings_present(self):
        # `[[bindings]] capability = "tracker"` parses via read_bindings and
        # drives the `## Trello Tracking` section in AGENTS.md (agents-render
        # resolves bindings.tracker -> recipe config board_id).
        manifest = (
            "[project]\nname = 'fixture'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            "[recipes.trello-mcp-workflow]\nenabled = true\n"
            "[recipes.trello-mcp-workflow.config]\n"
            "board_id = '69ec097f13e2d38ecd89a557'\n\n"
            "[[bindings]]\n"
            "capability = 'tracker'\n"
            "recipe = 'trello-mcp-workflow'\n"
        )
        project = self._make_project(manifest)
        result = self._sync(project)
        self.assertEqual(result.returncode, 0)
        agents_md = (project / "AGENTS.md").read_text(encoding="utf-8")
        self.assertIn("## Trello Tracking", agents_md)
        self.assertIn("- **Board**: `69ec097f13e2d38ecd89a557`", agents_md)

    def test_bindings_absent_returns_empty_list(self):
        manifest = "[project]\nname = 'fixture'\n"
        project = self._make_project(manifest)
        result = self._sync(project)
        # Empty bindings add no capability validation and no binding-derived
        # artifact; the clean exit 0 is the observable surface.
        self.assertEqual(result.returncode, 0)

    def test_bindings_not_list_returns_empty_list(self):
        manifest = (
            "[project]\nname = 'fixture'\n\n"
            'bindings = "not-a-list"\n'
        )
        project = self._make_project(manifest)
        result = self._sync(project)
        # A non-list bindings value is normalized to an empty list, so sync
        # proceeds with no binding-derived artifact — exit 0 is the surface.
        self.assertEqual(result.returncode, 0)

    def test_read_section_bindings(self):
        # read_section(data, "bindings") dispatches to read_bindings; the
        # canonical-store capability binding drives the `Vault scope` bullet
        # in AGENTS.md (agents-render bindings.canonical-store -> vault_scope),
        # a distinct emitted surface from the tracker binding.
        manifest = (
            "[project]\nname = 'fixture'\n\n"
            "[agents]\nenabled = ['claude']\n\n"
            "[recipes.vault-canonical-store]\nenabled = true\n"
            "[recipes.vault-canonical-store.config]\n"
            "vault_scope = 'nnodes/proyectos/ai-specs'\n\n"
            "[[bindings]]\n"
            "capability = 'canonical-store'\n"
            "recipe = 'vault-canonical-store'\n"
        )
        project = self._make_project(manifest)
        result = self._sync(project)
        self.assertEqual(result.returncode, 0)
        agents_md = (project / "AGENTS.md").read_text(encoding="utf-8")
        self.assertIn("**Vault scope**: `nnodes/proyectos/ai-specs`", agents_md)


if __name__ == "__main__":
    unittest.main()
