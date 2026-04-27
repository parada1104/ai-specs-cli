import importlib.util
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
TOML_READ_PATH = ROOT / "lib" / "_internal" / "toml-read.py"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


class TomlReadTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(TOML_READ_PATH, "toml_read_internal")

    def write_manifest(self, text: str) -> Path:
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        manifest = root / "ai-specs.toml"
        manifest.write_text(text)
        return manifest

    def test_missing_sections_resolve_to_stable_defaults(self):
        manifest = self.write_manifest("[project]\nname = 'fixture'\n")

        data = self.mod.load_toml(manifest)

        self.assertEqual(self.mod.read_project(data), {"name": "fixture", "subrepos": []})
        self.assertEqual(self.mod.read_agents(data), {"enabled": []})
        self.assertEqual(self.mod.read_deps(data), [])
        self.assertEqual(self.mod.read_mcp(data), {})

    def test_mcp_environment_alias_normalizes_to_env_and_keeps_passthrough_fields(self):
        manifest = self.write_manifest(
            "[project]\nname = 'fixture'\n\n"
            "[mcp.demo]\n"
            "command = 'npx'\n"
            "args = ['-y', '@demo/server']\n"
            "environment = { API_KEY = '$DEMO_API_KEY' }\n"
            "timeout = 30000\n"
            "enabled = true\n"
        )

        data = self.mod.load_toml(manifest)

        self.assertEqual(
            self.mod.read_mcp(data),
            {
                "demo": {
                    "command": "npx",
                    "args": ["-y", "@demo/server"],
                    "env": {"API_KEY": "$DEMO_API_KEY"},
                    "timeout": 30000,
                    "enabled": True,
                }
            },
        )

    def test_recipes_absent_returns_empty_dict(self):
        manifest = self.write_manifest("[project]\nname = 'fixture'\n")
        data = self.mod.load_toml(manifest)
        self.assertEqual(self.mod.read_recipes(data), {})

    def test_recipes_present_normalizes_enabled_and_version(self):
        manifest = self.write_manifest(
            "[project]\nname = 'fixture'\n\n"
            "[recipes.runtime-memory-openmemory]\n"
            "enabled = true\n"
            "version = \"1.0.0\"\n\n"
            "[recipes.another-recipe]\n"
            "enabled = false\n"
            "version = \"2.0.0\"\n"
        )
        data = self.mod.load_toml(manifest)
        self.assertEqual(
            self.mod.read_recipes(data),
            {
                "runtime-memory-openmemory": {"enabled": True, "version": "1.0.0", "config": {}},
                "another-recipe": {"enabled": False, "version": "2.0.0", "config": {}},
            },
        )

    def test_recipes_ignores_invalid_entries(self):
        manifest = self.write_manifest(
            "[project]\nname = 'fixture'\n\n"
            "[recipes.bad-recipe]\n"
            "enabled = \"not-a-bool\"\n"
            "version = 123\n"
        )
        data = self.mod.load_toml(manifest)
        self.assertEqual(
            self.mod.read_recipes(data),
            {"bad-recipe": {"enabled": False, "version": "123", "config": {}}},
        )


    # --- V2 manifest parsing tests ------------------------------------------

    def test_recipes_with_config(self):
        manifest = self.write_manifest(
            "[project]\nname = 'fixture'\n\n"
            "[recipes.my-recipe]\n"
            "enabled = true\n"
            'version = "1.0.0"\n'
            "[recipes.my-recipe.config]\n"
            "timeout = 60\n"
            "board_id = 'abc123'\n"
        )
        data = self.mod.load_toml(manifest)
        recipes = self.mod.read_recipes(data)
        self.assertEqual(recipes["my-recipe"]["config"], {"timeout": 60, "board_id": "abc123"})

    def test_recipes_without_config_returns_empty_dict(self):
        manifest = self.write_manifest(
            "[project]\nname = 'fixture'\n\n"
            "[recipes.my-recipe]\n"
            "enabled = true\n"
        )
        data = self.mod.load_toml(manifest)
        recipes = self.mod.read_recipes(data)
        self.assertEqual(recipes["my-recipe"]["config"], {})

    def test_bindings_present(self):
        manifest = self.write_manifest(
            "[project]\nname = 'fixture'\n\n"
            "[[bindings]]\n"
            'capability = "tracker"\n'
            'recipe = "my-recipe"\n'
        )
        data = self.mod.load_toml(manifest)
        bindings = self.mod.read_bindings(data)
        self.assertEqual(bindings, [{"capability": "tracker", "recipe": "my-recipe"}])

    def test_bindings_absent_returns_empty_list(self):
        manifest = self.write_manifest("[project]\nname = 'fixture'\n")
        data = self.mod.load_toml(manifest)
        self.assertEqual(self.mod.read_bindings(data), [])

    def test_bindings_not_list_returns_empty_list(self):
        manifest = self.write_manifest(
            "[project]\nname = 'fixture'\n\n"
            'bindings = "not-a-list"\n'
        )
        data = self.mod.load_toml(manifest)
        self.assertEqual(self.mod.read_bindings(data), [])

    def test_read_section_bindings(self):
        manifest = self.write_manifest(
            "[project]\nname = 'fixture'\n\n"
            "[[bindings]]\n"
            'capability = "tracker"\n'
            'recipe = "my-recipe"\n'
        )
        data = self.mod.load_toml(manifest)
        bindings = self.mod.read_section(data, "bindings")
        self.assertEqual(bindings, [{"capability": "tracker", "recipe": "my-recipe"}])


if __name__ == "__main__":
    unittest.main()
