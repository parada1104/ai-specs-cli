"""Tests for config_wizard.py."""
from __future__ import annotations

import importlib.util
import sys
import tempfile
import tomllib
import unittest
from pathlib import Path
from unittest.mock import MagicMock, patch

ROOT = Path(__file__).resolve().parents[1]
WIZARD_PATH = ROOT / "lib" / "_internal" / "config_wizard.py"
SCHEMA_PATH = ROOT / "lib" / "_internal" / "recipe_schema.py"
VENDOR = ROOT / "lib" / "_vendor"


def _ensure_vendor_path() -> None:
    if VENDOR.is_dir() and str(VENDOR) not in sys.path:
        sys.path.insert(0, str(VENDOR))


def load_module(path: Path, name: str):
    _ensure_vendor_path()
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


class ConfigWizardTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        _ensure_vendor_path()
        try:
            import questionary  # noqa: F401
        except ImportError as exc:
            raise unittest.SkipTest(f"questionary unavailable: {exc}") from exc
        cls.schema = load_module(SCHEMA_PATH, "recipe_schema_wizard")
        cls.mod = load_module(WIZARD_PATH, "config_wizard_internal")

    def _recipe(self, fields=None, extra=None, cli_deps=None, recipe_id="demo"):
        config = self.schema.ConfigSchema(
            fields=fields or {},
            extra=extra or {},
        )
        return self.schema.Recipe(
            id=recipe_id,
            name="Demo",
            description="D",
            version="1.0",
            config_schema=config,
            cli_deps=cli_deps or [],
        )

    def test_required_validator_rejects_blank_accepts_value(self):
        self.assertEqual(self.mod._required_validator(""), "This field is required.")
        self.assertEqual(self.mod._required_validator("   "), "This field is required.")
        self.assertIs(self.mod._required_validator("ok"), True)

    def test_regex_validator(self):
        v = self.mod._regex_validator(r"^[0-9a-fA-F]{24}$")
        self.assertIs(v(""), True)
        self.assertIs(v("0123456789abcdef01234567"), True)
        self.assertIn("Must match", v("bad"))

    def test_enum_field_uses_select(self):
        recipe = self._recipe(
            fields={
                "gate_mode": self.schema.ConfigField(
                    required=False, type="string", enum=["always", "ask", "off"], default="always"
                )
            }
        )
        select = MagicMock()
        select.return_value.ask.return_value = "ask"
        with patch.dict("sys.modules", {"questionary": MagicMock(select=select, text=MagicMock(), confirm=MagicMock())}):
            # Re-import path: run_config_wizard imports questionary inside function.
            import questionary as q

            with patch.object(q, "select", select):
                result = self.mod.run_config_wizard(recipe, {})
        select.assert_called()
        kwargs = select.call_args.kwargs
        self.assertEqual(kwargs.get("choices"), ["always", "ask", "off"])
        self.assertEqual(result["gate_mode"], "ask")

    def test_bool_field_uses_confirm(self):
        recipe = self._recipe(
            fields={
                "auto_remove_merged": self.schema.ConfigField(
                    required=False, type="bool", default=True
                )
            }
        )
        confirm = MagicMock()
        confirm.return_value.ask.return_value = False
        import questionary as q

        with patch.object(q, "confirm", confirm):
            result = self.mod.run_config_wizard(recipe, {})
        confirm.assert_called()
        self.assertIs(result["auto_remove_merged"], False)

    def test_default_prefill_kept_when_blank(self):
        recipe = self._recipe(
            fields={
                "base_branch": self.schema.ConfigField(
                    required=False, type="string", default="main"
                )
            }
        )
        text = MagicMock()
        text.return_value.ask.return_value = ""
        import questionary as q

        with patch.object(q, "text", text):
            result = self.mod.run_config_wizard(recipe, {})
        self.assertNotIn("base_branch", result)

    def test_existing_value_prefilled_as_default(self):
        recipe = self._recipe(
            fields={
                "base_branch": self.schema.ConfigField(
                    required=False, type="string", default="main"
                )
            }
        )
        text = MagicMock()
        text.return_value.ask.return_value = "develop"
        import questionary as q

        with patch.object(q, "text", text):
            self.mod.run_config_wizard(recipe, {"base_branch": "develop"})
        self.assertEqual(text.call_args.kwargs.get("default"), "develop")

    def test_extra_fields_never_prompted(self):
        recipe = self._recipe(
            fields={
                "board_id": self.schema.ConfigField(required=True, type="string"),
            },
            extra={"board_isolation": {"forbidden_tools": ["x"]}},
        )
        text = MagicMock()
        text.return_value.ask.return_value = "0123456789abcdef01234567"
        import questionary as q

        with patch.object(q, "text", text):
            result = self.mod.run_config_wizard(recipe, {})
        self.assertEqual(text.call_count, 1)
        self.assertIn("board_id", result)
        self.assertNotIn("board_isolation", result)

    def test_dep_gate_abort_skips_recipe(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            catalog = root / "catalog" / "recipes" / "demo"
            catalog.mkdir(parents=True)
            (catalog / "recipe.toml").write_text(
                "[recipe]\nid = \"demo\"\nname = \"D\"\ndescription = \"D\"\nversion = \"1\"\n\n"
                "[[deps.cli]]\nbinary = \"missing-bin\"\npurpose = \"x\"\nrequired = true\n\n"
                "[config.base_branch]\nrequired = false\ntype = \"string\"\ndefault = \"main\"\n"
            )
            project = root / "project"
            (project / "ai-specs").mkdir(parents=True)
            manifest = project / "ai-specs" / "ai-specs.toml"
            manifest.write_text(
                '[project]\nname = "p"\n\n[recipes.demo]\nenabled = true\nversion = "1"\n'
            )

            missing = [
                self.mod._dep_check.DepResult(
                    binary="missing-bin",
                    found=False,
                    version="",
                    ok=False,
                    install_url="",
                    purpose="x",
                    required=True,
                    recipe_id="demo",
                )
            ]
            confirm = MagicMock()
            confirm.return_value.ask.return_value = False
            import questionary as q

            with patch.dict("os.environ", {"AI_SPECS_HOME": str(root)}), patch.object(
                self.mod._dep_check, "check_cli_deps", return_value=missing
            ), patch.object(q, "confirm", confirm), patch.object(
                self.mod._config_write, "update_recipe_config"
            ) as write:
                configured = self.mod.configure_selected_recipes(project, ["demo"], manifest)
            write.assert_not_called()
            self.assertEqual(configured, {})

    def test_dep_gate_proceed_continues(self):
        recipe = self._recipe(
            fields={
                "base_branch": self.schema.ConfigField(
                    required=False, type="string", default="main"
                )
            },
            cli_deps=[self.schema.CliDep(binary="gh", purpose="PRs")],
        )
        missing = [
            self.mod._dep_check.DepResult(
                binary="gh",
                found=False,
                version="",
                ok=False,
                install_url="",
                purpose="PRs",
                required=True,
            )
        ]
        confirm = MagicMock()
        confirm.return_value.ask.return_value = True
        console = MagicMock()
        import questionary as q

        with patch.object(self.mod._dep_check, "check_cli_deps", return_value=missing), patch.object(
            q, "confirm", confirm
        ):
            self.assertTrue(self.mod._dep_gate(recipe, console))

    def test_configure_selected_writes_each(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            catalog = root / "catalog" / "recipes" / "demo"
            catalog.mkdir(parents=True)
            (catalog / "recipe.toml").write_text(
                "[recipe]\nid = \"demo\"\nname = \"D\"\ndescription = \"D\"\nversion = \"1\"\n\n"
                "[config.base_branch]\nrequired = false\ntype = \"string\"\ndefault = \"main\"\n"
            )
            project = root / "project"
            (project / "ai-specs").mkdir(parents=True)
            manifest = project / "ai-specs" / "ai-specs.toml"
            manifest.write_text(
                '[project]\nname = "p"\n\n[recipes.demo]\nenabled = true\nversion = "1"\n'
            )

            text = MagicMock()
            text.return_value.ask.return_value = "develop"
            import questionary as q

            with patch.dict("os.environ", {"AI_SPECS_HOME": str(root)}), patch.object(
                q, "text", text
            ):
                configured = self.mod.configure_selected_recipes(project, ["demo"], manifest)
            self.assertEqual(configured["demo"]["base_branch"], "develop")
            data = tomllib.loads(manifest.read_text(encoding="utf-8"))
            self.assertEqual(data["recipes"]["demo"]["config"]["base_branch"], "develop")



    def test_main_offers_envrc_generation(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = root / "project"
            (project / "ai-specs").mkdir(parents=True)
            (project / "ai-specs" / "ai-specs.toml").write_text(
                '[project]\nname = "p"\n\n[recipes.demo]\nenabled = true\nversion = "1"\n'
            )
            confirm = MagicMock()
            confirm.return_value.ask.return_value = True
            envrc = MagicMock()
            envrc.collect_env_vars.return_value = {"TRELLO_API_KEY": "required by trello"}
            envrc.write_envrc.return_value = project / "ai-specs" / ".envrc"
            import questionary as q

            with patch.object(self.mod, "_enabled_recipe_ids", return_value=["demo"]), patch.object(
                self.mod, "configure_selected_recipes", return_value={}
            ), patch.object(
                self.mod, "_load_sibling", return_value=envrc
            ), patch.object(q, "confirm", confirm), patch.object(
                self.mod._util, "ensure_deps", return_value=None
            ), patch("sys.stdin.isatty", return_value=True), patch(
                "sys.stdout.isatty", return_value=True
            ):
                rc = self.mod.main([str(project)])
            self.assertEqual(rc, 0)
            envrc.write_envrc.assert_called()
            # write_envrc(project_root, values) — first arg is project root
            self.assertEqual(envrc.write_envrc.call_args.args[0], project.resolve())

    def test_main_skips_envrc_when_no_mcp_recipes(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = root / "project"
            (project / "ai-specs").mkdir(parents=True)
            (project / "ai-specs" / "ai-specs.toml").write_text(
                '[project]\nname = "p"\n\n[recipes.demo]\nenabled = true\nversion = "1"\n'
            )
            confirm = MagicMock()
            envrc = MagicMock()
            envrc.collect_env_vars.return_value = {}
            import questionary as q

            with patch.object(self.mod, "_enabled_recipe_ids", return_value=["demo"]), patch.object(
                self.mod, "configure_selected_recipes", return_value={}
            ), patch.object(
                self.mod, "_load_sibling", return_value=envrc
            ), patch.object(q, "confirm", confirm), patch.object(
                self.mod._util, "ensure_deps", return_value=None
            ), patch("sys.stdin.isatty", return_value=True), patch(
                "sys.stdout.isatty", return_value=True
            ):
                rc = self.mod.main([str(project)])
            self.assertEqual(rc, 0)
            confirm.assert_not_called()
            envrc.generate_envrc_example.assert_not_called()



class ConfigureRecipesDispatchTests(unittest.TestCase):
    def test_configure_recipes_dispatch(self):
        import subprocess

        proc = subprocess.run(
            [str(ROOT / "bin" / "ai-specs"), "configure-recipes", "/nonexistent-ai-specs-path"],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertNotEqual(proc.returncode, 0)

    def test_recipe_config_sh_help(self):
        import subprocess

        proc = subprocess.run(
            ["bash", str(ROOT / "lib" / "recipe-config.sh"), "--help"],
            capture_output=True,
            text=True,
            check=False,
        )
        self.assertEqual(proc.returncode, 0)
        self.assertIn("configure-recipes", proc.stdout)



if __name__ == "__main__":
    unittest.main()
