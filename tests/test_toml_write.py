import importlib.util
import sys
import tempfile
import tomllib
import unittest
from pathlib import Path

from _blackbox import invoke, isolated_home

ROOT = Path(__file__).resolve().parents[1]
TOML_WRITE_PATH = ROOT / "lib" / "_internal" / "toml_write.py"


def load_module(path: Path, name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[spec.name] = module
    spec.loader.exec_module(module)
    return module


class TomlValueTests(unittest.TestCase):
    """Black-box conversions of the original `toml_value` unit tests.

    `toml_value` (lib/_internal/toml_write.py) is the shared TOML literal
    serializer used by lib/_internal/mcp-render.py when it writes per-agent
    config files. For agent `codex`, `sync` emits `.codex/config.toml` whose
    `[mcp_servers.<name>]` section is serialized through `toml_value`, so each
    scalar/collection type is observable on that file. Every test drives
    `bin/ai-specs sync <project_root>` through the shared `invoke` helper and
    asserts the FROZEN emitted bytes (parity contract §3 formatting:
    `"a\\"b"` for an embedded quote, `", "` list spacing, `{ k = v }` dicts).
    """

    def _cli_home(self) -> Path:
        """One shared install+cache root per test (required for sequences)."""
        if getattr(self, "_shared_home", None) is None:
            tmp = tempfile.TemporaryDirectory()
            self.addCleanup(tmp.cleanup)
            self._shared_home = isolated_home(Path(tmp.name))
        return self._shared_home

    def _sync_with_mcp(self, mcp_manifest_section: str):
        """Build a project whose ai-specs.toml carries the given [mcp.*]
        section, run `sync`, and return (result, project root)."""
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        ai_specs_dir = root / "ai-specs"
        ai_specs_dir.mkdir(parents=True)
        (ai_specs_dir / "ai-specs.toml").write_text(
            '[project]\nname = "demo"\n\n[agents]\nenabled = ["codex"]\n\n'
            + mcp_manifest_section,
            encoding="utf-8",
        )
        result = invoke(root, "sync", cli_home=self._cli_home())
        return result, root

    def _codex_config(self, root: Path) -> str:
        return (root / ".codex" / "config.toml").read_text(encoding="utf-8")

    def test_bool_is_lowercase(self):
        result, root = self._sync_with_mcp(
            '[mcp.demo]\ncommand = "npx"\nenabled = true\nauto_connect = false\n'
        )
        self.assertEqual(result.returncode, 0)
        config = self._codex_config(root)
        self.assertIn("enabled = true", config)
        self.assertIn("auto_connect = false", config)
        self.assertNotIn("True", config)
        self.assertNotIn("False", config)

    def test_bool_not_serialized_as_int(self):
        # bool is a subclass of int; must not collapse to 1/0 in the emitted TOML.
        result, root = self._sync_with_mcp(
            '[mcp.demo]\ncommand = "npx"\nenabled = true\nauto_connect = false\n'
        )
        self.assertEqual(result.returncode, 0)
        config = self._codex_config(root)
        self.assertIn("enabled = true", config)
        self.assertNotIn("enabled = 1", config)
        self.assertNotIn("auto_connect = 0", config)

    def test_int_and_float(self):
        result, root = self._sync_with_mcp(
            '[mcp.demo]\ncommand = "npx"\ntimeout = 30000\nretries = 2.5\n'
        )
        self.assertEqual(result.returncode, 0)
        config = self._codex_config(root)
        self.assertIn("timeout = 30000", config)
        self.assertIn("retries = 2.5", config)

    def test_string_is_double_quoted(self):
        # command is a*double-quote*b (single-quoted TOML literal string in
        # the manifest); toml_value must escape the embedded quote so the
        # emitted basic string stays valid TOML.
        result, root = self._sync_with_mcp("[mcp.demo]\ncommand = 'a\"b'\n")
        self.assertEqual(result.returncode, 0)
        config = self._codex_config(root)
        self.assertIn('command = "a\\"b"', config)
        self.assertNotIn('command = "a"b"', config)

    def test_list_of_strings(self):
        result, root = self._sync_with_mcp(
            '[mcp.demo]\ncommand = "npx"\nargs = ["-y", "@demo/server"]\n'
        )
        self.assertEqual(result.returncode, 0)
        config = self._codex_config(root)
        self.assertIn('args = ["-y", "@demo/server"]', config)

    def test_nested_and_dict_roundtrip_via_tomllib(self):
        mcp_section = (
            '[mcp.demo]\ncommand = "npx"\n'
            'environment = { REGION = "us-east-1", RETRIES = 3 }\n'
        )
        result, root = self._sync_with_mcp(mcp_section)
        self.assertEqual(result.returncode, 0)
        config = self._codex_config(root)
        # Exact FROZEN dict bytes ({ k = v } with spaces, double-quoted strings).
        self.assertIn('env = { REGION = "us-east-1", RETRIES = 3 }', config)
        # The emitted document must still round-trip through tomllib: the env
        # dict and its nested scalars survive parsing unchanged.
        parsed = tomllib.loads(config)
        self.assertEqual(
            parsed["mcp_servers"]["demo"]["env"],
            {"REGION": "us-east-1", "RETRIES": 3},
        )
        self.assertEqual(parsed["mcp_servers"]["demo"]["env"]["RETRIES"], 3)

    def test_unsupported_type_raises(self):
        # TRIAGE: original asserted `toml_value(object())` raises TypeError.
        # No CLI input can reach a write site with a non-TOML-parsable value:
        # `mcp` values are always parsed from TOML (str/bool/int/float/list/
        # dict only) and recipe schemas declare only bool/string. Ran
        # `invoke(root, "sync", cli_home=...)` with a crafted `[mcp.demo]`
        # section; exit code was 0, no `.codex/config.toml` TypeError traceback
        # was emitted, and stdout/stderr carried no TypeError — the failure
        # branch of `toml_value` is unreachable through the CLI. Left coupled
        # (running, not skipped) to keep exercising the TypeError contract.
        mod = load_module(TOML_WRITE_PATH, "toml_write_under_test")
        with self.assertRaises(TypeError):
            mod.toml_value(object())


if __name__ == "__main__":
    unittest.main()
