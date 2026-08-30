"""Tests for env_scaffold.py (harness env layout)."""
from __future__ import annotations

import importlib.util
import os
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import MagicMock, patch

ROOT = Path(__file__).resolve().parents[1]
MOD_PATH = ROOT / "lib" / "_internal" / "env_scaffold.py"
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


class EnvScaffoldTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        _ensure_vendor_path()
        cls.mod = load_module(MOD_PATH, "env_scaffold_internal")

    def _project_with_recipe(
        self,
        root: Path,
        *,
        recipe_id: str,
        recipe_toml: str,
        enabled: bool = True,
    ) -> Path:
        catalog = root / "catalog" / "recipes" / recipe_id
        catalog.mkdir(parents=True)
        (catalog / "recipe.toml").write_text(recipe_toml, encoding="utf-8")
        project = root / "project"
        (project / "ai-specs").mkdir(parents=True)
        flag = "true" if enabled else "false"
        (project / "ai-specs" / "ai-specs.toml").write_text(
            '[project]\nname = "p"\n\n'
            f"[recipes.{recipe_id}]\nenabled = {flag}\nversion = \"1.0\"\n",
            encoding="utf-8",
        )
        return project

    def _trello_toml(self) -> str:
        return (
            "[recipe]\n"
            'id = "trello-mcp-workflow"\n'
            'name = "Trello"\n'
            'description = "D"\n'
            'version = "1.0"\n\n'
            "[[provides.mcp]]\n"
            'id = "trello"\n'
            'command = "npx"\n'
            "env = { TRELLO_API_KEY = \"$TRELLO_API_KEY\", TRELLO_TOKEN = \"$TRELLO_TOKEN\" }\n"
        )

    def test_write_env_uses_dotenv_not_export(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            (project / "ai-specs").mkdir(parents=True)
            app_env = project / ".env"
            app_env.write_text("APP=keep\n", encoding="utf-8")
            path = self.mod.write_env(project, {"TRELLO_API_KEY": "secret"})
            text = path.read_text(encoding="utf-8")
            self.assertIn("TRELLO_API_KEY=secret", text)
            self.assertNotIn("export ", text)
            self.assertEqual(app_env.read_text(encoding="utf-8"), "APP=keep\n")

    def test_write_env_merges_preserves_extras(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            project.mkdir()
            env = project / "ai-specs.env"
            env.write_text("CUSTOM=1\nTRELLO_API_KEY=old\n", encoding="utf-8")
            self.mod.write_env(project, {"TRELLO_API_KEY": "new"})
            text = env.read_text(encoding="utf-8")
            self.assertIn("CUSTOM=1", text)
            self.assertIn("TRELLO_API_KEY=new", text)

    def test_write_env_quotes_spaces(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            project.mkdir()
            self.mod.write_env(project, {"CANONICAL_VAULT_PATH": "/path with spaces/x"})
            text = (project / "ai-specs.env").read_text(encoding="utf-8")
            self.assertIn('CANONICAL_VAULT_PATH="/path with spaces/x"', text)

    def test_write_env_blank_preserves_existing_secret(self):
        """JD-1: blank/whitespace updates must not wipe non-empty harness secrets."""
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            project.mkdir()
            self.mod.write_env(project, {"TRELLO_API_KEY": "secret", "TRELLO_TOKEN": "tok"})
            self.mod.write_env(
                project,
                {"TRELLO_API_KEY": "", "TRELLO_TOKEN": "   ", "CUSTOM": "keep-me"},
            )
            text = (project / "ai-specs.env").read_text(encoding="utf-8")
            self.assertIn("TRELLO_API_KEY=secret", text)
            self.assertIn("TRELLO_TOKEN=tok", text)
            self.assertIn("CUSTOM=keep-me", text)

    def test_offer_harness_env_blank_prompt_preserves_existing(self):
        """JD-1: offer path with blank prompt values preserves prior ai-specs.env."""
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = self._project_with_recipe(
                root, recipe_id="trello-mcp-workflow", recipe_toml=self._trello_toml()
            )
            (project / "ai-specs.env").write_text(
                "TRELLO_API_KEY=keep-key\nTRELLO_TOKEN=keep-tok\n",
                encoding="utf-8",
            )
            blanks = {"TRELLO_API_KEY": "", "TRELLO_TOKEN": ""}
            with patch.dict(os.environ, {"AI_SPECS_HOME": str(root)}), patch.object(
                self.mod, "prompt_env_vars", return_value=blanks
            ), patch.object(self.mod, "direnv_allow", return_value=True):
                self.mod.offer_harness_env(project, offer_direnv_install=False)
            text = (project / "ai-specs.env").read_text(encoding="utf-8")
            self.assertIn("TRELLO_API_KEY=keep-key", text)
            self.assertIn("TRELLO_TOKEN=keep-tok", text)

    def test_generate_env_example(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = self._project_with_recipe(
                root, recipe_id="trello-mcp-workflow", recipe_toml=self._trello_toml()
            )
            with patch.dict(os.environ, {"AI_SPECS_HOME": str(root)}):
                path = self.mod.generate_env_example(project)
            text = path.read_text(encoding="utf-8")
            self.assertTrue(str(path).endswith("ai-specs.env.example"))
            self.assertIn("TRELLO_API_KEY=", text)
            self.assertIn("trello.com/power-ups/admin", text)
            self.assertNotIn("export ", text)
            self.assertFalse((project / "ai-specs" / ".envrc.example").exists())
            self.assertFalse((project / "ai-specs" / ".env.example").exists())

    def test_generate_env_example_backup(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = self._project_with_recipe(
                root, recipe_id="trello-mcp-workflow", recipe_toml=self._trello_toml()
            )
            example = project / "ai-specs.env.example"
            example.write_text("OLD\n", encoding="utf-8")
            with patch.dict(os.environ, {"AI_SPECS_HOME": str(root)}):
                self.mod.generate_env_example(project)
            self.assertEqual(
                (project / "ai-specs.env.example.bak").read_text(encoding="utf-8"),
                "OLD\n",
            )

    def test_generate_env_example_skips_identical_rewrite(self):
        """Idempotent sync must not create .bak when example content is unchanged."""
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = self._project_with_recipe(
                root, recipe_id="trello-mcp-workflow", recipe_toml=self._trello_toml()
            )
            with patch.dict(os.environ, {"AI_SPECS_HOME": str(root)}):
                self.mod.generate_env_example(project)
                self.mod.generate_env_example(project)
            self.assertFalse((project / "ai-specs.env.example.bak").exists())
            self.assertTrue((project / "ai-specs.env.example").is_file())

    def test_ensure_root_envrc_creates(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            project.mkdir()
            path = self.mod.ensure_root_envrc(project)
            text = path.read_text(encoding="utf-8")
            self.assertIn(self.mod.MANAGED_START, text)
            self.assertIn("dotenv_if_exists .env", text)
            self.assertIn("dotenv_if_exists ai-specs.env", text)

    def test_ensure_root_envrc_appends_preserving_custom(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            project.mkdir()
            envrc = project / ".envrc"
            envrc.write_text("use nix\n", encoding="utf-8")
            self.mod.ensure_root_envrc(project)
            text = envrc.read_text(encoding="utf-8")
            self.assertTrue(text.startswith("use nix\n"))
            self.assertIn(self.mod.MANAGED_START, text)

    def test_ensure_root_envrc_idempotent(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            project.mkdir()
            self.mod.ensure_root_envrc(project)
            self.mod.ensure_root_envrc(project)
            text = (project / ".envrc").read_text(encoding="utf-8")
            self.assertEqual(text.count(self.mod.MANAGED_START), 1)

    def test_migrate_legacy_envrc(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            (project / "ai-specs").mkdir(parents=True)
            legacy = project / "ai-specs" / ".envrc"
            legacy.write_text(
                'export TRELLO_TOKEN="abc"\nexport EMPTY=""\n',
                encoding="utf-8",
            )
            (project / "ai-specs.env").write_text(
                "TRELLO_API_KEY=keep\nTRELLO_TOKEN=existing\n",
                encoding="utf-8",
            )
            self.assertTrue(self.mod.migrate_legacy_envrc(project))
            env_text = (project / "ai-specs.env").read_text(encoding="utf-8")
            self.assertIn("TRELLO_API_KEY=keep", env_text)
            self.assertIn("TRELLO_TOKEN=existing", env_text)
            self.assertFalse(legacy.exists())
            self.assertTrue((project / "ai-specs" / ".envrc.bak").is_file())
            self.assertTrue((project / ".envrc").is_file())

    def test_migrate_legacy_envrc_fills_absent_key(self):
        """Absent harness key receives migrated export value from legacy .envrc."""
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            (project / "ai-specs").mkdir(parents=True)
            legacy = project / "ai-specs" / ".envrc"
            legacy.write_text(
                'export TRELLO_TOKEN="abc"\n',
                encoding="utf-8",
            )
            (project / "ai-specs.env").write_text(
                "TRELLO_API_KEY=keep\n",
                encoding="utf-8",
            )
            self.assertTrue(self.mod.migrate_legacy_envrc(project))
            env_text = (project / "ai-specs.env").read_text(encoding="utf-8")
            self.assertIn("TRELLO_API_KEY=keep", env_text)
            self.assertIn("TRELLO_TOKEN=abc", env_text)
            self.assertFalse(legacy.exists())
            self.assertTrue((project / "ai-specs" / ".envrc.bak").is_file())

    def test_migrate_nested_harness_env(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            (project / "ai-specs").mkdir(parents=True)
            nested = project / "ai-specs" / ".env"
            nested.write_text("TRELLO_API_KEY=legacy\n", encoding="utf-8")
            self.assertTrue(self.mod.migrate_nested_harness_env(project))
            env_text = (project / "ai-specs.env").read_text(encoding="utf-8")
            self.assertIn("TRELLO_API_KEY=legacy", env_text)
            self.assertFalse(nested.exists())
            self.assertTrue((project / "ai-specs" / ".env.bak").is_file())
            self.assertIn("dotenv_if_exists ai-specs.env", (project / ".envrc").read_text())

    def test_migrate_nested_empty_does_not_rename(self):
        """JD-6: comment-only nested .env must not be renamed to .env.bak."""
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            (project / "ai-specs").mkdir(parents=True)
            nested = project / "ai-specs" / ".env"
            nested.write_text("# only comments\n\n", encoding="utf-8")
            self.assertFalse(self.mod.migrate_nested_harness_env(project))
            self.assertTrue(nested.is_file())
            self.assertFalse((project / "ai-specs" / ".env.bak").exists())

    def test_migrate_nested_export_lines_merged(self):
        """JD-6: export-prefixed nested .env keys must parse, merge, then rename."""
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            (project / "ai-specs").mkdir(parents=True)
            nested = project / "ai-specs" / ".env"
            nested.write_text('export TRELLO_TOKEN="from-export"\n', encoding="utf-8")
            self.assertTrue(self.mod.migrate_nested_harness_env(project))
            env_text = (project / "ai-specs.env").read_text(encoding="utf-8")
            self.assertIn("TRELLO_TOKEN=from-export", env_text)
            self.assertFalse(nested.exists())
            self.assertTrue((project / "ai-specs" / ".env.bak").is_file())

    def test_migrate_legacy_envrc_empty_does_not_rename(self):
        """JD-6 parity: non-export legacy .envrc is left in place (no silent bak)."""
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            (project / "ai-specs").mkdir(parents=True)
            legacy = project / "ai-specs" / ".envrc"
            legacy.write_text("# no exports\n", encoding="utf-8")
            self.assertFalse(self.mod.migrate_legacy_envrc(project))
            self.assertTrue(legacy.is_file())
            self.assertFalse((project / "ai-specs" / ".envrc.bak").exists())

    def test_managed_block_is_current_rejects_stale_body(self):
        """JD-8 helper: markers with old dotenv path are not current."""
        stale = (
            f"{self.mod.MANAGED_START}\n"
            "dotenv_if_exists .env\n"
            "dotenv_if_exists ai-specs/.env\n"
            f"{self.mod.MANAGED_END}\n"
        )
        self.assertTrue(self.mod.has_managed_block(stale))
        self.assertFalse(self.mod.managed_block_is_current(stale))
        current = self.mod.managed_block_text() + "\n"
        self.assertTrue(self.mod.managed_block_is_current(current))

    def test_migrate_noop_when_absent(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = Path(tmp) / "project"
            (project / "ai-specs").mkdir(parents=True)
            self.assertFalse(self.mod.migrate_legacy_harness_env(project))

    def test_offer_harness_env_soft_fails_on_prompt_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = self._project_with_recipe(
                root, recipe_id="trello-mcp-workflow", recipe_toml=self._trello_toml()
            )
            with patch.dict(os.environ, {"AI_SPECS_HOME": str(root)}), patch.object(
                self.mod,
                "prompt_env_vars",
                side_effect=TypeError("password="),
            ):
                self.mod.offer_harness_env(project, offer_direnv_install=False)
            self.assertFalse((project / "ai-specs.env").exists())

    def test_offer_harness_env_invokes_direnv_allow(self):
        """When direnv is present, offer path runs `direnv allow <project_root>`."""
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = self._project_with_recipe(
                root, recipe_id="trello-mcp-workflow", recipe_toml=self._trello_toml()
            )
            values = {"TRELLO_API_KEY": "k", "TRELLO_TOKEN": "t"}
            proc = MagicMock(returncode=0)
            with patch.dict(os.environ, {"AI_SPECS_HOME": str(root)}), patch.object(
                self.mod, "prompt_env_vars", return_value=values
            ), patch("shutil.which", return_value="/usr/bin/direnv"), patch(
                "subprocess.run", return_value=proc
            ) as run:
                self.mod.offer_harness_env(project, offer_direnv_install=False)
            allow_calls = [
                c
                for c in run.call_args_list
                if c.args and list(c.args[0][:2]) == ["direnv", "allow"]
            ]
            self.assertEqual(len(allow_calls), 1)
            self.assertEqual(allow_calls[0].args[0], ["direnv", "allow", str(project)])
            self.assertTrue((project / "ai-specs.env").is_file())

    def test_offer_harness_env_soft_fails_without_direnv(self):
        """Missing direnv does not abort; non-fatal install guidance is printed."""
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = self._project_with_recipe(
                root, recipe_id="trello-mcp-workflow", recipe_toml=self._trello_toml()
            )
            values = {"TRELLO_API_KEY": "k", "TRELLO_TOKEN": "t"}
            with patch.dict(os.environ, {"AI_SPECS_HOME": str(root)}), patch.object(
                self.mod, "prompt_env_vars", return_value=values
            ), patch("shutil.which", return_value=None), patch(
                "subprocess.run", side_effect=FileNotFoundError("direnv")
            ), patch("builtins.print") as mock_print:
                # Soft-fail path only — isolate from direnv install offer.
                self.mod.offer_harness_env(project, offer_direnv_install=False)
            self.assertTrue((project / "ai-specs.env").is_file())
            self.assertTrue((project / ".envrc").is_file())
            guidance = " ".join(
                str(c.args[0]) for c in mock_print.call_args_list if c.args
            )
            self.assertIn("direnv", guidance.lower())
            self.assertIn("brew install direnv", guidance)

    def test_offer_harness_env_offers_direnv_install_when_missing_tty(self):
        """When direnv is missing on a TTY, offer path calls dep_install.offer_and_install."""
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = self._project_with_recipe(
                root, recipe_id="trello-mcp-workflow", recipe_toml=self._trello_toml()
            )
            values = {"TRELLO_API_KEY": "k", "TRELLO_TOKEN": "t"}
            dep_install = MagicMock()
            plan = MagicMock(
                binary="direnv",
                command=["brew", "install", "direnv"],
                kind="brew",
            )
            dep_install.resolve_install_plan.return_value = plan
            dep_install.offer_and_install.return_value = []
            with patch.dict(os.environ, {"AI_SPECS_HOME": str(root)}), patch.object(
                self.mod, "prompt_env_vars", return_value=values
            ), patch.object(self.mod, "_load_sibling", return_value=dep_install), patch.object(
                self.mod, "direnv_allow", return_value=False
            ), patch("shutil.which", return_value=None), patch.object(
                sys.stdin, "isatty", return_value=True
            ), patch.object(sys.stdout, "isatty", return_value=True):
                self.mod.offer_harness_env(project, offer_direnv_install=True)
            dep_install.resolve_install_plan.assert_called()
            self.assertEqual(
                dep_install.resolve_install_plan.call_args.args[0], "direnv"
            )
            dep_install.offer_and_install.assert_called_once()
            call_args, call_kwargs = dep_install.offer_and_install.call_args
            self.assertEqual(call_args[0], [plan])
            self.assertTrue(call_kwargs.get("tty"))

    def test_prompt_env_vars_uses_password_api_for_secrets(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = self._project_with_recipe(
                root, recipe_id="trello-mcp-workflow", recipe_toml=self._trello_toml()
            )
            password = MagicMock()
            password.return_value.ask.return_value = "secret-key"
            text = MagicMock()
            text.return_value.ask.return_value = "plain"
            confirm = MagicMock()
            confirm.return_value.ask.return_value = True
            q = MagicMock(password=password, text=text, confirm=confirm)
            with patch.dict(os.environ, {"AI_SPECS_HOME": str(root)}), patch.dict(
                "sys.modules", {"questionary": q}
            ):
                # MODE not in trello toml — only secrets
                result = self.mod.prompt_env_vars(project)
            self.assertEqual(result["TRELLO_API_KEY"], "secret-key")
            password.assert_called()

    def test_missing_required_values_reports_absent_and_blank(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = self._project_with_recipe(
                root, recipe_id="trello-mcp-workflow", recipe_toml=self._trello_toml()
            )
            (project / "ai-specs.env").write_text(
                "TRELLO_API_KEY=present\nTRELLO_TOKEN=\n",
                encoding="utf-8",
            )
            with patch.dict(os.environ, {"AI_SPECS_HOME": str(root)}):
                missing = self.mod.missing_required_values(project)
            self.assertEqual(missing, ["TRELLO_TOKEN"])

    def test_main_warns_missing_values_nonfatal(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            project = self._project_with_recipe(
                root, recipe_id="trello-mcp-workflow", recipe_toml=self._trello_toml()
            )
            (project / "ai-specs.env").write_text(
                "TRELLO_API_KEY=k\n",
                encoding="utf-8",
            )
            import io

            buf = io.StringIO()
            with patch.dict(os.environ, {"AI_SPECS_HOME": str(root)}), patch.object(
                sys, "stderr", buf
            ):
                rc = self.mod.main([str(project)])
            self.assertEqual(rc, 0)
            err_text = buf.getvalue()
            self.assertIn(
                "! TRELLO_TOKEN sin valor en ai-specs.env — ejecuta ai-specs configure-recipes",
                err_text,
            )
            self.assertNotIn("TRELLO_API_KEY", err_text)
            self.assertFalse((project / "ai-specs" / ".env.example").exists())
            self.assertTrue((project / "ai-specs.env.example").is_file())
            self.assertTrue((project / ".envrc").is_file())
            self.assertEqual(
                (project / "ai-specs.env").read_text(encoding="utf-8"),
                "TRELLO_API_KEY=k\n",
            )


class DepInstallTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.mod = load_module(
            ROOT / "lib" / "_internal" / "dep_install.py", "dep_install_internal"
        )

    def test_npx_guidance_only(self):
        plan = self.mod.resolve_install_plan(
            "npx", install_url="https://nodejs.org/en/download"
        )
        self.assertEqual(plan.kind, "guidance")
        self.assertEqual(plan.command, [])

    def test_bb_guidance_only(self):
        plan = self.mod.resolve_install_plan("bb", install_url="https://example.com")
        self.assertEqual(plan.kind, "guidance")

    def test_unknown_binary_guidance_only(self):
        """Binary outside _PACKAGE_MAP / _GUIDANCE_ONLY stays guidance with empty command."""
        plan = self.mod.resolve_install_plan("totally-unknown-bin")
        self.assertEqual(plan.kind, "guidance")
        self.assertEqual(plan.command, [])
        self.assertEqual(plan.binary, "totally-unknown-bin")

    def test_brew_plan_when_brew_present(self):
        with patch.object(self.mod.shutil, "which", side_effect=lambda b: "/opt/brew" if b == "brew" else None), patch.object(
            self.mod.platform, "system", return_value="Darwin"
        ):
            plan = self.mod.resolve_install_plan("gh")
        self.assertEqual(plan.kind, "brew")
        self.assertEqual(plan.command, ["brew", "install", "gh"])

    def test_apt_plan_on_linux(self):
        def which(b):
            if b == "apt-get":
                return "/usr/bin/apt-get"
            return None

        with patch.object(self.mod.shutil, "which", side_effect=which), patch.object(
            self.mod.platform, "system", return_value="Linux"
        ):
            plan = self.mod.resolve_install_plan("jq")
        self.assertEqual(plan.kind, "apt")
        self.assertEqual(plan.command[:3], ["sudo", "apt-get", "install"])

    def test_offer_non_tty_noop(self):
        plan = self.mod.InstallPlan(
            binary="jq",
            command=["brew", "install", "jq"],
            display="brew install jq",
            guidance_url="",
            kind="brew",
        )
        with patch.object(self.mod.subprocess, "run") as run:
            out = self.mod.offer_and_install([plan], tty=False)
        self.assertEqual(out, [])
        run.assert_not_called()

    def test_offer_decline_no_run(self):
        plan = self.mod.InstallPlan(
            binary="jq",
            command=["brew", "install", "jq"],
            display="brew install jq",
            guidance_url="",
            kind="brew",
        )
        confirm = MagicMock()
        confirm.return_value.ask.return_value = False
        q = MagicMock(confirm=confirm)
        with patch.dict("sys.modules", {"questionary": q}), patch.object(
            self.mod.subprocess, "run"
        ) as run:
            out = self.mod.offer_and_install([plan], tty=True)
        self.assertEqual(out, [])
        run.assert_not_called()

    def test_offer_accept_runs_and_rechecks(self):
        plan = self.mod.InstallPlan(
            binary="jq",
            command=["brew", "install", "jq"],
            display="brew install jq",
            guidance_url="",
            kind="brew",
        )
        confirm = MagicMock()
        confirm.return_value.ask.return_value = True
        q = MagicMock(confirm=confirm)
        proc = MagicMock(returncode=0)
        with patch.dict("sys.modules", {"questionary": q}), patch.object(
            self.mod.subprocess, "run", return_value=proc
        ) as run, patch.object(self.mod.shutil, "which", return_value="/usr/bin/jq"):
            out = self.mod.offer_and_install([plan], tty=True)
        self.assertEqual(out, ["jq"])
        run.assert_called_once()


if __name__ == "__main__":
    unittest.main()
